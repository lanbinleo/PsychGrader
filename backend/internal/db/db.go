package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"psychgrader/internal/models"
)

// ErrReviewed is returned when a regrade would overwrite a teacher-reviewed
// submission without an explicit force flag.
var ErrReviewed = errors.New("submission is already reviewed")

type Store struct {
	DB *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	s := &Store{DB: conn}
	if err := s.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := s.seedIfEmpty(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := s.ReconcileInterruptedTasks(); err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS problem_sets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	question_type TEXT NOT NULL DEFAULT 'EBQ',
	prompt_text TEXT NOT NULL,
	sources_json TEXT NOT NULL DEFAULT '[]',
	ineligible_concepts_json TEXT NOT NULL DEFAULT '[]',
	scoring_notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS submissions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	problem_set_id INTEGER NOT NULL REFERENCES problem_sets(id) ON DELETE CASCADE,
	student_name TEXT NOT NULL,
	student_code TEXT NOT NULL DEFAULT '',
	page_range TEXT NOT NULL DEFAULT '',
	raw_text TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	ai_score INTEGER,
	final_score INTEGER,
	result_json TEXT NOT NULL DEFAULT '',
	teacher_notes TEXT NOT NULL DEFAULT '',
	error_msg TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_submissions_pset ON submissions(problem_set_id);

CREATE TABLE IF NOT EXISTS grading_tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	problem_set_id INTEGER NOT NULL REFERENCES problem_sets(id) ON DELETE CASCADE,
	status TEXT NOT NULL DEFAULT 'queued',
	total INTEGER NOT NULL DEFAULT 0,
	succeeded INTEGER NOT NULL DEFAULT 0,
	failed INTEGER NOT NULL DEFAULT 0,
	canceled INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT '',
	error_summary TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS grading_task_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id INTEGER NOT NULL REFERENCES grading_tasks(id) ON DELETE CASCADE,
	submission_id INTEGER NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
	status TEXT NOT NULL DEFAULT 'queued',
	attempt INTEGER NOT NULL DEFAULT 0,
	error_msg TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT '',
	UNIQUE(task_id, submission_id)
);

CREATE INDEX IF NOT EXISTS idx_task_items_task ON grading_task_items(task_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_pset ON grading_tasks(problem_set_id, status);
`
	_, err := s.DB.Exec(schema)
	return err
}

func nowISO() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (s *Store) GetSetting(key, def string) string {
	var val string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return def
	}
	return val
}

func (s *Store) SetSetting(key, val string) error {
	_, err := s.DB.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, val)
	return err
}

func (s *Store) LoadAppSettings(envPassRequired bool) (models.AppSettings, string) {
	baseURL := s.GetSetting("api_base_url", os.Getenv("DEEPSEEK_BASE_URL"))
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepseek.com"
	}
	apiKey := s.GetSetting("api_key", os.Getenv("DEEPSEEK_API_KEY"))
	model := s.GetSetting("model", os.Getenv("DEEPSEEK_MODEL"))
	if strings.TrimSpace(model) == "" {
		model = "deepseek-flash"
	}
	thinkingStr := s.GetSetting("thinking_enabled", "true")
	effort := s.GetSetting("reasoning_effort", "high")
	conc := 6
	fmt.Sscanf(s.GetSetting("concurrency", "6"), "%d", &conc)
	if conc < 1 {
		conc = 1
	}
	if conc > 16 {
		conc = 16
	}

	masked := ""
	trimmedKey := strings.TrimSpace(apiKey)
	if len(trimmedKey) > 8 {
		masked = trimmedKey[:4] + "••••••••" + trimmedKey[len(trimmedKey)-4:]
	} else if len(trimmedKey) > 0 {
		masked = "••••••••"
	}

	return models.AppSettings{
		APIBaseURL:      baseURL,
		APIKeyMasked:    masked,
		HasAPIKey:       trimmedKey != "",
		Model:           model,
		ThinkingEnabled: thinkingStr != "false",
		ReasoningEffort: effort,
		Concurrency:     conc,
		AuthEnabled:     envPassRequired,
	}, trimmedKey
}

func (s *Store) ListProblemSets() ([]models.ProblemSet, error) {
	rows, err := s.DB.Query(`
SELECT p.id, p.title, p.question_type, p.prompt_text, p.sources_json, p.ineligible_concepts_json, p.scoring_notes, p.created_at, p.updated_at,
       COUNT(sub.id) AS total_cnt,
       SUM(CASE WHEN sub.status IN ('ai_graded', 'reviewed') THEN 1 ELSE 0 END) AS graded_cnt,
       SUM(CASE WHEN sub.status = 'reviewed' THEN 1 ELSE 0 END) AS reviewed_cnt,
       COALESCE(AVG(COALESCE(sub.final_score, sub.ai_score)), 0) AS avg_sc
FROM problem_sets p
LEFT JOIN submissions sub ON sub.problem_set_id = p.id
GROUP BY p.id
ORDER BY p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ProblemSet
	for rows.Next() {
		var ps models.ProblemSet
		var srcJSON, badJSON string
		var gradedCnt, revCnt sql.NullInt64
		if err := rows.Scan(&ps.ID, &ps.Title, &ps.QuestionType, &ps.PromptText, &srcJSON, &badJSON, &ps.ScoringNotes, &ps.CreatedAt, &ps.UpdatedAt,
			&ps.TotalCount, &gradedCnt, &revCnt, &ps.AvgScore); err != nil {
			return nil, err
		}
		ps.GradedCount = int(gradedCnt.Int64)
		ps.ReviewedCount = int(revCnt.Int64)
		_ = json.Unmarshal([]byte(srcJSON), &ps.Sources)
		_ = json.Unmarshal([]byte(badJSON), &ps.IneligibleConcepts)
		if ps.Sources == nil {
			ps.Sources = []models.SourceDoc{}
		}
		if ps.IneligibleConcepts == nil {
			ps.IneligibleConcepts = []string{}
		}
		out = append(out, ps)
	}
	if out == nil {
		out = []models.ProblemSet{}
	}
	return out, nil
}

func (s *Store) GetProblemSet(id int64) (*models.ProblemSet, error) {
	var ps models.ProblemSet
	var srcJSON, badJSON string
	var gradedCnt, revCnt sql.NullInt64
	err := s.DB.QueryRow(`
SELECT p.id, p.title, p.question_type, p.prompt_text, p.sources_json, p.ineligible_concepts_json, p.scoring_notes, p.created_at, p.updated_at,
       COUNT(sub.id) AS total_cnt,
       SUM(CASE WHEN sub.status IN ('ai_graded', 'reviewed') THEN 1 ELSE 0 END) AS graded_cnt,
       SUM(CASE WHEN sub.status = 'reviewed' THEN 1 ELSE 0 END) AS reviewed_cnt,
       COALESCE(AVG(COALESCE(sub.final_score, sub.ai_score)), 0) AS avg_sc
FROM problem_sets p
LEFT JOIN submissions sub ON sub.problem_set_id = p.id
WHERE p.id = ?
GROUP BY p.id`, id).Scan(&ps.ID, &ps.Title, &ps.QuestionType, &ps.PromptText, &srcJSON, &badJSON, &ps.ScoringNotes, &ps.CreatedAt, &ps.UpdatedAt,
		&ps.TotalCount, &gradedCnt, &revCnt, &ps.AvgScore)
	if err != nil {
		return nil, err
	}
	ps.GradedCount = int(gradedCnt.Int64)
	ps.ReviewedCount = int(revCnt.Int64)
	_ = json.Unmarshal([]byte(srcJSON), &ps.Sources)
	_ = json.Unmarshal([]byte(badJSON), &ps.IneligibleConcepts)
	if ps.Sources == nil {
		ps.Sources = []models.SourceDoc{}
	}
	if ps.IneligibleConcepts == nil {
		ps.IneligibleConcepts = []string{}
	}
	return &ps, nil
}

func (s *Store) CreateProblemSet(ps *models.ProblemSet) error {
	now := nowISO()
	if ps.QuestionType == "" {
		ps.QuestionType = "EBQ"
	}
	srcJSON, _ := json.Marshal(ps.Sources)
	badJSON, _ := json.Marshal(ps.IneligibleConcepts)
	res, err := s.DB.Exec(`
INSERT INTO problem_sets(title, question_type, prompt_text, sources_json, ineligible_concepts_json, scoring_notes, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		ps.Title, ps.QuestionType, ps.PromptText, string(srcJSON), string(badJSON), ps.ScoringNotes, now, now)
	if err != nil {
		return err
	}
	ps.ID, _ = res.LastInsertId()
	ps.CreatedAt = now
	ps.UpdatedAt = now
	return nil
}

func (s *Store) UpdateProblemSet(ps *models.ProblemSet) error {
	now := nowISO()
	srcJSON, _ := json.Marshal(ps.Sources)
	badJSON, _ := json.Marshal(ps.IneligibleConcepts)
	_, err := s.DB.Exec(`
UPDATE problem_sets
SET title = ?, question_type = ?, prompt_text = ?, sources_json = ?, ineligible_concepts_json = ?, scoring_notes = ?, updated_at = ?
WHERE id = ?`,
		ps.Title, ps.QuestionType, ps.PromptText, string(srcJSON), string(badJSON), ps.ScoringNotes, now, ps.ID)
	return err
}

func (s *Store) DeleteProblemSet(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM problem_sets WHERE id = ?`, id)
	return err
}

func (s *Store) ListSubmissions(psetID int64) ([]models.Submission, error) {
	rows, err := s.DB.Query(`
SELECT id, problem_set_id, student_name, student_code, page_range, raw_text, status, ai_score, final_score, result_json, teacher_notes, error_msg, created_at, updated_at
FROM submissions
WHERE problem_set_id = ?
ORDER BY id ASC`, psetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Submission
	for rows.Next() {
		sub, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	if out == nil {
		out = []models.Submission{}
	}
	return out, nil
}

func (s *Store) GetSubmission(id int64) (*models.Submission, error) {
	row := s.DB.QueryRow(`
SELECT id, problem_set_id, student_name, student_code, page_range, raw_text, status, ai_score, final_score, result_json, teacher_notes, error_msg, created_at, updated_at
FROM submissions WHERE id = ?`, id)
	sub, err := scanSubmission(row)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubmission(r rowScanner) (models.Submission, error) {
	var sub models.Submission
	var aiSc, finSc sql.NullInt64
	var resJSON string
	err := r.Scan(&sub.ID, &sub.ProblemSetID, &sub.StudentName, &sub.StudentCode, &sub.PageRange, &sub.RawText,
		&sub.Status, &aiSc, &finSc, &resJSON, &sub.TeacherNotes, &sub.ErrorMsg, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return sub, err
	}
	if aiSc.Valid {
		v := int(aiSc.Int64)
		sub.AIScore = &v
	}
	if finSc.Valid {
		v := int(finSc.Int64)
		sub.FinalScore = &v
	}
	if strings.TrimSpace(resJSON) != "" {
		var gr models.GradingResult
		if err := json.Unmarshal([]byte(resJSON), &gr); err == nil {
			sub.Result = &gr
		}
	}
	return sub, nil
}

func (s *Store) BulkInsertSubmissions(psetID int64, segs []models.SplitSegment) ([]models.Submission, error) {
	now := nowISO()
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT INTO submissions(problem_set_id, student_name, student_code, page_range, raw_text, status, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, 'pending', ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var created []models.Submission
	for i, seg := range segs {
		txt := strings.TrimSpace(seg.RawText)
		if txt == "" {
			continue
		}
		name := strings.TrimSpace(seg.StudentName)
		if name == "" {
			name = fmt.Sprintf("Student #%02d", i+1)
		}
		res, err := stmt.Exec(psetID, name, strings.TrimSpace(seg.StudentCode), strings.TrimSpace(seg.PageRange), txt, now, now)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		created = append(created, models.Submission{
			ID:           id,
			ProblemSetID: psetID,
			StudentName:  name,
			StudentCode:  seg.StudentCode,
			PageRange:    seg.PageRange,
			RawText:      txt,
			Status:       "pending",
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) SetSubmissionStatus(id int64, status string, errMsg string) error {
	_, err := s.DB.Exec(`UPDATE submissions SET status = ?, error_msg = ?, updated_at = ? WHERE id = ? AND status <> 'reviewed'`,
		status, errMsg, nowISO(), id)
	return err
}

// ClaimSubmission atomically moves a submission into "grading". It returns
// false when another worker already claimed it or it is reviewed.
func (s *Store) ClaimSubmission(id int64) (bool, error) {
	res, err := s.DB.Exec(`
UPDATE submissions SET status = 'grading', error_msg = '', updated_at = ?
WHERE id = ? AND status IN ('queued', 'pending', 'error', 'canceled')`, nowISO(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SaveGradingResult protects teacher-reviewed submissions: without force it
// refuses to overwrite a reviewed submission. With force (or when the
// submission is already needs_rereview) it merges the teacher's per-part
// scores/comments and marks the submission needs_rereview.
func (s *Store) SaveGradingResult(id int64, res *models.GradingResult, force bool) error {
	raw, err := json.Marshal(res)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status, oldJSON string
	if err := tx.QueryRow(`SELECT status, result_json FROM submissions WHERE id = ?`, id).Scan(&status, &oldJSON); err != nil {
		return err
	}
	if status == "reviewed" && !force {
		return ErrReviewed
	}

	newStatus := "ai_graded"
	var old models.GradingResult
	if strings.TrimSpace(oldJSON) != "" {
		_ = json.Unmarshal([]byte(oldJSON), &old)
	}
	if status == "reviewed" || status == "needs_rereview" || hasTeacherParts(&old) {
		mergeTeacherParts(&old, res)
		newStatus = "needs_rereview"
	}

	if _, err := tx.Exec(`
UPDATE submissions
SET status = ?, ai_score = ?, result_json = ?, error_msg = '', updated_at = ?
WHERE id = ?`, newStatus, res.AITotalScore, string(raw), nowISO(), id); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeTeacherParts(old *models.GradingResult, res *models.GradingResult) {
	if old == nil {
		return
	}
	byID := make(map[string]models.RubricPartScore, len(old.RubricParts))
	for _, p := range old.RubricParts {
		byID[p.PartID] = p
	}
	for i := range res.RubricParts {
		p, ok := byID[res.RubricParts[i].PartID]
		if !ok {
			continue
		}
		if p.TeacherScore != nil {
			res.RubricParts[i].TeacherScore = p.TeacherScore
		}
		if p.TeacherComment != "" {
			res.RubricParts[i].TeacherComment = p.TeacherComment
		}
	}
}

func hasTeacherParts(res *models.GradingResult) bool {
	if res == nil {
		return false
	}
	for _, p := range res.RubricParts {
		if p.TeacherScore != nil || p.TeacherComment != "" {
			return true
		}
	}
	return false
}

func (s *Store) SaveTeacherReview(id int64, finalScore int, teacherNotes string, updatedParts []models.RubricPartScore) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var resJSON string
	if err := tx.QueryRow(`SELECT result_json FROM submissions WHERE id = ?`, id).Scan(&resJSON); err != nil {
		return err
	}
	var result models.GradingResult
	if strings.TrimSpace(resJSON) != "" {
		_ = json.Unmarshal([]byte(resJSON), &result)
	}
	if len(updatedParts) > 0 {
		result.RubricParts = updatedParts
	}
	maxTotal := 0
	total := 0
	for _, p := range result.RubricParts {
		maxTotal += p.MaxScore
		total += p.AIScore
	}
	if result.MaxTotalScore == 0 {
		result.MaxTotalScore = maxTotal
	}
	if result.AITotalScore == 0 {
		result.AITotalScore = total
	}
	raw, err := json.Marshal(&result)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE submissions
SET status = 'reviewed', final_score = ?, teacher_notes = ?, result_json = ?, updated_at = ?
WHERE id = ?`, finalScore, teacherNotes, string(raw), nowISO(), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteSubmission(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM submissions WHERE id = ?`, id)
	return err
}

// ---------- Grading tasks ----------

// ReconcileInterruptedTasks runs once at startup: in-flight work is paused,
// items go back to the queue, and submissions no longer look forever-grading.
func (s *Store) ReconcileInterruptedTasks() error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE grading_task_items SET status = 'queued', started_at = '' WHERE status = 'running'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE grading_tasks SET status = 'paused', error_summary = '服务重启中断，可在任务中心继续' WHERE status IN ('queued', 'running')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE submissions SET status = 'queued', error_msg = '服务重启中断，等待继续', updated_at = ? WHERE status = 'grading'`, nowISO()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateGradingTask(psetID int64, source string, ids []int64) (*models.GradingTask, error) {
	now := nowISO()
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
INSERT INTO grading_tasks(problem_set_id, status, total, source, created_at)
VALUES(?, 'queued', 0, ?, ?)`, psetID, source, now)
	if err != nil {
		return nil, err
	}
	taskID, _ := res.LastInsertId()
	total := 0
	for _, id := range ids {
		upd, err := tx.Exec(`
UPDATE submissions SET status = 'queued', error_msg = '', updated_at = ?
WHERE id = ? AND problem_set_id = ? AND status IN ('pending', 'error', 'canceled', 'ai_graded')`, now, id, psetID)
		if err != nil {
			return nil, err
		}
		n, _ := upd.RowsAffected()
		if n == 0 {
			continue
		}
		if _, err := tx.Exec(`
INSERT OR IGNORE INTO grading_task_items(task_id, submission_id, status) VALUES(?, ?, 'queued')`, taskID, id); err != nil {
			return nil, err
		}
		total++
	}
	if _, err := tx.Exec(`UPDATE grading_tasks SET total = ? WHERE id = ?`, total, taskID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTask(taskID)
}

func scanTask(row rowScanner) (models.GradingTask, error) {
	var t models.GradingTask
	err := row.Scan(&t.ID, &t.ProblemSetID, &t.Status, &t.Total, &t.Succeeded,
		&t.Failed, &t.Canceled, &t.Source, &t.ErrorSummary, &t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	t.Current = []models.TaskItem{}
	return t, err
}

const taskColumns = `id, problem_set_id, status, total, succeeded, failed, canceled, source, error_summary, created_at, started_at, finished_at`

func (s *Store) GetTask(id int64) (*models.GradingTask, error) {
	t, err := scanTask(s.DB.QueryRow(`SELECT `+taskColumns+` FROM grading_tasks WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`
SELECT i.task_id, i.submission_id, s.student_name, i.status, i.error_msg, i.started_at, i.finished_at
FROM grading_task_items i
JOIN submissions s ON s.id = i.submission_id
WHERE i.task_id = ? AND i.status = 'running'
ORDER BY i.started_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it models.TaskItem
		if err := rows.Scan(&it.TaskID, &it.SubmissionID, &it.StudentName, &it.Status, &it.ErrorMsg, &it.StartedAt, &it.FinishedAt); err != nil {
			return nil, err
		}
		t.Current = append(t.Current, it)
	}
	return &t, nil
}

func (s *Store) GetActiveTask(psetID int64) (*models.GradingTask, error) {
	t, err := scanTask(s.DB.QueryRow(`
SELECT `+taskColumns+` FROM grading_tasks
WHERE problem_set_id = ? AND status IN ('queued', 'running', 'paused')
ORDER BY id DESC LIMIT 1`, psetID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetTask(t.ID)
}

func (s *Store) ListActiveTasks() ([]models.GradingTask, error) {
	rows, err := s.DB.Query(`
SELECT ` + taskColumns + ` FROM grading_tasks
WHERE status IN ('queued', 'running', 'paused') ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.GradingTask{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *Store) ListQueuedTaskItems(taskID int64) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT submission_id FROM grading_task_items WHERE task_id = ? AND status = 'queued' ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *Store) ClaimTaskItem(taskID, subID int64) (bool, error) {
	res, err := s.DB.Exec(`
UPDATE grading_task_items
SET status = 'running', attempt = attempt + 1, started_at = ?, error_msg = ''
WHERE task_id = ? AND submission_id = ? AND status = 'queued'`, nowISO(), taskID, subID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) SetTaskItemStatus(taskID, subID int64, status, errMsg string) error {
	_, err := s.DB.Exec(`
UPDATE grading_task_items
SET status = ?, error_msg = ?, finished_at = ?
WHERE task_id = ? AND submission_id = ?`, status, errMsg, nowISO(), taskID, subID)
	return err
}

func (s *Store) RefreshTaskCounts(taskID int64) error {
	_, err := s.DB.Exec(`
UPDATE grading_tasks SET
  succeeded = (SELECT COUNT(*) FROM grading_task_items WHERE task_id = ? AND status = 'succeeded'),
  failed = (SELECT COUNT(*) FROM grading_task_items WHERE task_id = ? AND status = 'failed'),
  canceled = (SELECT COUNT(*) FROM grading_task_items WHERE task_id = ? AND status = 'canceled')
WHERE id = ?`, taskID, taskID, taskID, taskID)
	return err
}

func (s *Store) SetTaskRunning(taskID int64) (bool, error) {
	res, err := s.DB.Exec(`
UPDATE grading_tasks SET status = 'running',
  started_at = CASE WHEN started_at = '' THEN ? ELSE started_at END,
  finished_at = '', error_summary = ''
WHERE id = ? AND status IN ('queued', 'paused')`, nowISO(), taskID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) SetTaskFinished(taskID int64, status, errorSummary string) error {
	_, err := s.DB.Exec(`
UPDATE grading_tasks SET status = ?, error_summary = ?, finished_at = ?
WHERE id = ? AND status NOT IN ('completed', 'failed', 'canceled')`,
		status, errorSummary, nowISO(), taskID)
	return err
}

func (s *Store) SetTaskPaused(taskID int64, reason string) error {
	_, err := s.DB.Exec(`
UPDATE grading_tasks SET status = 'paused', error_summary = ? WHERE id = ?`, reason, taskID)
	return err
}

func (s *Store) PrepareTaskResume(taskID int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
UPDATE grading_tasks SET status = 'queued', error_summary = '', finished_at = ''
WHERE id = ? AND status IN ('paused', 'failed')`, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE grading_task_items SET status = 'queued', started_at = '', error_msg = ''
WHERE task_id = ? AND status IN ('running', 'failed')`, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE submissions SET status = 'queued', error_msg = '', updated_at = ?
WHERE id IN (SELECT submission_id FROM grading_task_items WHERE task_id = ? AND status = 'queued')
  AND status IN ('pending', 'error', 'canceled')`, nowISO(), taskID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CancelTask(taskID int64) error {
	now := nowISO()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
UPDATE grading_tasks SET status = 'canceled', finished_at = ?, error_summary = '用户取消'
WHERE id = ? AND status IN ('queued', 'running', 'paused')`, now, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE grading_task_items SET status = 'canceled', finished_at = ?, error_msg = '已取消'
WHERE task_id = ? AND status IN ('queued', 'running')`, now, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE submissions SET status = 'canceled', error_msg = '批改已取消', updated_at = ?
WHERE status IN ('queued', 'grading') AND id IN (
  SELECT submission_id FROM grading_task_items WHERE task_id = ?
)`, now, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListSubmissionStatuses is the light polling payload: no raw text or full
// grading result, just what the queue/progress UI needs.
func (s *Store) ListSubmissionStatuses(psetID int64) ([]models.Submission, error) {
	rows, err := s.DB.Query(`
SELECT id, problem_set_id, student_name, student_code, page_range, status, ai_score, final_score,
       teacher_notes, error_msg, created_at, updated_at
FROM submissions WHERE problem_set_id = ? ORDER BY id ASC`, psetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Submission{}
	for rows.Next() {
		var sub models.Submission
		var aiSc, finSc sql.NullInt64
		if err := rows.Scan(&sub.ID, &sub.ProblemSetID, &sub.StudentName, &sub.StudentCode, &sub.PageRange,
			&sub.Status, &aiSc, &finSc, &sub.TeacherNotes, &sub.ErrorMsg, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		if aiSc.Valid {
			v := int(aiSc.Int64)
			sub.AIScore = &v
		}
		if finSc.Valid {
			v := int(finSc.Int64)
			sub.FinalScore = &v
		}
		out = append(out, sub)
	}
	return out, nil
}
