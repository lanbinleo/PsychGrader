package pdfparse

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
	"psychgrader/internal/models"
)

const maxPDFPages = 80

// ExtractPages extracts plain text per page (1-indexed) from a PDF byte slice.
// It prefers poppler's pdftotext when available (as in our Docker container) and
// falls back cleanly to the pure-Go ledongthuc/pdf parser.
func ExtractPages(ctx context.Context, data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty PDF file")
	}
	if pages, err := extractWithPdftotext(ctx, data); err == nil && len(pages) > 0 {
		if len(pages) > maxPDFPages {
			return nil, fmt.Errorf("PDF 页数 %d 超过上限 %d", len(pages), maxPDFPages)
		}
		return pages, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("PDF 解析超时或已取消: %w", err)
	}
	return extractPureGo(data)
}

func extractWithPdftotext(ctx context.Context, data []byte) ([]string, error) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "psychgrader-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	tmp.Close()

	outTxt := tmp.Name() + ".txt"
	defer os.Remove(outTxt)
	cmd := exec.CommandContext(ctx, bin, "-layout", tmp.Name(), outTxt)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(outTxt)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(raw), "\f")
	var pages []string
	for _, p := range parts {
		cleaned := cleanExtractedText(p)
		if cleaned != "" || len(pages) < len(parts)-1 {
			pages = append(pages, cleaned)
		}
	}
	return pages, nil
}

func extractPureGo(data []byte) ([]string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}
	total := reader.NumPage()
	if total > maxPDFPages {
		return nil, fmt.Errorf("PDF 页数 %d 超过上限 %d", total, maxPDFPages)
	}
	pages := make([]string, 0, total)
	for i := 1; i <= total; i++ {
		p := reader.Page(i)
		if p.V.IsNull() {
			pages = append(pages, "")
			continue
		}
		txt, err := p.GetPlainText(nil)
		if err != nil {
			// Fall back to row-based extraction if plain text fails on a page
			rows, rErr := p.GetTextByRow()
			if rErr == nil {
				var sb strings.Builder
				for _, row := range rows {
					for _, word := range row.Content {
						sb.WriteString(word.S)
						sb.WriteString(" ")
					}
					sb.WriteString("\n")
				}
				txt = sb.String()
			}
		}
		pages = append(pages, cleanExtractedText(txt))
	}
	return pages, nil
}

func cleanExtractedText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	var out []string
	for _, ln := range lines {
		trimmed := strings.Join(strings.Fields(ln), " ")
		out = append(out, trimmed)
	}
	// Collapse 3+ consecutive blank lines into 2
	joined := strings.TrimSpace(strings.Join(out, "\n"))
	reBlank := regexp.MustCompile(`\n{3,}`)
	return reBlank.ReplaceAllString(joined, "\n\n")
}

var (
	reStudentHeader = regexp.MustCompile(`(?im)^(?:student(?:\s+name)?|name|candidate|examinee)\s*[:：]\s*(.+)$`)
	reAPIDHeader    = regexp.MustCompile(`(?im)^(?:ap\s*id|student\s*id|id)\s*[:：]\s*([A-Za-z0-9_-]+)$`)
	rePageOneOf     = regexp.MustCompile(`(?im)\bpage\s+1\s+of\s+\d+\b`)
)

// SplitPagesToSubmissions groups extracted PDF pages into student submissions.
// mode: "auto" | "pages_per_student"
func SplitPagesToSubmissions(pages []string, mode string, pagesPerStudent int) []models.SplitSegment {
	if pagesPerStudent <= 0 {
		pagesPerStudent = 1
	}
	if mode == "pages_per_student" {
		var segs []models.SplitSegment
		idx := 1
		for i := 0; i < len(pages); i += pagesPerStudent {
			end := i + pagesPerStudent
			if end > len(pages) {
				end = len(pages)
			}
			chunk := strings.TrimSpace(strings.Join(pages[i:end], "\n\n"))
			if chunk == "" {
				continue
			}
			name, code, body := extractHeaderMetadata(chunk, idx)
			pageRange := fmt.Sprintf("P.%d", i+1)
			if end-i > 1 {
				pageRange = fmt.Sprintf("P.%d-%d", i+1, end)
			}
			segs = append(segs, models.SplitSegment{
				Index:       idx,
				StudentName: name,
				StudentCode: code,
				PageRange:   pageRange,
				RawText:     body,
				WordCount:   len(strings.Fields(body)),
			})
			idx++
		}
		return segs
	}

	// Auto mode: merge consecutive pages belonging to the same student header,
	// or split 1 page per student when no repeating student headers are present.
	type bucket struct {
		startPage int
		endPage   int
		name      string
		code      string
		texts     []string
	}
	var buckets []bucket
	anyHeader := false
	for i, p := range pages {
		if strings.TrimSpace(p) == "" {
			continue
		}
		mName := reStudentHeader.FindStringSubmatch(p)
		mID := reAPIDHeader.FindStringSubmatch(p)
		isPageOne := rePageOneOf.MatchString(p)

		name := ""
		if len(mName) > 1 {
			name = strings.TrimSpace(mName[1])
			anyHeader = true
		}
		code := ""
		if len(mID) > 1 {
			code = strings.TrimSpace(mID[1])
			anyHeader = true
		}

		if len(buckets) > 0 && anyHeader {
			prev := &buckets[len(buckets)-1]
			sameStudent := (name != "" && name == prev.name) ||
				(code != "" && code == prev.code) ||
				(name == "" && code == "" && !isPageOne)
			if sameStudent {
				prev.endPage = i + 1
				prev.texts = append(prev.texts, stripMetadataLines(p))
				continue
			}
		}

		buckets = append(buckets, bucket{
			startPage: i + 1,
			endPage:   i + 1,
			name:      name,
			code:      code,
			texts:     []string{stripMetadataLines(p)},
		})
	}

	var segs []models.SplitSegment
	for idx, b := range buckets {
		body := strings.TrimSpace(strings.Join(b.texts, "\n\n"))
		if body == "" {
			continue
		}
		name := b.name
		if name == "" {
			name = fmt.Sprintf("Student #%02d", idx+1)
		}
		pageRange := fmt.Sprintf("P.%d", b.startPage)
		if b.endPage > b.startPage {
			pageRange = fmt.Sprintf("P.%d-%d", b.startPage, b.endPage)
		}
		segs = append(segs, models.SplitSegment{
			Index:       idx + 1,
			StudentName: name,
			StudentCode: b.code,
			PageRange:   pageRange,
			RawText:     body,
			WordCount:   len(strings.Fields(body)),
		})
	}
	return segs
}

