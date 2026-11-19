package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"psychgrader/internal/db"
	"psychgrader/internal/handlers"
	"psychgrader/internal/models"
)

// buildMinimalMultiPagePDF constructs a standards-compliant uncompressed PDF 1.4
// with len(pageLines) pages using built-in Type1 Helvetica, exercising pure-Go PDF extraction.
func buildMinimalMultiPagePDF(pageLines [][]string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	offsets := make(map[int]int)
	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	// Obj 1: Catalog, Obj 2: Pages, Obj 3: Font
	numPages := len(pageLines)
	kids := make([]string, 0, numPages)
	for i := 0; i < numPages; i++ {
		pageObjID := 4 + i*2
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObjID))
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), numPages))
	writeObj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	escapePDF := func(s string) string {
		r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
		return r.Replace(s)
	}

	for i, lines := range pageLines {
		pageObjID := 4 + i*2
		contentObjID := pageObjID + 1

		var stream bytes.Buffer
		stream.WriteString("BT\n/F1 11 Tf\n50 750 Td\n16 TL\n")
		for idx, ln := range lines {
			if idx == 0 {
				fmt.Fprintf(&stream, "(%s) Tj\n", escapePDF(ln))
			} else {
				fmt.Fprintf(&stream, "T* (%s) Tj\n", escapePDF(ln))
			}
		}
		stream.WriteString("ET\n")
		contentBytes := stream.String()

		writeObj(pageObjID, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
			contentObjID,
		))
		writeObj(contentObjID, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(contentBytes), contentBytes))
	}

	totalObjs := 3 + numPages*2
	xrefPos := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", totalObjs+1)
	buf.WriteString("0000000000 65535 f \n")
	for id := 1; id <= totalObjs; id++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", totalObjs+1, xrefPos)
	return buf.Bytes()
}

