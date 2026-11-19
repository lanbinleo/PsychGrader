package grader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"psychgrader/internal/ced"
	"psychgrader/internal/models"
	"psychgrader/internal/pdfparse"
)

type DeepSeekConfig struct {
	BaseURL         string
	APIKey          string
	Model           string
	ThinkingEnabled bool
	ReasoningEffort string
}

type dsMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type dsThinking struct {
	Type string `json:"type"` // "enabled" or "disabled"
}

type dsRespFormat struct {
	Type string `json:"type"` // "json_object"
}

type dsRequest struct {
	Model           string        `json:"model"`
	Messages        []dsMessage   `json:"messages"`
	Thinking        *dsThinking   `json:"thinking,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	ResponseFormat  *dsRespFormat `json:"response_format,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
	MaxTokens       int           `json:"max_tokens,omitempty"`
	Stream          bool          `json:"stream"`
}

type dsResponse struct {
	Choices []struct {
		FinishReason string    `json:"finish_reason"`
		Message      dsMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type llmParsedOutput struct {
	Translations []struct {
		ID string `json:"id"`
		Zh string `json:"zh"`
	} `json:"translations"`
	Annotations []models.Annotation `json:"annotations"`
	RubricParts []llmRubricPart     `json:"rubric_parts"`
	SummaryZh   string              `json:"summary_zh"`
}

// flexInt tolerates 1, 1.0 and "1" from the model instead of failing the whole
// submission on a non-integer score.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		s = strings.TrimSpace(str)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("invalid score %q: %w", s, err)
	}
	*f = flexInt(int(math.Round(v)))
	return nil
}

type llmRubricPart struct {
	PartID         string   `json:"part_id"`
	Label          string   `json:"label"`
	Subtitle       string   `json:"subtitle"`
	MaxScore       flexInt  `json:"max_score"`
	AIScore        flexInt  `json:"ai_score"`
	CitedSource    string   `json:"cited_source"`
	AppliedConcept string   `json:"applied_concept"`
	ReasonZh       string   `json:"reason_zh"`
	EvidenceQuote  string   `json:"evidence_quote"`
	Flags          []string `json:"flags"`
}

func BuildChatCompletionsURL(baseURL string) string {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if b == "" {
		b = "https://api.deepseek.com"
	}
	if strings.HasSuffix(b, "/chat/completions") {
		return b
	}
	return b + "/chat/completions"
}