func extractHeaderMetadata(chunk string, fallbackIdx int) (name, code, body string) {
	if m := reStudentHeader.FindStringSubmatch(chunk); len(m) > 1 {
		name = strings.TrimSpace(m[1])
	}
	if m := reAPIDHeader.FindStringSubmatch(chunk); len(m) > 1 {
		code = strings.TrimSpace(m[1])
	}
	body = strings.TrimSpace(stripMetadataLines(chunk))
	if name == "" {
		name = fmt.Sprintf("Student #%02d", fallbackIdx)
	}
	return name, code, body
}

func stripMetadataLines(s string) string {
	lines := strings.Split(s, "\n")
	var kept []string
	for i, ln := range lines {
		if i < 5 {
			if reStudentHeader.MatchString(ln) || reAPIDHeader.MatchString(ln) {
				continue
			}
			if matched, _ := regexp.MatchString(`(?i)^page\s+\d+\s+of\s+\d+$`, strings.TrimSpace(ln)); matched {
				continue
			}
		}
		kept = append(kept, ln)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// SplitIntoSentences splits an English student essay into clean sentence units (S1..Sn)
// while avoiding false breaks on abbreviations (Dr., Prof., vs., e.g., i.e., et al.)
// and decimal statistics (M = 31.6, p < .05).
func SplitIntoSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	paras := regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(text), -1)
	var out []string
	abbrevs := map[string]bool{
		"dr.": true, "prof.": true, "mr.": true, "ms.": true, "mrs.": true,
		"vs.": true, "etc.": true, "e.g.": true, "i.e.": true, "fig.": true, "al.": true,
	}

	for _, para := range paras {
		para = strings.Join(strings.Fields(para), " ")
		if para == "" {
			continue
		}
		var cur strings.Builder
		tokens := strings.Split(para, " ")
		for i, tok := range tokens {
			if cur.Len() > 0 {
				cur.WriteByte(' ')
			}
			cur.WriteString(tok)

			lastChar := tok[len(tok)-1]
			isEndPunct := lastChar == '.' || lastChar == '!' || lastChar == '?'
			// Also handle closing quote or parenthesis after period, e.g. `(Source 1).` or `higher."`
			if !isEndPunct && len(tok) >= 2 && (lastChar == '"' || lastChar == '\'' || lastChar == ')') {
				prev := tok[len(tok)-2]
				if prev == '.' || prev == '!' || prev == '?' {
					isEndPunct = true
				}
			}
			if !isEndPunct {
				continue
			}
			lowerTok := strings.ToLower(tok)
			if abbrevs[lowerTok] {
				continue
			}
			// Single uppercase initial like "J."
			if len(tok) == 2 && tok[0] >= 'A' && tok[0] <= 'Z' && tok[1] == '.' {
				continue
			}
			if i == len(tokens)-1 {
				s := strings.TrimSpace(cur.String())
				if s != "" {
					out = append(out, s)
				}
				cur.Reset()
				continue
			}
			// Peek next token: start of new sentence usually begins with uppercase, quote, or '('
			next := tokens[i+1]
			if len(next) > 0 {
				c0 := next[0]
				if (c0 >= 'A' && c0 <= 'Z') || c0 == '"' || c0 == '(' || (c0 >= '0' && c0 <= '9') {
					s := strings.TrimSpace(cur.String())
					if s != "" {
						out = append(out, s)
					}
					cur.Reset()
				}
			}
		}
		if rest := strings.TrimSpace(cur.String()); rest != "" {
			out = append(out, rest)
		}
	}
	return out
}
