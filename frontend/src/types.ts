export interface SourceDoc {
  id: string;
  label: string;
  title: string;
  summary: string;
}

export interface ProblemSet {
  id: number;
  title: string;
  question_type: 'EBQ' | 'AAQ';
  prompt_text: string;
  sources: SourceDoc[];
  ineligible_concepts: string[];
  scoring_notes: string;
  created_at: string;
  updated_at: string;
  total_count: number;
  graded_count: number;
  reviewed_count: number;
  avg_score: number;
}

export interface SentencePair {
  id: string;
  en: string;
  zh: string;
}

export interface ConceptCheck {
  term: string;
  canonical_term: string;
  ced_unit: string;
  in_ap_ced: boolean;
  is_ineligible: boolean;
  applied_properly: boolean;
  duplicate_with_part_b: boolean;
  note_zh: string;
}

export type AnnotationCategory = 'claim' | 'evidence' | 'explanation' | 'concept' | 'issue';

export interface Annotation {
  id: string;
  sentence_id: string;
  exact_quote: string;
  category: AnnotationCategory;
  part_id: string;
  badge: string;
  comment_zh: string;
  concept_check?: ConceptCheck;
}

export interface RubricPartScore {
  part_id: string;
  label: string;
  subtitle: string;
  max_score: number;
  ai_score: number;
  teacher_score?: number | null;
  cited_source?: string;
  applied_concept?: string;
  reason_zh: string;
  evidence_quote?: string;
  flags?: string[];
  teacher_comment?: string;
}

export interface GradingResult {
  sentences: SentencePair[];
  annotations: Annotation[];
  rubric_parts: RubricPartScore[];
  ai_total_score: number;
  max_total_score: number;
  summary_zh: string;
  grader_mode: string;
  model_used: string;
  reasoning_trace?: string;
}

export interface Submission {
  id: number;
  problem_set_id: number;
  student_name: string;
  student_code: string;
  page_range: string;
  raw_text: string;
  status: 'pending' | 'grading' | 'ai_graded' | 'reviewed' | 'error';
  ai_score?: number | null;
  final_score?: number | null;
  result?: GradingResult;
  teacher_notes: string;
  error_msg?: string;
  created_at: string;
  updated_at: string;
}

export interface SplitSegment {
  index: number;
  student_name: string;
  student_code: string;
  page_range: string;
  raw_text: string;
  word_count: number;
}

export interface AppSettings {
  api_base_url: string;
  api_key_masked: string;
  has_api_key: boolean;
  model: string;
  thinking_enabled: boolean;
  reasoning_effort: string;
  concurrency: number;
  auth_enabled: boolean;
}

export interface CEDEntry {
  term: string;
  unit: string;
  aliases?: string[];
  definition: string;
}
