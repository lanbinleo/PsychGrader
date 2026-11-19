package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"psychgrader/internal/ced"
	"psychgrader/internal/db"
	"psychgrader/internal/grader"
	"psychgrader/internal/models"
	"psychgrader/internal/pdfparse"
)

type Server struct {
	Store     *db.Store
	Passcode  string
	SecretKey []byte
	gradeMu   sync.Mutex
}

func NewServer(store *db.Store) *Server {
	pass := strings.TrimSpace(os.Getenv("ADMIN_PASSCODE"))
	sec := os.Getenv("APP_SECRET")
	if sec == "" {
		sec = "psychgrader-hmac-key-2026"
	}
	return &Server{
		Store:     store,
		Passcode:  pass,
		SecretKey: []byte(sec),
	}
}

func (s *Server) signToken() string {
	mac := hmac.New(sha256.New, s.SecretKey)
	mac.Write([]byte("teacher-authenticated:" + s.Passcode))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Passcode == "" {
			c.Next()
			return
		}
		tok := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if tok == "" {
			tok = c.Query("token")
		}
		if tok != s.signToken() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "需要口令登录", "auth_required": true})
			return
		}
		c.Next()
	}
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/auth/status", s.handleAuthStatus)
		api.POST("/auth/login", s.handleLogin)

		protected := api.Group("")
		protected.Use(s.AuthMiddleware())
		{
			protected.GET("/settings", s.handleGetSettings)
			protected.PUT("/settings", s.handleUpdateSettings)
			protected.POST("/settings/test", s.handleTestLLM)

			protected.GET("/ced/search", s.handleSearchCED)

			protected.GET("/problem-sets", s.handleListProblemSets)
			protected.POST("/problem-sets", s.handleCreateProblemSet)
			protected.GET("/problem-sets/:id", s.handleGetProblemSet)
			protected.PUT("/problem-sets/:id", s.handleUpdateProblemSet)
			protected.DELETE("/problem-sets/:id", s.handleDeleteProblemSet)

			protected.POST("/problem-sets/:id/parse-pdf", s.handleParsePDF)
			protected.POST("/problem-sets/:id/import", s.handleImportSubmissions)
			protected.GET("/problem-sets/:id/submissions", s.handleListSubmissions)
			protected.POST("/problem-sets/:id/grade-all", s.handleGradeAll)
			protected.GET("/problem-sets/:id/export.csv", s.handleExportCSV)

			protected.GET("/submissions/:id", s.handleGetSubmission)
			protected.POST("/submissions/:id/grade", s.handleGradeOne)
			protected.PUT("/submissions/:id/review", s.handleReviewSubmission)
			protected.DELETE("/submissions/:id", s.handleDeleteSubmission)
		}
	}
}

