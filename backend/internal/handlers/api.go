package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
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

const (
	maxUploadBytes = 25 << 20
	maxImportBytes = 12 << 20
	maxLoginFails  = 5
	loginWindow    = time.Minute
	tokenTTL       = 7 * 24 * time.Hour
)

type Server struct {
	Store     *db.Store
	Passcode  string
	SecretKey []byte

	gradeSem chan struct{}

	taskMu      sync.Mutex
	taskRunning map[int64]bool
	taskCancel  map[int64]context.CancelFunc
	oneShot     map[int64]bool

	loginMu    sync.Mutex
	loginFails map[string][]time.Time
}

func NewServer(store *db.Store) *Server {
	pass := strings.TrimSpace(os.Getenv("ADMIN_PASSCODE"))
	sec := []byte(strings.TrimSpace(os.Getenv("APP_SECRET")))
	if len(sec) == 0 {
		sec = make([]byte, 32)
		if _, err := rand.Read(sec); err != nil {
			// crypto/rand should not fail; fall back to a process-local value.
			sec = []byte(fmt.Sprintf("psychgrader-%d", time.Now().UnixNano()))
		}
	}
	return &Server{
		Store:       store,
		Passcode:    pass,
		SecretKey:   sec,
		gradeSem:    make(chan struct{}, 16),
		taskRunning: make(map[int64]bool),
		taskCancel:  make(map[int64]context.CancelFunc),
		oneShot:     make(map[int64]bool),
		loginFails:  make(map[string][]time.Time),
	}
}

func (s *Server) signToken() string {
	exp := time.Now().Add(tokenTTL).Unix()
	payload := fmt.Sprintf("%d:%s", exp, s.Passcode)
	mac := hmac.New(sha256.New, s.SecretKey)
	mac.Write([]byte(payload))
	token := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(exp, 10))) +
		"." + hex.EncodeToString(mac.Sum(nil))
	return token
}

func (s *Server) verifyToken(tok string) bool {
	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	exp, err := strconv.ParseInt(string(expBytes), 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	payload := fmt.Sprintf("%d:%s", exp, s.Passcode)
	mac := hmac.New(sha256.New, s.SecretKey)
	mac.Write([]byte(payload))
	want := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(want)) == 1
}

func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Passcode == "" {
			c.Next()
			return
		}
		tok := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if tok == "" || !s.verifyToken(tok) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "需要口令登录", "auth_required": true})
			return
		}
		c.Next()
	}
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
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
			protected.GET("/problem-sets/:id/submission-statuses", s.handleListSubmissionStatuses)
			protected.GET("/problem-sets/:id/active-task", s.handleActiveTask)
			protected.POST("/problem-sets/:id/grade-all", s.handleGradeAll)
			protected.GET("/problem-sets/:id/export.csv", s.handleExportCSV)

			protected.GET("/tasks", s.handleListTasks)
			protected.GET("/tasks/:id", s.handleGetTask)
			protected.POST("/tasks/:id/cancel", s.handleCancelTask)
			protected.POST("/tasks/:id/resume", s.handleResumeTask)

			protected.GET("/submissions/:id", s.handleGetSubmission)
			protected.POST("/submissions/:id/grade", s.handleGradeOne)
			protected.PUT("/submissions/:id/review", s.handleReviewSubmission)
			protected.DELETE("/submissions/:id", s.handleDeleteSubmission)
		}
	}
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAuthStatus(c *gin.Context) {
	if s.Passcode == "" {
		c.JSON(http.StatusOK, gin.H{"auth_required": false, "authenticated": true})
		return
	}
	tok := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	c.JSON(http.StatusOK, gin.H{"auth_required": true, "authenticated": s.verifyToken(tok)})
}

func (s *Server) allowLogin(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	kept := s.loginFails[ip][:0]
	for _, t := range s.loginFails[ip] {
		if now.Sub(t) < loginWindow {
			kept = append(kept, t)
		}
	}
	s.loginFails[ip] = kept
	return len(kept) < maxLoginFails
}