func TestConnection(ctx context.Context, cfg DeepSeekConfig) (string, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", fmt.Errorf("API Key 不能为空")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "deepseek-flash"
	}
	temp := 0.0
	reqBody := dsRequest{
		Model:    model,
		Messages: []dsMessage{{Role: "user", Content: "Reply with OK only."}},
		Thinking: &dsThinking{Type: "disabled"},
		Stream:   false,
	}
	if !strings.Contains(model, "reasoner") {
		reqBody.Temperature = &temp
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BuildChatCompletionsURL(cfg.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed dsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("invalid JSON response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty choices from API")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

// GradeSubmission runs DeepSeek V4.1 (deepseek-flash / deepseek-v4-pro) when an API key is set,
// or falls back to the deterministic local rule analyzer when running offline without a key.
func GradeSubmission(ctx context.Context, cfg DeepSeekConfig, ps models.ProblemSet, rawText string) (*models.GradingResult, error) {
	rawSentences := pdfparse.SplitIntoSentences(rawText)
	if len(rawSentences) == 0 {
		return nil, fmt.Errorf("空白答卷，未提取到有效英文句子")
	}
	sentences := make([]models.SentencePair, len(rawSentences))
	for i, s := range rawSentences {
		sentences[i] = models.SentencePair{
			ID: fmt.Sprintf("S%d", i+1),
			En: s,
		}
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		if strings.ToUpper(ps.QuestionType) == "AAQ" {
			return nil, fmt.Errorf("AAQ 评分需要配置 DeepSeek API Key")
		}
		res := HeuristicOfflineGrade(ps, sentences)
		return EnforceOfficialRules(ps, res), nil
	}

	sysPrompt, usrPrompt := BuildGradingPrompts(ps, sentences)
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "deepseek-flash"
	}

	reqBody := dsRequest{
		Model: model,
		Messages: []dsMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: usrPrompt},
		},
		ResponseFormat: &dsRespFormat{Type: "json_object"},
		MaxTokens:      8192,
		Stream:         false,
	}

	// Follow DeepSeek V4.1 API contract: omit temperature in thinking mode.
	if strings.Contains(strings.ToLower(model), "flash") || strings.Contains(strings.ToLower(model), "v4") {
		if cfg.ThinkingEnabled {
			reqBody.Thinking = &dsThinking{Type: "enabled"}
			effort := cfg.ReasoningEffort
			if effort == "" {
				effort = "high"
			}
			reqBody.ReasoningEffort = effort
		} else {
			reqBody.Thinking = &dsThinking{Type: "disabled"}
			zero := 0.1
			reqBody.Temperature = &zero
		}
	} else if !strings.Contains(strings.ToLower(model), "reasoner") {
		zero := 0.1
		reqBody.Temperature = &zero
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	respBytes, err := doChatRequest(ctx, cfg, payload)
	if err != nil {
		return nil, err
	}

	var dsResp dsResponse
	if err := json.Unmarshal(respBytes, &dsResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		return nil, fmt.Errorf("模型未返回候选结果")
	}
	if strings.EqualFold(dsResp.Choices[0].FinishReason, "length") {
		return nil, fmt.Errorf("模型输出被截断（finish_reason=length），请降低 thinking/推理强度后重试")
	}

	content := cleanJSONFence(dsResp.Choices[0].Message.Content)
	var out llmParsedOutput
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("模型输出 JSON 解析失败: %w (raw: %.200s)", err, content)
	}

	incoming := make([]models.RubricPartScore, 0, len(out.RubricParts))
	for _, p := range out.RubricParts {
		incoming = append(incoming, models.RubricPartScore{
			PartID:         strings.TrimSpace(p.PartID),
			MaxScore:       int(p.MaxScore),
			AIScore:        int(p.AIScore),
			CitedSource:    p.CitedSource,
			AppliedConcept: p.AppliedConcept,
			ReasonZh:       p.ReasonZh,
			EvidenceQuote:  p.EvidenceQuote,
			Flags:          p.Flags,
		})
	}
	if missing := missingTemplateParts(ps.QuestionType, incoming); len(missing) > 0 {
		return nil, fmt.Errorf("模型输出缺少采分项：%s，请重试或改用更稳定的模型", strings.Join(missing, ", "))
	}

	zhMap := make(map[string]string, len(out.Translations))
	for _, t := range out.Translations {
		zhMap[t.ID] = t.Zh
	}
	for i := range sentences {
		if zh, ok := zhMap[sentences[i].ID]; ok && strings.TrimSpace(zh) != "" {
			sentences[i].Zh = zh
		} else {
			sentences[i].Zh = "（未返回该句译文）"
		}
	}

	res := &models.GradingResult{
		Sentences:      sentences,
		Annotations:    validateAnnotations(out.Annotations, sentences),
		RubricParts:    mergeWithTemplate(ps.QuestionType, incoming),
		SummaryZh:      out.SummaryZh,
		GraderMode:     "deepseek",
		ModelUsed:      model,
		ReasoningTrace: strings.TrimSpace(dsResp.Choices[0].Message.ReasoningContent),
	}
	return EnforceOfficialRules(ps, res), nil
}