func (s *Server) handleAuthStatus(c *gin.Context) {
	if s.Passcode == "" {
		c.JSON(http.StatusOK, gin.H{"auth_required": false, "authenticated": true})
		return
	}
	tok := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	c.JSON(http.StatusOK, gin.H{
		"auth_required": true,
		"authenticated": tok == s.signToken(),
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var body struct {
		Passcode string `json:"passcode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	if s.Passcode != "" && strings.TrimSpace(body.Passcode) != s.Passcode {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "访问口令不正确"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": s.signToken()})
}

func (s *Server) handleGetSettings(c *gin.Context) {
	cfg, _ := s.Store.LoadAppSettings(s.Passcode != "")
	c.JSON(http.StatusOK, cfg)
}

func (s *Server) handleUpdateSettings(c *gin.Context) {
	var in struct {
		APIBaseURL      string `json:"api_base_url"`
		APIKey          string `json:"api_key"`
		ClearAPIKey     bool   `json:"clear_api_key"`
		Model           string `json:"model"`
		ThinkingEnabled *bool  `json:"thinking_enabled"`
		ReasoningEffort string `json:"reasoning_effort"`
		Concurrency     int    `json:"concurrency"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(in.APIBaseURL) != "" {
		_ = s.Store.SetSetting("api_base_url", strings.TrimSpace(in.APIBaseURL))
	}
	if in.ClearAPIKey {
		_ = s.Store.SetSetting("api_key", "")
	} else if strings.TrimSpace(in.APIKey) != "" {
		_ = s.Store.SetSetting("api_key", strings.TrimSpace(in.APIKey))
	}
	if strings.TrimSpace(in.Model) != "" {
		_ = s.Store.SetSetting("model", strings.TrimSpace(in.Model))
	}
	if in.ThinkingEnabled != nil {
		if *in.ThinkingEnabled {
			_ = s.Store.SetSetting("thinking_enabled", "true")
		} else {
			_ = s.Store.SetSetting("thinking_enabled", "false")
		}
	}
	if strings.TrimSpace(in.ReasoningEffort) != "" {
		_ = s.Store.SetSetting("reasoning_effort", strings.TrimSpace(in.ReasoningEffort))
	}
	if in.Concurrency >= 1 && in.Concurrency <= 16 {
		_ = s.Store.SetSetting("concurrency", strconv.Itoa(in.Concurrency))
	}
	cfg, _ := s.Store.LoadAppSettings(s.Passcode != "")
	c.JSON(http.StatusOK, cfg)
}

func (s *Server) handleTestLLM(c *gin.Context) {
	var in struct {
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
		Model      string `json:"model"`
	}
	_ = c.ShouldBindJSON(&in)
	saved, savedKey := s.Store.LoadAppSettings(s.Passcode != "")
	key := strings.TrimSpace(in.APIKey)
	if key == "" {
		key = savedKey
	}
	baseURL := strings.TrimSpace(in.APIBaseURL)
	if baseURL == "" {
		baseURL = saved.APIBaseURL
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = saved.Model
	}
	reply, err := grader.TestConnection(c.Request.Context(), grader.DeepSeekConfig{
		BaseURL: baseURL,
		APIKey:  key,
		Model:   model,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reply": reply, "model": model})
}

func (s *Server) handleSearchCED(c *gin.Context) {
	c.JSON(http.StatusOK, ced.Search(c.Query("q")))
}

func (s *Server) handleListProblemSets(c *gin.Context) {
	list, err := s.Store.ListProblemSets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (s *Server) handleCreateProblemSet(c *gin.Context) {
	var ps models.ProblemSet
	if err := c.ShouldBindJSON(&ps); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(ps.Title) == "" || strings.TrimSpace(ps.PromptText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "题目标题与 Prompt 不能为空"})
		return
	}
	if err := s.Store.CreateProblemSet(&ps); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ps)
}

func (s *Server) handleGetProblemSet(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ps, err := s.Store.GetProblemSet(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "题目集不存在"})
		return
	}
	c.JSON(http.StatusOK, ps)
}

func (s *Server) handleUpdateProblemSet(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var ps models.ProblemSet
	if err := c.ShouldBindJSON(&ps); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ps.ID = id
	if err := s.Store.UpdateProblemSet(&ps); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, _ := s.Store.GetProblemSet(id)
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleDeleteProblemSet(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.Store.DeleteProblemSet(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (s *Server) handleParsePDF(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传 PDF 文件"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pages, err := pdfparse.ExtractPages(data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF 解析失败: " + err.Error()})
		return
	}

	mode := c.DefaultPostForm("mode", "auto")
	pps, _ := strconv.Atoi(c.DefaultPostForm("pages_per_student", "1"))
	segments := pdfparse.SplitPagesToSubmissions(pages, mode, pps)

	c.JSON(http.StatusOK, gin.H{
		"filename":   fh.Filename,
		"page_count": len(pages),
		"pages":      pages,
		"segments":   segments,
	})
}

func (s *Server) handleImportSubmissions(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if _, err := s.Store.GetProblemSet(psetID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "题目集不存在"})
		return
	}
	var in struct {
		Segments  []models.SplitSegment `json:"segments"`
		AutoGrade bool                  `json:"auto_grade"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := s.Store.BulkInsertSubmissions(psetID, in.Segments)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if in.AutoGrade && len(created) > 0 {
		ids := make([]int64, len(created))
		for i, sub := range created {
			ids[i] = sub.ID
		}
		go s.runBatchGrade(psetID, ids)
	}
	c.JSON(http.StatusCreated, gin.H{"imported_count": len(created), "submissions": created})
}

func (s *Server) handleListSubmissions(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	subs, err := s.Store.ListSubmissions(psetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, subs)
}

func (s *Server) handleGetSubmission(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	sub, err := s.Store.GetSubmission(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "答卷不存在"})
		return
	}
	c.JSON(http.StatusOK, sub)
}

func (s *Server) handleGradeOne(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	sub, err := s.Store.GetSubmission(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "答卷不存在"})
		return
	}
	ps, err := s.Store.GetProblemSet(sub.ProblemSetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "所属题目不存在"})
		return
	}

	_ = s.Store.SetSubmissionStatus(id, "grading", "")
	cfg, apiKey := s.Store.LoadAppSettings(s.Passcode != "")

	res, err := grader.GradeSubmission(c.Request.Context(), grader.DeepSeekConfig{
		BaseURL:         cfg.APIBaseURL,
		APIKey:          apiKey,
		Model:           cfg.Model,
		ThinkingEnabled: cfg.ThinkingEnabled,
		ReasoningEffort: cfg.ReasoningEffort,
	}, *ps, sub.RawText)
	if err != nil {
		_ = s.Store.SetSubmissionStatus(id, "error", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if err := s.Store.SaveGradingResult(id, res); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, _ := s.Store.GetSubmission(id)
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleGradeAll(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		OnlyUnscored bool `json:"only_unscored"`
	}
	_ = c.ShouldBindJSON(&body)

	subs, err := s.Store.ListSubmissions(psetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var ids []int64
	for _, sub := range subs {
		if body.OnlyUnscored && (sub.Status == "ai_graded" || sub.Status == "reviewed") {
			continue
		}
		ids = append(ids, sub.ID)
	}
	if len(ids) > 0 {
		go s.runBatchGrade(psetID, ids)
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": len(ids)})
}

func (s *Server) runBatchGrade(psetID int64, ids []int64) {
	ps, err := s.Store.GetProblemSet(psetID)
	if err != nil {
		return
	}
	cfg, apiKey := s.Store.LoadAppSettings(s.Passcode != "")
	dsCfg := grader.DeepSeekConfig{
		BaseURL:         cfg.APIBaseURL,
		APIKey:          apiKey,
		Model:           cfg.Model,
		ThinkingEnabled: cfg.ThinkingEnabled,
		ReasoningEffort: cfg.ReasoningEffort,
	}

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(subID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sub, err := s.Store.GetSubmission(subID)
			if err != nil {
				return
			}
			_ = s.Store.SetSubmissionStatus(subID, "grading", "")
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			res, err := grader.GradeSubmission(ctx, dsCfg, *ps, sub.RawText)
			if err != nil {
				_ = s.Store.SetSubmissionStatus(subID, "error", err.Error())
				return
			}
			_ = s.Store.SaveGradingResult(subID, res)
		}(id)
	}
	wg.Wait()
}

func (s *Server) handleReviewSubmission(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in struct {
		FinalScore   int                      `json:"final_score"`
		TeacherNotes string                   `json:"teacher_notes"`
		RubricParts  []models.RubricPartScore `json:"rubric_parts"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.Store.SaveTeacherReview(id, in.FinalScore, in.TeacherNotes, in.RubricParts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, _ := s.Store.GetSubmission(id)
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleDeleteSubmission(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.Store.DeleteSubmission(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (s *Server) handleExportCSV(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ps, err := s.Store.GetProblemSet(psetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "题目集不存在"})
		return
	}
	subs, err := s.Store.ListSubmissions(psetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	// Write UTF-8 BOM so Microsoft Excel renders Chinese comments properly
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)

	isAAQ := strings.ToUpper(ps.QuestionType) == "AAQ"
	var headers []string
	if isAAQ {
		headers = []string{"Student Name", "AP ID", "Pages", "Status", "Part A (1)", "Part B (1)", "Part C (1)", "Part D (1)", "Part E (1)", "Part F (2)", "AI Score (7)", "Final Score (7)", "AI Summary", "Teacher Notes"}
	} else {
		headers = []string{"Student Name", "AP ID", "Pages", "Status", "Part A Claim (1)", "Part B(i) Evid (1)", "Part B(ii) Reason (2)", "Part C(i) Evid (1)", "Part C(ii) Reason (2)", "AI Score (7)", "Final Score (7)", "Cited Sources", "Applied Concepts", "AI Summary", "Teacher Notes"}
	}
	_ = w.Write(headers)

	for _, sub := range subs {
		aiStr := ""
		if sub.AIScore != nil {
			aiStr = strconv.Itoa(*sub.AIScore)
		}
		finStr := aiStr
		if sub.FinalScore != nil {
			finStr = strconv.Itoa(*sub.FinalScore)
		}
		summary := ""
		partScores := make([]string, 6)
		var sources, concepts []string

		if sub.Result != nil {
			summary = sub.Result.SummaryZh
			for i, p := range sub.Result.RubricParts {
				if i < len(partScores) {
					sc := p.AIScore
					if p.TeacherScore != nil {
						sc = *p.TeacherScore
					}
					partScores[i] = strconv.Itoa(sc)
				}
				if p.CitedSource != "" {
					sources = append(sources, p.CitedSource)
				}
				if p.AppliedConcept != "" {
					concepts = append(concepts, p.AppliedConcept)
				}
			}
		}

		if isAAQ {
			_ = w.Write([]string{
				strings.TrimSpace(sub.StudentName), sub.StudentCode, sub.PageRange, sub.Status,
				partScores[0], partScores[1], partScores[2], partScores[3], partScores[4], partScores[5],
				aiStr, finStr, summary, sub.TeacherNotes,
			})
		} else {
			_ = w.Write([]string{
				strings.TrimSpace(sub.StudentName), sub.StudentCode, sub.PageRange, sub.Status,
				partScores[0], partScores[1], partScores[2], partScores[3], partScores[4],
				aiStr, finStr, strings.Join(sources, " / "), strings.Join(concepts, " | "),
				summary, sub.TeacherNotes,
			})
		}
	}
	w.Flush()

	safeTitle := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, ps.Title)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_Grades.csv"`, safeTitle))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