func (s *Server) recordLoginFailure(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	s.loginFails[ip] = append(s.loginFails[ip], time.Now())
}

func (s *Server) handleLogin(c *gin.Context) {
	ip := c.ClientIP()
	if !s.allowLogin(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试次数过多，请稍后再试"})
		return
	}
	var body struct {
		Passcode string `json:"passcode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	if s.Passcode != "" {
		given := []byte(strings.TrimSpace(body.Passcode))
		if subtle.ConstantTimeCompare(given, []byte(s.Passcode)) != 1 {
			s.recordLoginFailure(ip)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "访问口令不正确"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"token": s.signToken()})
}

func (s *Server) loadLLMConfig() grader.DeepSeekConfig {
	cfg, key := s.Store.LoadAppSettings(s.Passcode != "")
	return grader.DeepSeekConfig{
		BaseURL:         cfg.APIBaseURL,
		APIKey:          key,
		Model:           cfg.Model,
		ThinkingEnabled: cfg.ThinkingEnabled,
		ReasoningEffort: cfg.ReasoningEffort,
	}
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
		if err := s.validateBaseURL(in.APIBaseURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
		_ = s.Store.SetSetting("thinking_enabled", strconv.FormatBool(*in.ThinkingEnabled))
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

func isLocalHostname(h string) bool {
	h = strings.ToLower(strings.Trim(h, "[]"))
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func (s *Server) validateBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("API Base URL 格式不正确")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocalHostname(u.Hostname())) {
		return fmt.Errorf("API Base URL 必须使用 https（本地测试可用 127.0.0.1）")
	}
	if isLocalHostname(u.Hostname()) {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	allowed := map[string]bool{"api.deepseek.com": true}
	for _, h := range strings.Split(os.Getenv("ALLOWED_API_HOSTS"), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = true
		}
	}
	if allowed[host] {
		return nil
	}
	for _, prefix := range strings.Split(os.Getenv("ALLOWED_API_BASE_URLS"), ",") {
		prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
		if prefix == "" || !strings.HasPrefix(raw, prefix) {
			continue
		}
		rest := strings.TrimPrefix(raw, prefix)
		// Require a path boundary; reject https://api.deepseek.com.evil.example
		// and https://api.deepseek.com@evil.example.
		if rest == "" || strings.HasPrefix(rest, "/") {
			return nil
		}
	}
	return fmt.Errorf("不允许的 API 域名 %s，请使用官方地址或配置 ALLOWED_API_HOSTS", host)
}

func (s *Server) handleTestLLM(c *gin.Context) {
	var in struct {
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
		Model      string `json:"model"`
	}
	_ = c.ShouldBindJSON(&in)
	saved, _ := s.Store.LoadAppSettings(s.Passcode != "")
	key := strings.TrimSpace(in.APIKey)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "测试连接必须显式填写 API Key（不会使用已保存的 Key）"})
		return
	}
	baseURL := strings.TrimSpace(in.APIBaseURL)
	if baseURL == "" {
		baseURL = saved.APIBaseURL
	}
	if err := s.validateBaseURL(baseURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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
	if strings.TrimSpace(ps.Title) == "" || strings.TrimSpace(ps.PromptText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "题目标题与 Prompt 不能为空"})
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
	if active, _ := s.Store.GetActiveTask(id); active != nil {
		s.taskMu.Lock()
		cancel := s.taskCancel[active.ID]
		s.taskMu.Unlock()
		if cancel != nil {
			cancel()
		}
		_ = s.Store.CancelTask(active.ID)
	}
	if err := s.Store.DeleteProblemSet(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func (s *Server) handleParsePDF(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	fh, err := c.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "PDF 超过 25MB 上限"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传 PDF 文件"})
		return
	}
	if fh.Size > maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "PDF 超过 25MB 上限"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(data) > maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "PDF 超过 25MB 上限"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	pages, err := pdfparse.ExtractPages(ctx, data)
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportBytes)
	var in struct {
		Segments  []models.SplitSegment `json:"segments"`
		AutoGrade bool                  `json:"auto_grade"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		if isBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "导入数据超过 12MB 上限"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(in.Segments) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次导入不能超过 1000 份答卷"})
		return
	}
	created, err := s.Store.BulkInsertSubmissions(psetID, in.Segments)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{"imported_count": len(created), "submissions": created}
	if in.AutoGrade && len(created) > 0 {
		ids := make([]int64, len(created))
		for i, sub := range created {
			ids[i] = sub.ID
		}
		task, err := s.startNewTask(psetID, "import", ids)
		if errors.Is(err, db.ErrActiveTask) {
			resp["task_error"] = "该题集已有批改任务在进行中，请完成后再批改新导入的答卷"
		} else if err != nil {
			resp["task_error"] = err.Error()
		} else {
			resp["task"] = task
		}
	}
	c.JSON(http.StatusCreated, resp)
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

func (s *Server) handleListSubmissionStatuses(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	subs, err := s.Store.ListSubmissionStatuses(psetID)
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
	var in struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&in)

	sub, err := s.Store.GetSubmission(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "答卷不存在"})
		return
	}
	if sub.Status == "grading" || sub.Status == "queued" {
		c.JSON(http.StatusConflict, gin.H{"error": "该答卷正在批量批改中，请等待完成", "busy": true})
		return
	}
	if (sub.Status == "reviewed" || sub.Status == "needs_rereview") && !in.Force {
		c.JSON(http.StatusConflict, gin.H{
			"error":    "该答卷已核定；重跑会保留教师分项并标记为待复核，请确认后使用 force",
			"reviewed": true,
		})
		return
	}
	ps, err := s.Store.GetProblemSet(sub.ProblemSetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "所属题目不存在"})
		return
	}

	s.taskMu.Lock()
	if s.oneShot[id] {
		s.taskMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "该答卷正在单份批改中", "busy": true})
		return
	}
	s.oneShot[id] = true
	s.taskMu.Unlock()
	defer func() {
		s.taskMu.Lock()
		delete(s.oneShot, id)
		s.taskMu.Unlock()
	}()

	claimedForGrading := false
	defer func() {
		if !claimedForGrading {
			return
		}
		if cur, err := s.Store.GetSubmission(id); err == nil && cur.Status == "grading" {
			_ = s.Store.SetSubmissionStatus(id, "error", "批改异常中断，请重试")
		}
	}()

	if sub.Status != "reviewed" {
		claimed, err := s.Store.ClaimSubmissionForGrading(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !claimed {
			c.JSON(http.StatusConflict, gin.H{"error": "该答卷状态已变化，请刷新后重试", "busy": true})
			return
		}
		claimedForGrading = true
	}

	select {
	case s.gradeSem <- struct{}{}:
		defer func() { <-s.gradeSem }()
	case <-c.Request.Context().Done():
		_ = s.Store.SetSubmissionStatus(id, "error", "等待批改槽位时请求被取消")
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "请求已取消"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	cfg := s.loadLLMConfig()
	res, err := grader.GradeSubmission(ctx, cfg, *ps, sub.RawText)
	if err != nil {
		_ = s.Store.SetSubmissionStatus(id, "error", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if err := s.Store.SaveGradingResult(id, res, in.Force); err != nil {
		if errors.Is(err, db.ErrReviewed) {
			c.JSON(http.StatusConflict, gin.H{"error": "该答卷已核定，已拒绝覆盖", "reviewed": true})
			return
		}
		_ = s.Store.SetSubmissionStatus(id, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, err := s.Store.GetSubmission(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "答卷已被删除"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleGradeAll(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if _, err := s.Store.GetProblemSet(psetID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "题目集不存在"})
		return
	}
	var body struct {
		OnlyUnscored *bool `json:"only_unscored"`
	}
	_ = c.ShouldBindJSON(&body)
	onlyUnscored := true
	if body.OnlyUnscored != nil {
		onlyUnscored = *body.OnlyUnscored
	}

	active, err := s.Store.GetActiveTask(psetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if active != nil {
		c.JSON(http.StatusOK, gin.H{"task": active, "reused": true, "queued": 0, "skipped": 0})
		return
	}

	subs, err := s.Store.ListSubmissions(psetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var ids []int64
	skipped := 0
	for _, sub := range subs {
		if sub.Status == "reviewed" || sub.Status == "needs_rereview" {
			skipped++
			continue
		}
		if onlyUnscored && (sub.Status == "ai_graded" || sub.Status == "grading" || sub.Status == "queued") {
			skipped++
			continue
		}
		if !onlyUnscored && (sub.Status == "grading" || sub.Status == "queued") {
			skipped++
			continue
		}
		ids = append(ids, sub.ID)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"task": nil, "reused": false, "queued": 0, "skipped": skipped})
		return
	}
	task, err := s.startNewTask(psetID, "grade-all", ids)
	if errors.Is(err, db.ErrActiveTask) {
		active, _ := s.Store.GetActiveTask(psetID)
		if active != nil {
			c.JSON(http.StatusOK, gin.H{"task": active, "reused": true, "queued": 0, "skipped": skipped})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "任务状态已变化，请重试"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": task, "reused": false, "queued": task.Total, "skipped": skipped})
}

func (s *Server) startNewTask(psetID int64, source string, ids []int64) (*models.GradingTask, error) {
	task, err := s.Store.CreateGradingTask(psetID, source, ids)
	if err != nil {
		return nil, err
	}
	if task.Total == 0 {
		_ = s.Store.SetTaskFinished(task.ID, "completed", "")
		return s.Store.GetTask(task.ID)
	}
	go s.startTask(task.ID)
	return task, nil
}

func (s *Server) runTaskConcurrency() int {
	cfg, _ := s.Store.LoadAppSettings(s.Passcode != "")
	if cfg.Concurrency < 1 {
		return 1
	}
	if cfg.Concurrency > 16 {
		return 16
	}
	return cfg.Concurrency
}

func (s *Server) startTask(taskID int64) {
	s.taskMu.Lock()
	if s.taskRunning[taskID] {
		s.taskMu.Unlock()
		return
	}
	s.taskRunning[taskID] = true
	ctx, cancel := context.WithCancel(context.Background())
	s.taskCancel[taskID] = cancel
	s.taskMu.Unlock()
	defer func() {
		cancel()
		s.taskMu.Lock()
		delete(s.taskRunning, taskID)
		delete(s.taskCancel, taskID)
		s.taskMu.Unlock()
	}()

	task, err := s.Store.GetTask(taskID)
	if err != nil {
		_ = s.Store.SetTaskPaused(taskID, "读取任务失败: "+err.Error())
		return
	}
	if task.Status == "canceled" {
		return
	}
	ps, err := s.Store.GetProblemSet(task.ProblemSetID)
	if err != nil {
		_ = s.Store.SetTaskFinished(taskID, "failed", "题目集不存在或已删除")
		return
	}
	cfg := s.loadLLMConfig()
	items, err := s.Store.ListQueuedTaskItems(taskID)
	if err != nil {
		_ = s.Store.SetTaskPaused(taskID, "读取任务项失败: "+err.Error())
		return
	}
	if len(items) == 0 {
		_ = s.Store.RefreshTaskCounts(taskID)
		_ = s.Store.SetTaskFinished(taskID, "completed", "")
		return
	}
	running, err := s.Store.SetTaskRunning(taskID)
	if err != nil {
		_ = s.Store.SetTaskPaused(taskID, "启动任务失败: "+err.Error())
		return
	}
	if !running {
		return
	}

	workers := s.runTaskConcurrency()
	if workers > len(items) {
		workers = len(items)
	}
	itemCh := make(chan int64)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for subID := range itemCh {
				s.gradeTaskItemSafe(ctx, taskID, subID, ps, cfg)
			}
		}()
	}
feed:
	for _, id := range items {
		select {
		case <-ctx.Done():
			break feed
		case itemCh <- id:
		}
	}
	close(itemCh)
	wg.Wait()
	_ = s.Store.RefreshTaskCounts(taskID)
	if ctx.Err() != nil {
		_ = s.Store.SetTaskFinished(taskID, "canceled", "已取消")
		return
	}
	final, err := s.Store.GetTask(taskID)
	if err != nil {
		_ = s.Store.SetTaskFinished(taskID, "failed", err.Error())
		return
	}
	summary := ""
	if final.Failed > 0 {
		summary = fmt.Sprintf("%d 份失败，可在队列中重试", final.Failed)
	}
	_ = s.Store.SetTaskFinished(taskID, "completed", summary)
}

func (s *Server) gradeTaskItemSafe(ctx context.Context, taskID, subID int64, ps *models.ProblemSet, cfg grader.DeepSeekConfig) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("批改 worker panic: %v", r)
			_ = s.Store.SetTaskItemStatus(taskID, subID, "failed", msg)
			_ = s.Store.SetSubmissionStatus(subID, "error", msg)
			_ = s.Store.RefreshTaskCounts(taskID)
		}
	}()
	s.gradeTaskItem(ctx, taskID, subID, ps, cfg)
}

func (s *Server) gradeTaskItem(ctx context.Context, taskID, subID int64, ps *models.ProblemSet, cfg grader.DeepSeekConfig) {
	claimed, err := s.Store.ClaimTaskItem(taskID, subID)
	if err != nil || !claimed {
		return
	}
	if ctx.Err() != nil {
		_ = s.Store.SetTaskItemStatus(taskID, subID, "canceled", "已取消")
		_ = s.Store.SetSubmissionStatus(subID, "canceled", "批改已取消")
		return
	}
	ok, err := s.Store.ClaimSubmission(subID)
	if err != nil || !ok {
		_ = s.Store.SetTaskItemStatus(taskID, subID, "canceled", "答卷已核定或状态已变化")
		return
	}
	sub, err := s.Store.GetSubmission(subID)
	if err != nil {
		_ = s.Store.SetTaskItemStatus(taskID, subID, "failed", "答卷不存在或已被删除")
		_ = s.Store.SetSubmissionStatus(subID, "error", "答卷不存在或已被删除")
		_ = s.Store.RefreshTaskCounts(taskID)
		return
	}

	select {
	case s.gradeSem <- struct{}{}:
		defer func() { <-s.gradeSem }()
	case <-ctx.Done():
		_ = s.Store.SetTaskItemStatus(taskID, subID, "canceled", "已取消")
		_ = s.Store.SetSubmissionStatus(subID, "canceled", "批改已取消")
		return
	}

	itemCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	res, err := grader.GradeSubmission(itemCtx, cfg, *ps, sub.RawText)
	if err != nil {
		if ctx.Err() != nil {
			_ = s.Store.SetTaskItemStatus(taskID, subID, "canceled", "已取消")
			_ = s.Store.SetSubmissionStatus(subID, "canceled", "批改已取消")
		} else if errors.Is(err, context.DeadlineExceeded) {
			_ = s.Store.SetTaskItemStatus(taskID, subID, "failed", "批改超时（180s）")
			_ = s.Store.SetSubmissionStatus(subID, "error", "批改超时（180s），可重试")
		} else {
			_ = s.Store.SetTaskItemStatus(taskID, subID, "failed", err.Error())
			_ = s.Store.SetSubmissionStatus(subID, "error", err.Error())
		}
		_ = s.Store.RefreshTaskCounts(taskID)
		return
	}
	if ctx.Err() != nil {
		_ = s.Store.SetTaskItemStatus(taskID, subID, "canceled", "已取消")
		_ = s.Store.SetSubmissionStatus(subID, "canceled", "批改已取消")
		_ = s.Store.RefreshTaskCounts(taskID)
		return
	}
	if err := s.Store.SaveGradingResult(subID, res, false); err != nil {
		if errors.Is(err, db.ErrReviewed) {
			_ = s.Store.SetTaskItemStatus(taskID, subID, "canceled", "已核定，跳过")
		} else {
			_ = s.Store.SetTaskItemStatus(taskID, subID, "failed", err.Error())
			_ = s.Store.SetSubmissionStatus(subID, "error", err.Error())
		}
		_ = s.Store.RefreshTaskCounts(taskID)
		return
	}
	_ = s.Store.SetTaskItemStatus(taskID, subID, "succeeded", "")
	_ = s.Store.RefreshTaskCounts(taskID)
}

func (s *Server) handleListTasks(c *gin.Context) {
	tasks, err := s.Store.ListActiveTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func (s *Server) handleActiveTask(c *gin.Context) {
	psetID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	task, err := s.Store.GetActiveTask(psetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusOK, gin.H{"task": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (s *Server) handleGetTask(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	task, err := s.Store.GetTask(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) handleCancelTask(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	task, err := s.Store.GetTask(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "canceled" {
		c.JSON(http.StatusConflict, gin.H{"error": "任务已结束"})
		return
	}
	s.taskMu.Lock()
	cancel := s.taskCancel[id]
	s.taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err := s.Store.CancelTask(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.Store.RefreshTaskCounts(id)
	updated, err := s.Store.GetTask(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务已不存在"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleResumeTask(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	task, err := s.Store.GetTask(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if task.Status != "paused" && task.Status != "failed" {
		c.JSON(http.StatusConflict, gin.H{"error": "只有暂停或失败的任务可以继续"})
		return
	}
	s.taskMu.Lock()
	alreadyRunning := s.taskRunning[id]
	s.taskMu.Unlock()
	if alreadyRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "任务仍在运行中"})
		return
	}
	if err := s.Store.PrepareTaskResume(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.Store.RefreshTaskCounts(id)
	go s.startTask(id)
	updated, err := s.Store.GetTask(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务已不存在"})
		return
	}
	c.JSON(http.StatusOK, updated)
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
	sub, err := s.Store.GetSubmission(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "答卷不存在"})
		return
	}
	maxScore := 7
	if sub.Result != nil && sub.Result.MaxTotalScore > 0 {
		maxScore = sub.Result.MaxTotalScore
	} else {
		sum := 0
		for _, p := range in.RubricParts {
			sum += p.MaxScore
		}
		if sum > 0 {
			maxScore = sum
		}
	}
	if in.FinalScore < 0 || in.FinalScore > maxScore {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("最终分必须在 0-%d 之间", maxScore)})
		return
	}
	for i := range in.RubricParts {
		p := &in.RubricParts[i]
		if p.MaxScore <= 0 {
			continue
		}
		if p.AIScore < 0 || p.AIScore > p.MaxScore {
			c.JSON(http.StatusBadRequest, gin.H{"error": "AI 分超出采分项上限"})
			return
		}
		if p.TeacherScore != nil && (*p.TeacherScore < 0 || *p.TeacherScore > p.MaxScore) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "教师分超出采分项上限"})
			return
		}
	}
	if err := s.Store.SaveTeacherReview(id, in.FinalScore, in.TeacherNotes, in.RubricParts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, err := s.Store.GetSubmission(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "答卷不存在"})
		return
	}
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

func csvCell(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t':
		return "'" + s
	}
	return s
}

func safeFilename(title string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-", "\r", " ", "\n", " ")
	return replacer.Replace(strings.TrimSpace(title))
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
				csvCell(strings.TrimSpace(sub.StudentName)), csvCell(sub.StudentCode), csvCell(sub.PageRange), csvCell(sub.Status),
				partScores[0], partScores[1], partScores[2], partScores[3], partScores[4], partScores[5],
				aiStr, finStr, csvCell(summary), csvCell(sub.TeacherNotes),
			})
		} else {
			_ = w.Write([]string{
				csvCell(strings.TrimSpace(sub.StudentName)), csvCell(sub.StudentCode), csvCell(sub.PageRange), csvCell(sub.Status),
				partScores[0], partScores[1], partScores[2], partScores[3], partScores[4],
				aiStr, finStr, csvCell(strings.Join(sources, " / ")), csvCell(strings.Join(concepts, " | ")),
				csvCell(summary), csvCell(sub.TeacherNotes),
			})
		}
	}
	w.Flush()

	filename := safeFilename(ps.Title) + "_Grades.csv"
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
