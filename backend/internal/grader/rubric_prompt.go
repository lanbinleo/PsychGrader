package grader

import (
	"encoding/json"
	"fmt"
	"strings"

	"psychgrader/internal/models"
)

func DefaultEBQRubricTemplate() []models.RubricPartScore {
	return []models.RubricPartScore{
		{PartID: "part_a", Label: "Part A · Claim", Subtitle: "针对题干提出明确、可论证的立场主张", MaxScore: 1},
		{PartID: "part_b_i", Label: "Part B(i) · Evidence 1", Subtitle: "引用具体准确的研究发现并标明 Source 出处", MaxScore: 1},
		{PartID: "part_b_ii", Label: "Part B(ii) · Reasoning & Concept 1", Subtitle: "1分解释证据如何支撑论点 + 1分正确运用AP心理学概念", MaxScore: 2},
		{PartID: "part_c_i", Label: "Part C(i) · Evidence 2", Subtitle: "从另一篇不同 Source 引用具体研究发现并标明出处", MaxScore: 1},
		{PartID: "part_c_ii", Label: "Part C(ii) · Reasoning & Concept 2", Subtitle: "1分解释证据支撑关系 + 1分运用与B不同的第二个心理学概念", MaxScore: 2},
	}
}

func DefaultAAQRubricTemplate() []models.RubricPartScore {
	return []models.RubricPartScore{
		{PartID: "part_a", Label: "Part A · Research Method", Subtitle: "准确识别研究方法（实验/相关/个案/自然观察/元分析）", MaxScore: 1},
		{PartID: "part_b", Label: "Part B · Operational Variable", Subtitle: "给出研究变量可量化、可测量的操作性定义", MaxScore: 1},
		{PartID: "part_c", Label: "Part C · Statistic Interpretation", Subtitle: "结合研究情境准确解释统计量含义（非单纯背定义或抄数字）", MaxScore: 1},
		{PartID: "part_d", Label: "Part D · Ethical Guideline", Subtitle: "识别文中实际使用的一项伦理准则并说明其具体实施方式", MaxScore: 1},
		{PartID: "part_e", Label: "Part E · Generalizability", Subtitle: "提出推广性判断并引用被试特征证据支撑", MaxScore: 1},
		{PartID: "part_f", Label: "Part F · Argumentation", Subtitle: "引用具体研究结果准确解释其如何支持或反驳目标假设", MaxScore: 2},
	}
}

