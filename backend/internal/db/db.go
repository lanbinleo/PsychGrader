package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"psychgrader/internal/models"
)

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
	_, err := s.DB.Exec(`UPDATE submissions SET status = ?, error_msg = ?, updated_at = ? WHERE id = ?`,
		status, errMsg, nowISO(), id)
	return err
}

func (s *Store) SaveGradingResult(id int64, res *models.GradingResult) error {
	raw, err := json.Marshal(res)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
UPDATE submissions
SET status = CASE WHEN status = 'reviewed' THEN 'reviewed' ELSE 'ai_graded' END,
    ai_score = ?,
    result_json = ?,
    error_msg = '',
    updated_at = ?
WHERE id = ?`, res.AITotalScore, string(raw), nowISO(), id)
	return err
}

func (s *Store) SaveTeacherReview(id int64, finalScore int, teacherNotes string, updatedParts []models.RubricPartScore) error {
	sub, err := s.GetSubmission(id)
	if err != nil {
		return err
	}
	resJSON := ""
	if sub.Result != nil {
		if len(updatedParts) > 0 {
			sub.Result.RubricParts = updatedParts
		}
		b, _ := json.Marshal(sub.Result)
		resJSON = string(b)
	}
	_, err = s.DB.Exec(`
UPDATE submissions
SET status = 'reviewed',
    final_score = ?,
    teacher_notes = ?,
    result_json = CASE WHEN ? != '' THEN ? ELSE result_json END,
    updated_at = ?
WHERE id = ?`, finalScore, teacherNotes, resJSON, resJSON, nowISO(), id)
	return err
}

func (s *Store) DeleteSubmission(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM submissions WHERE id = ?`, id)
	return err
}
