package models

type SourceDoc struct {
	ID      string `json:"id"`      // e.g. "Source 1"
	Label   string `json:"label"`   // e.g. "Source 1"
	Title   string `json:"title"`   // Study citation / short title
	Summary string `json:"summary"` // Full stimulus text of the study
}

type ProblemSet struct {
	ID                 int64       `json:"id"`
	Title              string      `json:"title"`
	QuestionType       string      `json:"question_type"` // "EBQ" or "AAQ"
	PromptText         string      `json:"prompt_text"`
	Sources            []SourceDoc `json:"sources"`
	IneligibleConcepts []string    `json:"ineligible_concepts"`
	ScoringNotes       string      `json:"scoring_notes"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`

	// Aggregated stats populated in list/detail queries
	TotalCount    int     `json:"total_count"`
	GradedCount   int     `json:"graded_count"`
	ReviewedCount int     `json:"reviewed_count"`
	AvgScore      float64 `json:"avg_score"`
}

type SentencePair struct {
	ID string `json:"id"` // "S1", "S2", ...
	En string `json:"en"`
	Zh string `json:"zh"`
}

type ConceptCheck struct {
	Term               string `json:"term"`
	CanonicalTerm      string `json:"canonical_term"`
	CEDUnit            string `json:"ced_unit"`
	InAPCED            bool   `json:"in_ap_ced"`
	IsIneligible       bool   `json:"is_ineligible"`
	AppliedProperly    bool   `json:"applied_properly"`
	DuplicateWithPartB bool   `json:"duplicate_with_part_b"`
	NoteZh             string `json:"note_zh"`
}

type Annotation struct {
	ID           string        `json:"id"`
	SentenceID   string        `json:"sentence_id"`
	ExactQuote   string        `json:"exact_quote"`
	Category     string        `json:"category"` // "claim" | "evidence" | "explanation" | "concept" | "issue"
	PartID       string        `json:"part_id"`  // "part_a", "part_b_i", "part_b_ii", "part_c_i", "part_c_ii"
	Badge        string        `json:"badge"`
	CommentZh    string        `json:"comment_zh"`
	Flags        []string      `json:"flags,omitempty"`
	ConceptCheck *ConceptCheck `json:"concept_check,omitempty"`
}

type RubricPartScore struct {
	PartID         string   `json:"part_id"`
	Label          string   `json:"label"`
	Subtitle       string   `json:"subtitle"`
	MaxScore       int      `json:"max_score"`
	AIScore        int      `json:"ai_score"`
	TeacherScore   *int     `json:"teacher_score"`
	CitedSource    string   `json:"cited_source,omitempty"`
	AppliedConcept string   `json:"applied_concept,omitempty"`
	ReasonZh       string   `json:"reason_zh"`
	EvidenceQuote  string   `json:"evidence_quote,omitempty"`
	Flags          []string `json:"flags,omitempty"`
	TeacherComment string   `json:"teacher_comment,omitempty"`
}

type GradingResult struct {
	Sentences      []SentencePair    `json:"sentences"`
	Annotations    []Annotation      `json:"annotations"`
	RubricParts    []RubricPartScore `json:"rubric_parts"`
	AITotalScore   int               `json:"ai_total_score"`
	MaxTotalScore  int               `json:"max_total_score"`
	SummaryZh      string            `json:"summary_zh"`
	GraderMode     string            `json:"grader_mode"` // "deepseek" | "calibration_seed" | "heuristic_fallback"
	ModelUsed      string            `json:"model_used"`
	ReasoningTrace string            `json:"reasoning_trace,omitempty"`
}

type Submission struct {
	ID           int64          `json:"id"`
	ProblemSetID int64          `json:"problem_set_id"`
	StudentName  string         `json:"student_name"`
	StudentCode  string         `json:"student_code"`
	PageRange    string         `json:"page_range"`
	RawText      string         `json:"raw_text"`
	Status       string         `json:"status"` // "pending" | "queued" | "grading" | "ai_graded" | "needs_rereview" | "reviewed" | "error" | "canceled"
	AIScore      *int           `json:"ai_score"`
	FinalScore   *int           `json:"final_score"`
	Result       *GradingResult `json:"result,omitempty"`
	TeacherNotes string         `json:"teacher_notes"`
	ErrorMsg     string         `json:"error_msg,omitempty"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

type SplitSegment struct {
	Index       int    `json:"index"`
	StudentName string `json:"student_name"`
	StudentCode string `json:"student_code"`
	PageRange   string `json:"page_range"`
	RawText     string `json:"raw_text"`
	WordCount   int    `json:"word_count"`
}

// GradingTask is a persistent batch-grading job. The UI polls this object
// instead of inferring progress from submission statuses.
type GradingTask struct {
	ID           int64      `json:"id"`
	ProblemSetID int64      `json:"problem_set_id"`
	Status       string     `json:"status"` // "queued" | "running" | "paused" | "completed" | "failed" | "canceled"
	Total        int        `json:"total"`
	Succeeded    int        `json:"succeeded"`
	Failed       int        `json:"failed"`
	Canceled     int        `json:"canceled"`
	Source       string     `json:"source"`
	ErrorSummary string     `json:"error_summary,omitempty"`
	Current      []TaskItem `json:"current"`
	CreatedAt    string     `json:"created_at"`
	StartedAt    string     `json:"started_at,omitempty"`
	FinishedAt   string     `json:"finished_at,omitempty"`
}

// TaskItem is the per-submission state inside a GradingTask.
type TaskItem struct {
	TaskID       int64  `json:"task_id"`
	SubmissionID int64  `json:"submission_id"`
	StudentName  string `json:"student_name"`
	Status       string `json:"status"` // "queued" | "running" | "succeeded" | "failed" | "canceled" | "superseded"
	ErrorMsg     string `json:"error_msg,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

type AppSettings struct {
	APIBaseURL      string `json:"api_base_url"`
	APIKey          string `json:"api_key,omitempty"`
	APIKeyMasked    string `json:"api_key_masked"`
	HasAPIKey       bool   `json:"has_api_key"`
	Model           string `json:"model"`
	ThinkingEnabled bool   `json:"thinking_enabled"`
	ReasoningEffort string `json:"reasoning_effort"`
	Concurrency     int    `json:"concurrency"`
	AuthEnabled     bool   `json:"auth_enabled"`
}