func TestFullEndToEndPipeline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. Start a local mock DeepSeek V4.1 server verifying protocol parameters
	var capturedModel string
	var capturedThinking bool
	mockDeepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-mock-deepseek-v41" {
			t.Errorf("unexpected auth header: %s", got)
		}
		var reqPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
			t.Fatalf("decode mock request: %v", err)
		}
		capturedModel, _ = reqPayload["model"].(string)
		if th, ok := reqPayload["thinking"].(map[string]any); ok && th["type"] == "enabled" {
			capturedThinking = true
		}

		// Handle connectivity ping vs full rubric grading
		msgs, _ := reqPayload["messages"].([]any)
		if len(msgs) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"DeepSeek-V4.1-Flash-1017","choices":[{"message":{"content":"OK"}}]}`))
			return
		}

		// Return realistic grading JSON where C(ii) tries to sneak in a duplicate synonym ("distributed practice")
		// so we also verify deterministic post-processing caps C(ii) from 2 -> 1!
		mockGradingJSON := `{
			"summary_zh": "立论清晰，但 C(ii) 与 B(ii) 使用了同义概念。",
			"translations": [
				{"id": "S1", "zh": "分散练习能显著增强新学信息的长期提取能力。"},
				{"id": "S2", "zh": "在 Source 1 中，分四周练习缝合的住院医一个月后得分高于单日组（Source 1）。"},
				{"id": "S3", "zh": "这通过间隔效应支撑了主张，给大脑留出巩固技能的时间。"},
				{"id": "S4", "zh": "在 Source 2 中，间隔 48 小时阅读科学文章的学生一周后保持了 73% 的高分（Source 2）。"},
				{"id": "S5", "zh": "这同样通过分散练习（distributed practice）证明了间隔学习能维持记忆。"}
			],
			"annotations": [
				{"id": "A1", "sentence_id": "S1", "exact_quote": "Spaced practice strengthens long-term retrieval", "category": "claim", "part_id": "part_a", "badge": "Claim (+1)", "comment_zh": "明确可辩护主张"},
				{"id": "A2", "sentence_id": "S2", "exact_quote": "Source 1, surgical residents spaced across four weeks scored higher", "category": "evidence", "part_id": "part_b_i", "badge": "Source 1 (+1)", "comment_zh": "准确引用 Source 1"},
				{"id": "A3", "sentence_id": "S3", "exact_quote": "spacing effect", "category": "concept", "part_id": "part_b_ii", "badge": "Spacing Effect (+2)", "comment_zh": "有效运用间隔效应"}
			],
			"rubric_parts": [
				{"part_id": "part_a", "label": "Part A · Claim", "subtitle": "提出主张", "max_score": 1, "ai_score": 1, "reason_zh": "主张清晰"},
				{"part_id": "part_b_i", "label": "Part B(i) · Evidence 1", "subtitle": "证据1", "max_score": 1, "ai_score": 1, "cited_source": "Source 1", "reason_zh": "引用 Source 1 准确"},
				{"part_id": "part_b_ii", "label": "Part B(ii) · Reasoning 1", "subtitle": "推理1", "max_score": 2, "ai_score": 2, "cited_source": "Source 1", "applied_concept": "spacing effect", "reason_zh": "运用 Spacing Effect 得当"},
				{"part_id": "part_c_i", "label": "Part C(i) · Evidence 2", "subtitle": "证据2", "max_score": 1, "ai_score": 1, "cited_source": "Source 2", "reason_zh": "引用 Source 2 准确"},
				{"part_id": "part_c_ii", "label": "Part C(ii) · Reasoning 2", "subtitle": "推理2", "max_score": 2, "ai_score": 2, "cited_source": "Source 2", "applied_concept": "distributed practice", "reason_zh": "模型误给了 2 分，应被规则引擎拦截为同义重复 1 分"}
			]
		}`

		respEnvelope := map[string]any{
			"model": "DeepSeek-V4.1-Flash-1017",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"reasoning_content": "Step 1: Check Part A claim -> valid. Step 2: Check Source 1 & Source 2 -> distinct.",
						"content":           mockGradingJSON,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(respEnvelope)
	}))
	defer mockDeepSeek.Close()

	// 2. Boot SQLite + Gin router with ADMIN_PASSCODE enabled
	t.Setenv("ADMIN_PASSCODE", "tsinglan-2026")
	tmpDB := filepath.Join(t.TempDir(), "e2e.db")
	store, err := db.Open(tmpDB)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.DB.Close()

	router := gin.New()
	srv := handlers.NewServer(store)
	srv.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 3. Verify unauthenticated access is rejected with 401
	unauthResp, err := http.Get(ts.URL + "/api/problem-sets")
	if err != nil {
		t.Fatalf("GET problem-sets: %v", err)
	}
	unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 before login, got %d", unauthResp.StatusCode)
	}

	// 4. Login with teacher passcode
	loginRes, err := http.Post(
		ts.URL+"/api/auth/login",
		"application/json",
		strings.NewReader(`{"passcode":"tsinglan-2026"}`),
	)
	if err != nil {
		t.Fatalf("login post: %v", err)
	}
	defer loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login failed with %d", loginRes.StatusCode)
	}
	var loginBody struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(loginRes.Body).Decode(&loginBody)
	if loginBody.Token == "" {
		t.Fatal("expected non-empty HMAC auth token")
	}

	doJSON := func(method, path string, body any, out any) int {
		var rdr io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rdr)
		req.Header.Set("Authorization", "Bearer "+loginBody.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", method, path, err)
		}
		defer resp.Body.Close()
		if out != nil {
			_ = json.NewDecoder(resp.Body).Decode(out)
		}
		return resp.StatusCode
	}

	// 5. Configure DeepSeek settings pointing to our mock V4.1 server & run connectivity test
	var updatedCfg models.AppSettings
	if code := doJSON("PUT", "/api/settings", map[string]any{
		"api_base_url":     mockDeepSeek.URL,
		"api_key":          "sk-mock-deepseek-v41",
		"model":            "deepseek-flash",
		"thinking_enabled": true,
		"reasoning_effort": "medium",
		"concurrency":      3,
	}, &updatedCfg); code != 200 || !updatedCfg.HasAPIKey {
		t.Fatalf("settings update failed: code=%d cfg=%+v", code, updatedCfg)
	}

	var pingOut struct {
		OK    bool   `json:"ok"`
		Reply string `json:"reply"`
	}
	if code := doJSON("POST", "/api/settings/test", map[string]any{"api_key": "sk-mock-deepseek-v41"}, &pingOut); code != 200 || !pingOut.OK {
		t.Fatalf("LLM test endpoint failed: code=%d out=%+v", code, pingOut)
	}

	// 6. Upload a 3-page batch PDF containing 2 students (Student 1 spans pages 1–2; Student 2 on page 3)
	pdfBytes := buildMinimalMultiPagePDF([][]string{
		{
			"Student: Lucas Wang",
			"AP ID: 99881122",
			"Spaced practice strengthens long-term retrieval of new material.",
			"In Source 1, surgical residents spaced across four weeks scored higher than single-day trainees (Source 1).",
			"This supports my claim via the spacing effect, giving time for consolidation.",
		},
		{
			"In Source 2, students who spaced science reading by 48 hours retained 73% after a week (Source 2).",
			"This also works because distributed practice prevents cramming decay.",
		},
		{
			"Student: Sophia Liu",
			"AP ID: 99881199",
			"Active testing and spaced intervals improve retention.",
			"In Source 3, one hour of self-quizzing matched 23 hours of lecture review (Source 3).",
		},
	})

	var mpBuf bytes.Buffer
	mw := multipart.NewWriter(&mpBuf)
	fw, _ := mw.CreateFormFile("file", "cb_batch.pdf")
	_, _ = fw.Write(pdfBytes)
	_ = mw.WriteField("mode", "auto")
	_ = mw.Close()

	parseReq, _ := http.NewRequest("POST", ts.URL+"/api/problem-sets/1/parse-pdf", &mpBuf)
	parseReq.Header.Set("Authorization", "Bearer "+loginBody.Token)
	parseReq.Header.Set("Content-Type", mw.FormDataContentType())
	parseResp, err := http.DefaultClient.Do(parseReq)
	if err != nil {
		t.Fatalf("parse-pdf post: %v", err)
	}
	defer parseResp.Body.Close()
	if parseResp.StatusCode != 200 {
		raw, _ := io.ReadAll(parseResp.Body)
		t.Fatalf("parse-pdf status=%d body=%s", parseResp.StatusCode, string(raw))
	}
	var parsedOut struct {
		PageCount int                   `json:"page_count"`
		Segments  []models.SplitSegment `json:"segments"`
	}
	_ = json.NewDecoder(parseResp.Body).Decode(&parsedOut)
	if parsedOut.PageCount != 3 {
		t.Fatalf("expected 3 PDF pages, got %d", parsedOut.PageCount)
	}
	if len(parsedOut.Segments) != 2 {
		t.Fatalf("expected smart splitter to merge continuation page into 2 students, got %d: %+v", len(parsedOut.Segments), parsedOut.Segments)
	}
	if !strings.Contains(parsedOut.Segments[0].StudentName, "Lucas Wang") || parsedOut.Segments[0].PageRange != "P.1-2" {
		t.Fatalf("unexpected student 1 segment: %+v", parsedOut.Segments[0])
	}

	// 7. Import the 2 parsed segments without auto-grade, then grade Lucas synchronously
	var importOut struct {
		ImportedCount int                 `json:"imported_count"`
		Submissions   []models.Submission `json:"submissions"`
	}
	if code := doJSON("POST", "/api/problem-sets/1/import", map[string]any{
		"segments":   parsedOut.Segments,
		"auto_grade": false,
	}, &importOut); code != 201 || importOut.ImportedCount != 2 {
		t.Fatalf("import failed: code=%d out=%+v", code, importOut)
	}

	lucasID := importOut.Submissions[0].ID
	var gradedSub models.Submission
	if code := doJSON("POST", fmt.Sprintf("/api/submissions/%d/grade", lucasID), nil, &gradedSub); code != 200 {
		t.Fatalf("gradeOne failed with %d", code)
	}

	if capturedModel != "deepseek-flash" || !capturedThinking {
		t.Fatalf("expected deepseek-flash + thinking:enabled, got model=%q thinking=%v", capturedModel, capturedThinking)
	}
	// Crucial check: LLM returned 7 (1+1+2+1+2), but EnforceOfficialRules caught "distributed practice" == "spacing effect" and capped total to 6!
	if gradedSub.Result == nil || gradedSub.Result.AITotalScore != 6 {
		t.Fatalf("expected deterministic rule engine to cap duplicate synonym score from 7 to 6, got %+v", gradedSub.Result)
	}
	if len(gradedSub.Result.Sentences) != 5 || gradedSub.Result.Sentences[0].Zh == "" {
		t.Fatalf("expected 5 aligned bilingual sentences, got %+v", gradedSub.Result.Sentences)
	}

	// 8. Teacher reviews Lucas's submission and overrides score to 7 with a note
	var reviewedSub models.Submission
	overrideParts := gradedSub.Result.RubricParts
	seven := 2
	overrideParts[4].TeacherScore = &seven
	if code := doJSON("PUT", fmt.Sprintf("/api/submissions/%d/review", lucasID), map[string]any{
		"final_score":   7,
		"teacher_notes": "教师人工复核通过",
		"rubric_parts":  overrideParts,
	}, &reviewedSub); code != 200 || reviewedSub.Status != "reviewed" || *reviewedSub.FinalScore != 7 {
		t.Fatalf("teacher review failed: code=%d sub=%+v", code, reviewedSub)
	}

	// 8.5 Reviewed submissions are protected unless the caller explicitly forces.
	var conflict map[string]any
	if code := doJSON("POST", fmt.Sprintf("/api/submissions/%d/grade", lucasID), nil, &conflict); code != 409 {
		t.Fatalf("expected 409 when regrading a reviewed submission, got %d: %+v", code, conflict)
	}
	var forced models.Submission
	if code := doJSON("POST", fmt.Sprintf("/api/submissions/%d/grade", lucasID), map[string]any{"force": true}, &forced); code != 200 {
		t.Fatalf("forced regrade failed: code=%d", code)
	}
	if forced.Status != "needs_rereview" {
		t.Fatalf("expected needs_rereview after forced regrade, got %q", forced.Status)
	}
	if forced.FinalScore == nil || *forced.FinalScore != 7 {
		t.Fatalf("teacher final score must be preserved, got %+v", forced.FinalScore)
	}
	if forced.Result == nil || len(forced.Result.RubricParts) < 5 || forced.Result.RubricParts[4].TeacherScore == nil {
		t.Fatalf("teacher rubric override must be merged into the new AI result: %+v", forced.Result)
	}

	// 8.6 Batch grading creates a persistent task and the second call is idempotent.
	sophiaID := importOut.Submissions[1].ID
	var batchOut struct {
		Task  models.GradingTask `json:"task"`
		Reuse bool               `json:"reused"`
		Queued int               `json:"queued"`
	}
	if code := doJSON("POST", "/api/problem-sets/1/grade-all", map[string]any{"only_unscored": true}, &batchOut); code != 202 {
		t.Fatalf("grade-all failed: code=%d", code)
	}
	if batchOut.Task.ID == 0 || batchOut.Task.Total != 1 || batchOut.Queued != 1 {
		t.Fatalf("unexpected batch task: %+v", batchOut)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var current models.GradingTask
		if code := doJSON("GET", fmt.Sprintf("/api/tasks/%d", batchOut.Task.ID), nil, &current); code != 200 {
			t.Fatalf("get task failed: %d", code)
		}
		if current.Status == "completed" || current.Status == "failed" || current.Status == "canceled" {
			if current.Status != "completed" || current.Succeeded != 1 {
				t.Fatalf("batch did not complete successfully: %+v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("batch task timed out: %+v", current)
		}
		time.Sleep(25 * time.Millisecond)
	}
	var sophia models.Submission
	if code := doJSON("GET", fmt.Sprintf("/api/submissions/%d", sophiaID), nil, &sophia); code != 200 {
		t.Fatalf("get sophia failed: %d", code)
	}
	if sophia.Status != "ai_graded" || sophia.Result == nil {
		t.Fatalf("sophia should be graded by the batch task: %+v", sophia)
	}

	// 9. Verify CED glossary search & CSV gradebook export
	var cedHits []cedEntryStub
	if code := doJSON("GET", "/api/ced/search?q=interference", nil, &cedHits); code != 200 || len(cedHits) == 0 {
		t.Fatalf("expected CED hits for interference, got code=%d len=%d", code, len(cedHits))
	}

	csvReq, _ := http.NewRequest("GET", ts.URL+"/api/problem-sets/1/export.csv", nil)
	csvReq.Header.Set("Authorization", "Bearer "+loginBody.Token)
	csvResp, err := http.DefaultClient.Do(csvReq)
	if err != nil {
		t.Fatalf("export csv: %v", err)
	}
	csvBytes, _ := io.ReadAll(csvResp.Body)
	csvResp.Body.Close()
	if csvResp.StatusCode != 200 || !strings.Contains(string(csvBytes), "Lucas Wang") || !strings.Contains(string(csvBytes), "教师人工复核通过") {
		t.Fatalf("unexpected CSV export: status=%d len=%d", csvResp.StatusCode, len(csvBytes))
	}
}

type cedEntryStub struct {
	Term string `json:"term"`
	Unit string `json:"unit"`
}