func doChatRequest(ctx context.Context, cfg DeepSeekConfig, payload []byte) ([]byte, error) {
	client := &http.Client{Timeout: 150 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, BuildChatCompletionsURL(cfg.BaseURL), bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("DeepSeek 请求失败: %w", err)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("DeepSeek HTTP %d: %.300s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("DeepSeek HTTP %d: %.300s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return body, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("DeepSeek 请求失败")
	}
	return nil, lastErr
}

func missingTemplateParts(qType string, incoming []models.RubricPartScore) []string {
	tmpl := DefaultEBQRubricTemplate()
	if strings.ToUpper(qType) == "AAQ" {
		tmpl = DefaultAAQRubricTemplate()
	}
	have := make(map[string]bool, len(incoming))
	for _, p := range incoming {
		have[strings.ToLower(strings.TrimSpace(p.PartID))] = true
	}
	var missing []string
	for _, p := range tmpl {
		if !have[p.PartID] {
			missing = append(missing, p.PartID)
		}
	}
	return missing
}

func validateAnnotations(in []models.Annotation, sentences []models.SentencePair) []models.Annotation {
	byID := make(map[string]models.SentencePair, len(sentences))
	for _, s := range sentences {
		byID[s.ID] = s
	}
	out := make([]models.Annotation, 0, len(in))
	for i := range in {
		a := in[i]
		sent, ok := byID[a.SentenceID]
		if !ok {
			continue
		}
		if a.ID == "" {
			a.ID = fmt.Sprintf("A%d", len(out)+1)
		}
		switch a.Category {
		case "claim", "evidence", "explanation", "concept", "issue":
		default:
			a.Category = "issue"
		}
		q := strings.TrimSpace(a.ExactQuote)
		if q != "" && !strings.Contains(strings.ToLower(sent.En), strings.ToLower(q)) {
			a.Flags = appendUnique(a.Flags, "unverified_quote")
		}
		out = append(out, a)
	}
	return out
}

func cleanJSONFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func mergeWithTemplate(qType string, incoming []models.RubricPartScore) []models.RubricPartScore {
	tmpl := DefaultEBQRubricTemplate()
	if strings.ToUpper(qType) == "AAQ" {
		tmpl = DefaultAAQRubricTemplate()
	}
	inMap := make(map[string]models.RubricPartScore)
	for _, item := range incoming {
		inMap[strings.ToLower(strings.TrimSpace(item.PartID))] = item
	}
	for i := range tmpl {
		if got, ok := inMap[tmpl[i].PartID]; ok {
			score := got.AIScore
			if score < 0 {
				score = 0
			}
			if score > tmpl[i].MaxScore {
				score = tmpl[i].MaxScore
			}
			tmpl[i].AIScore = score
			tmpl[i].CitedSource = got.CitedSource
			tmpl[i].AppliedConcept = got.AppliedConcept
			tmpl[i].ReasonZh = got.ReasonZh
			tmpl[i].EvidenceQuote = got.EvidenceQuote
			tmpl[i].Flags = got.Flags
		}
	}
	return tmpl
}

// EnforceOfficialRules cross-verifies deterministic AP Psychology EBQ constraints in Go code:
// - Enriches concept annotations against the local AP Psychology CED catalog & ineligible list
// - Verifies Part C(i) does not earn credit if citing the exact same source as Part B(i)
// - Verifies Part B(ii)/C(ii) cannot earn the 2nd concept point if the concept is on the ineligible list or duplicated
// - Recomputes AITotalScore cleanly
func EnforceOfficialRules(ps models.ProblemSet, res *models.GradingResult) *models.GradingResult {
	for i := range res.Annotations {
		ann := &res.Annotations[i]
		if ann.ID == "" {
			ann.ID = fmt.Sprintf("A%d", i+1)
		}
		if ann.ConceptCheck != nil && ann.ConceptCheck.Term != "" {
			entry, banned := ced.MatchConcept(ann.ConceptCheck.Term, ps.IneligibleConcepts)
			if entry != nil {
				ann.ConceptCheck.CanonicalTerm = entry.Term
				ann.ConceptCheck.CEDUnit = entry.Unit
				ann.ConceptCheck.InAPCED = true
			}
			if banned {
				ann.ConceptCheck.IsIneligible = true
			}
		}
	}

	if strings.ToUpper(ps.QuestionType) != "AAQ" && len(res.RubricParts) == 5 {
		bSrc := normalizeSourceTag(res.RubricParts[1].CitedSource)
		cSrc := normalizeSourceTag(res.RubricParts[3].CitedSource)
		if bSrc != "" && cSrc != "" && bSrc == cSrc && res.RubricParts[3].AIScore > 0 {
			res.RubricParts[3].AIScore = 0
			res.RubricParts[3].Flags = appendUnique(res.RubricParts[3].Flags, "duplicate_source")
			res.RubricParts[3].ReasonZh = "[规则拦截] C(i) 与 B(i) 引用了同一篇文献 (" + res.RubricParts[3].CitedSource + ")，按官方细则 C(i) 不得分。 " + res.RubricParts[3].ReasonZh
		}

		// Check banned concepts on B(ii) and C(ii)
		for _, idx := range []int{2, 4} {
			part := &res.RubricParts[idx]
			if part.AppliedConcept != "" {
				_, banned := ced.MatchConcept(part.AppliedConcept, ps.IneligibleConcepts)
				if banned && part.AIScore == 2 {
					part.AIScore = 1
					part.Flags = appendUnique(part.Flags, "ineligible_concept")
					part.ReasonZh = "[规则拦截] 所用概念 \"" + part.AppliedConcept + "\" 属于本题题干禁用词，扣除第2分概念运用分。 " + part.ReasonZh
				}
			}
		}

		// Check duplicate concept between B(ii) and C(ii)
		bConcept := canonicalConceptKey(res.RubricParts[2].AppliedConcept, ps.IneligibleConcepts)
		cConcept := canonicalConceptKey(res.RubricParts[4].AppliedConcept, ps.IneligibleConcepts)
		if bConcept != "" && cConcept != "" && bConcept == cConcept && res.RubricParts[4].AIScore == 2 {
			res.RubricParts[4].AIScore = 1
			res.RubricParts[4].Flags = appendUnique(res.RubricParts[4].Flags, "duplicate_concept")
			res.RubricParts[4].ReasonZh = "[规则拦截] C(ii) 复用了与 B(ii) 相同的心理学概念 (" + res.RubricParts[4].AppliedConcept + ")，封顶 1 分。 " + res.RubricParts[4].ReasonZh
		}
	}

	total := 0
	maxTotal := 0
	for _, p := range res.RubricParts {
		total += p.AIScore
		maxTotal += p.MaxScore
	}
	res.AITotalScore = total
	res.MaxTotalScore = maxTotal
	return res
}

func normalizeSourceTag(s string) string {
	re := regexp.MustCompile(`(?i)source\s*([123])`)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return "source_" + m[1]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func canonicalConceptKey(raw string, ineligible []string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if entry, _ := ced.MatchConcept(raw, ineligible); entry != nil {
		return strings.ToLower(entry.Term)
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func appendUnique(slice []string, item string) []string {
	for _, v := range slice {
		if v == item {
			return slice
		}
	}
	return append(slice, item)
}

// HeuristicOfflineGrade provides deterministic structural analysis when DEEPSEEK_API_KEY
// is not configured yet, enabling full offline pipeline & UI testing before plugging in live keys.
func HeuristicOfflineGrade(ps models.ProblemSet, sentences []models.SentencePair) *models.GradingResult {
	reSource := regexp.MustCompile(`(?i)\b(?:source|study)\s*([123])\b`)
	parts := DefaultEBQRubricTemplate()
	if strings.ToUpper(ps.QuestionType) == "AAQ" {
		parts = DefaultAAQRubricTemplate()
	}

	var annotations = make([]models.Annotation, 0)
	var foundSources []struct {
		sentID string
		src    string
		quote  string
	}
	var foundConcepts []struct {
		sentID string
		entry  *ced.ConceptEntry
		banned bool
		raw    string
	}

	for i := range sentences {
		en := sentences[i].En
		sentences[i].Zh = "[离线演练模式·配置 DeepSeek Key 后自动生成逐句直译] " + en

		if i == 0 && !strings.HasSuffix(strings.TrimSpace(en), "?") && len(strings.Fields(en)) >= 7 {
			annotations = append(annotations, models.Annotation{
				ID:         fmt.Sprintf("A%d", len(annotations)+1),
				SentenceID: sentences[i].ID,
				ExactQuote: en,
				Category:   "claim",
				PartID:     "part_a",
				Badge:      "Part A · 候选论点",
				CommentZh:  "首句提出明确主张陈述（离线规则识别，接入 DeepSeek 后核验与题干契合度）",
			})
		}

		if matches := reSource.FindAllStringSubmatch(en, -1); len(matches) > 0 {
			srcLabel := "Source " + matches[0][1]
			foundSources = append(foundSources, struct {
				sentID string
				src    string
				quote  string
			}{sentences[i].ID, srcLabel, en})
			pID := "part_b_i"
			if len(foundSources) > 1 {
				pID = "part_c_i"
			}
			annotations = append(annotations, models.Annotation{
				ID:         fmt.Sprintf("A%d", len(annotations)+1),
				SentenceID: sentences[i].ID,
				ExactQuote: en,
				Category:   "evidence",
				PartID:     pID,
				Badge:      srcLabel + " 引用",
				CommentZh:  "检测到明确文献编号引用 " + srcLabel,
			})
		}

		lowerEn := strings.ToLower(en)
		seenConcept := make(map[string]bool)
		for idx := range ced.Catalog {
			c := &ced.Catalog[idx]
			if seenConcept[c.Term] {
				continue
			}
			matchedAlias := ""
			if strings.Contains(lowerEn, strings.ToLower(c.Term)) {
				matchedAlias = c.Term
			} else {
				for _, al := range c.Aliases {
					if len(al) >= 4 && strings.Contains(lowerEn, al) {
						matchedAlias = al
						break
					}
				}
			}
			if matchedAlias != "" {
				seenConcept[c.Term] = true
				_, banned := ced.MatchConcept(matchedAlias, ps.IneligibleConcepts)
				foundConcepts = append(foundConcepts, struct {
					sentID string
					entry  *ced.ConceptEntry
					banned bool
					raw    string
				}{sentences[i].ID, c, banned, matchedAlias})
				cat := "concept"
				if banned {
					cat = "issue"
				}
				annotations = append(annotations, models.Annotation{
					ID:         fmt.Sprintf("A%d", len(annotations)+1),
					SentenceID: sentences[i].ID,
					ExactQuote: matchedAlias,
					Category:   cat,
					PartID:     "part_b_ii",
					Badge:      c.Term,
					CommentZh:  "命中 AP CED 概念：" + c.Term + "（" + c.Unit + "）",
					ConceptCheck: &models.ConceptCheck{
						Term:            matchedAlias,
						CanonicalTerm:   c.Term,
						CEDUnit:         c.Unit,
						InAPCED:         true,
						IsIneligible:    banned,
						AppliedProperly: !banned,
						NoteZh:          c.Definition,
					},
				})
			}
		}
	}

	if strings.ToUpper(ps.QuestionType) != "AAQ" && len(parts) == 5 {
		if len(sentences) > 0 && !strings.HasSuffix(strings.TrimSpace(sentences[0].En), "?") {
			parts[0].AIScore = 1
			parts[0].ReasonZh = "离线规则检测：首段包含针对题干的陈述性主张。"
			parts[0].EvidenceQuote = sentences[0].En
		} else {
			parts[0].AIScore = 0
			parts[0].ReasonZh = "离线规则检测：未在开头检测到明确陈述主张（或仅复述了问句）。"
		}

		if len(foundSources) >= 1 {
			parts[1].AIScore = 1
			parts[1].CitedSource = foundSources[0].src
			parts[1].ReasonZh = "检测到第一处文献引用 (" + foundSources[0].src + ")。"
			parts[1].EvidenceQuote = foundSources[0].quote
		} else {
			parts[1].AIScore = 0
			parts[1].Flags = []string{"missing_citation"}
			parts[1].ReasonZh = "未检测到明确的 (Source 1/2/3) 文献出处标注。"
		}

		if len(foundConcepts) >= 1 {
			parts[2].AIScore = 2
			parts[2].AppliedConcept = foundConcepts[0].entry.Term
			parts[2].ReasonZh = "检测到考纲概念 " + foundConcepts[0].entry.Term + " 并结合证据展开阐述。"
		} else if len(sentences) >= 3 {
			parts[2].AIScore = 1
			parts[2].ReasonZh = "包含对证据的解释句，但未命中明确的 AP 考纲心理学概念。"
		}

		if len(foundSources) >= 2 {
			parts[3].AIScore = 1
			parts[3].CitedSource = foundSources[1].src
			parts[3].ReasonZh = "检测到第二处文献引用 (" + foundSources[1].src + ")。"
			parts[3].EvidenceQuote = foundSources[1].quote
		} else {
			parts[3].AIScore = 0
			parts[3].Flags = []string{"missing_citation"}
			parts[3].ReasonZh = "未检测到第二处独立的文献引用。"
		}

		if len(foundConcepts) >= 2 {
			parts[4].AIScore = 2
			parts[4].AppliedConcept = foundConcepts[1].entry.Term
			parts[4].ReasonZh = "检测到第二个考纲概念 " + foundConcepts[1].entry.Term + "。"
		} else if len(sentences) >= 5 {
			parts[4].AIScore = 1
			parts[4].ReasonZh = "存在第二段解释句，但未检测到第二个不同的考纲概念。"
		}
	}

	return &models.GradingResult{
		Sentences:   sentences,
		Annotations: annotations,
		RubricParts: parts,
		SummaryZh:   "当前为本地规则预检结果（尚未填写 DeepSeek API Key）。填入 Key 后点击「重新 AI 评阅」即可获取 DeepSeek V4.1 Flash 深度判分与逐句中文直译。",
		GraderMode:  "heuristic_fallback",
		ModelUsed:   "local-rule-engine",
	}
}