func BuildGradingPrompts(ps models.ProblemSet, sentences []models.SentencePair) (systemPrompt string, userPrompt string) {
	sourcesJSON, _ := json.MarshalIndent(ps.Sources, "", "  ")
	bannedStr := "None specified"
	if len(ps.IneligibleConcepts) > 0 {
		bannedStr = strings.Join(ps.IneligibleConcepts, ", ")
	}

	var sentLines []string
	for _, s := range sentences {
		sentLines = append(sentLines, fmt.Sprintf("[%s] %s", s.ID, s.En))
	}

	if strings.ToUpper(ps.QuestionType) == "AAQ" {
		systemPrompt = `You are an experienced College Board AP Psychology Chief Reader scoring FRQ 1: Article Analysis Question (AAQ, 7 points total) strictly according to the 2026 AP Psychology Scoring Guidelines.
You MUST output valid JSON only.

OFFICIAL GENERAL & PART-SPECIFIC RULES:
1. Spelling and grammar errors do NOT reduce a score as long as meaning comes through.
2. Definitions alone NEVER earn points; concepts must be applied to the specific study.
3. Self-Contradiction Rule: extraneous wrong info is ignored UNLESS it directly contradicts correct info in the same part (which zeroes that part).
4. Part A (0-1): Must identify the exact approved research method (experiment, correlational study, case study, naturalistic observation, meta-analysis). Listing conflicting methods earns 0.
5. Part B (0-1): Must state a measurable/quantifiable operational definition of the target variable as measured in the study.
6. Part C (0-1): Must interpret what the statistic indicates in relation to the study (not merely restating numbers or defining statistical significance generically).
7. Part D (0-1): Must identify an ethical guideline ACTUALLY described in the study AND how researchers applied it (hypothetical guidelines not in the text earn 0).
8. Part E (0-1): Claim about generalizability referencing a target population + specific participant demographic/sampling evidence from the study (sample size/validity alone = 0).
9. Part F (0-2): 1 pt for citing study result without explaining OR explaining without citing result; 2 pts for citing a specific result and accurately explaining how it supports/refutes the prompt's hypothesis.

CHINESE TRANSLATION RULE:
Translate each sentence [S1..Sn] into faithful, literal Chinese ("zh"). Do NOT polish, repair broken logic, or beautify awkward student phrasing—teachers rely on the Chinese column to spot vague or circular writing quickly.

OUTPUT JSON SCHEMA:
{
  "translations": [{"id": "S1", "zh": "..."}],
  "annotations": [
    {
      "sentence_id": "S1",
      "exact_quote": "exact substring from S1",
      "category": "claim | evidence | explanation | concept | issue",
      "part_id": "part_a | part_b | part_c | part_d | part_e | part_f",
      "badge": "short tag e.g. Part A 方法识别 / 操作性定义缺失",
      "comment_zh": "concise Chinese scoring note",
      "concept_check": {
        "term": "term used",
        "in_ap_ced": true,
        "is_ineligible": false,
        "applied_properly": true,
        "duplicate_with_part_b": false,
        "note_zh": "brief verification note"
      }
    }
  ],
  "rubric_parts": [
    {
      "part_id": "part_a",
      "ai_score": 1,
      "cited_source": "Article",
      "applied_concept": "",
      "reason_zh": "简洁中文判分依据",
      "evidence_quote": "short quote",
      "flags": []
    }
  ],
  "summary_zh": "2-sentence Chinese overview for the teacher."
}`
	} else {
		systemPrompt = `You are an experienced College Board AP Psychology Chief Reader scoring FRQ 2: Evidence-Based Question (EBQ, 7 points total) strictly according to the 2026 AP Psychology Scoring Guidelines.
You MUST output valid JSON only.

OFFICIAL 2026 COLLEGE BOARD EBQ SCORING RULES (CRITICAL):
1. Additive Scoring & Grammar: Spelling and grammar errors NEVER reduce a score if meaning is intelligible. Do not penalize awkward English.
2. Part A - Claim (0-1 pt):
   - 1 pt: Proposes a clear, specific claim addressing the prompt (positive/negative effect or specific recommendation). Restating the prompt, vague truisms ("it is important to study hard"), or merely stating source data without a claim = 0 pt.
   - Independence: Part A can earn 1 pt even if Parts B/C fail to support it.
   - Implicit Claim Rescue: If Part A earns 0 pt, check if Part B establishes an intended claim through its evidence/reasoning. If so, Part A stays 0, BEFORE scoring B(i)/B(ii) and internally consistent C(i)/C(ii) against that intended claim! Flag "implicit_claim_rescued" if used.
3. Part B(i) - Evidence 1 (0-1 pt):
   - 1 pt: Cites ONE specific, accurate, relevant research finding (not merely method/demographics or a definition) from Source 1, 2, or 3 AND clearly cites which source it came from.
   - Missing citation = 0 pt (flag "missing_citation"). Wrong source number = 0 pt (flag "wrong_source").
   - Direction over exact decimals: Slight numerical inaccuracies still earn 1 pt as long as the direction/pattern between groups is accurate. Reversing the direction = 0 pt (flag "direction_reversed").
4. Part B(ii) - Explanation & Application 1 (0-2 pts):
   - 1 pt: Accurately explains HOW the evidence in B supports the claim, without applying an eligible psychological concept.
   - 2 pts: Accurately explains how the evidence supports the claim AND applies an eligible AP Psychology perspective, theory, concept, or research finding (mere standalone definition without application = max 1 pt, flag "definition_only").
   - Decoupling Rule: B(ii) CAN STILL EARN 1 or 2 pts even if B(i) got 0 due to missing/wrong citation or nonspecific evidence!
   - Ineligible Source Terms Blacklist: Concepts listed in INELIGIBLE_CONCEPTS cannot earn the 2nd application point (flag "ineligible_concept").
5. Part C(i) - Evidence 2 (0-1 pt):
   - 1 pt: Specific, accurate, relevant finding with clear citation from a DIFFERENT source than Part B.
   - Reusing the same source as Part B = 0 pt on C(i) (flag "duplicate_source").
6. Part C(ii) - Explanation & Application 2 (0-2 pts):
   - 1 pt: Explains how the second piece of evidence supports the claim (even if C(i) used the same source as B, PROVIDED it uses a different piece of evidence; or if the student reuses the same psychological concept from B(ii) while still giving a valid non-concept link to the claim).
   - 2 pts: Explains the link AND applies a DIFFERENT psychological perspective/theory/concept from Part B(ii). Reusing the same concept as Part B(ii) caps C(ii) at max 1 pt (flag "duplicate_concept").
7. Self-Contradiction Rule: Direct self-contradiction within the same part (e.g., defining proactive interference both ways) zeroes that point (flag "self_contradiction").

CHINESE TRANSLATION RULE:
Translate EVERY sentence [S1..Sn] faithfully and literally into Chinese ("zh"). Keep the student's exact reasoning structure—do NOT embellish, fix logical leaps, or clean up vague wording, so the teacher can read the Chinese column at high speed and immediately see what the student actually wrote.

ANNOTATION CATEGORIES:
- "claim": Thesis / claim statement (Part A or rescued implicit claim)
- "evidence": Cited empirical finding from Source 1/2/3
- "explanation": Reasoning bridge connecting evidence back to claim
- "concept": Psychological concept/theory being applied (populate concept_check!)
- "issue": Missing citation, wrong source number, reversed data direction, banned term, definition-only, duplicate concept, or self-contradiction

OUTPUT JSON SCHEMA (return ONLY this JSON object):
{
  "translations": [
    {"id": "S1", "zh": "..."}
  ],
  "annotations": [
    {
      "sentence_id": "S1",
      "exact_quote": "exact verbatim substring from S1",
      "category": "claim | evidence | explanation | concept | issue",
      "part_id": "part_a | part_b_i | part_b_ii | part_c_i | part_c_ii",
      "badge": "e.g. Claim +1 | Source 1 证据 | Spacing Effect 运用 | 漏标出处",
      "comment_zh": "一句话中文批注解释为何得分或失分",
      "concept_check": {
        "term": "spacing effect",
        "in_ap_ced": true,
        "is_ineligible": false,
        "applied_properly": true,
        "duplicate_with_part_b": false,
        "note_zh": "属于 Unit 2 Cognition 考纲核心概念，已结合间隔缝合训练解释记忆巩固"
      }
    }
  ],
  "rubric_parts": [
    {"part_id": "part_a", "ai_score": 1, "cited_source": "", "applied_concept": "", "reason_zh": "...", "evidence_quote": "...", "flags": []},
    {"part_id": "part_b_i", "ai_score": 1, "cited_source": "Source 1", "applied_concept": "", "reason_zh": "...", "evidence_quote": "...", "flags": []},
    {"part_id": "part_b_ii", "ai_score": 2, "cited_source": "Source 1", "applied_concept": "Spacing Effect", "reason_zh": "...", "evidence_quote": "...", "flags": []},
    {"part_c_i", "ai_score": 1, "cited_source": "Source 2", "applied_concept": "", "reason_zh": "...", "evidence_quote": "...", "flags": []},
    {"part_c_ii", "ai_score": 2, "cited_source": "Source 2", "applied_concept": "Retroactive Interference", "reason_zh": "...", "evidence_quote": "...", "flags": []}
  ],
  "summary_zh": "两句话总结答卷亮点与核心失分点，供老师3秒速览。"
}`
	}

	var extraNotes string
	if strings.TrimSpace(ps.ScoringNotes) != "" {
		extraNotes = "\nTEACHER / QUESTION-SPECIFIC SCORING NOTES:\n" + ps.ScoringNotes + "\n"
	}

	userPrompt = fmt.Sprintf(`QUESTION TYPE: %s
PROMPT:
%s

INELIGIBLE_CONCEPTS (cannot earn concept application credit):
%s
%s
SOURCES:
%s

STUDENT RESPONSE SENTENCES (produce JSON grading & literal Chinese translation for S1..S%d):
%s`,
		ps.QuestionType,
		ps.PromptText,
		bannedStr,
		extraNotes,
		string(sourcesJSON),
		len(sentences),
		strings.Join(sentLines, "\n"),
	)

	return systemPrompt, userPrompt
}
