package grader

import (
	"testing"

	"psychgrader/internal/models"
	"psychgrader/internal/pdfparse"
)

func TestSentenceSplittingPreservesStats(t *testing.T) {
	essay := `Distributing practice strengthens memory. In Source 1, Dr. Moulton found Group 2 scored higher (M = 31.60, p < .05) than Group 1 (M = 27.63) (Source 1). This reflects the spacing effect.`
	sents := pdfparse.SplitIntoSentences(essay)
	if len(sents) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %#v", len(sents), sents)
	}
}

func TestEnforceOfficialRulesBlocksDuplicateSourceAndConcept(t *testing.T) {
	ps := models.ProblemSet{
		QuestionType:       "EBQ",
		IneligibleConcepts: []string{"retrieval", "mean", "statistical significance"},
	}
	res := &models.GradingResult{
		RubricParts: []models.RubricPartScore{
			{PartID: "part_a", MaxScore: 1, AIScore: 1},
			{PartID: "part_b_i", MaxScore: 1, AIScore: 1, CitedSource: "Source 1"},
			{PartID: "part_b_ii", MaxScore: 2, AIScore: 2, AppliedConcept: "spacing effect"},
			{PartID: "part_c_i", MaxScore: 1, AIScore: 1, CitedSource: "Source 1"},            // duplicate source!
			{PartID: "part_c_ii", MaxScore: 2, AIScore: 2, AppliedConcept: "spaced practice"}, // duplicate canonical concept!
		},
	}
	EnforceOfficialRules(ps, res)

	if res.RubricParts[3].AIScore != 0 {
		t.Fatalf("expected C(i) duplicate source to be zeroed, got %d", res.RubricParts[3].AIScore)
	}
	if res.RubricParts[4].AIScore != 1 {
		t.Fatalf("expected C(ii) duplicate concept to be capped at 1, got %d", res.RubricParts[4].AIScore)
	}
	if res.AITotalScore != 5 {
		t.Fatalf("expected total 5, got %d", res.AITotalScore)
	}
}

func TestEnforceOfficialRulesBlocksIneligibleConcept(t *testing.T) {
	ps := models.ProblemSet{
		QuestionType:       "EBQ",
		IneligibleConcepts: []string{"retrieval", "rehearsal", "statistical significance"},
	}
	res := &models.GradingResult{
		RubricParts: []models.RubricPartScore{
			{PartID: "part_a", MaxScore: 1, AIScore: 1},
			{PartID: "part_b_i", MaxScore: 1, AIScore: 1, CitedSource: "Source 3"},
			{PartID: "part_b_ii", MaxScore: 2, AIScore: 2, AppliedConcept: "testing effect"}, // allowed!
			{PartID: "part_c_i", MaxScore: 1, AIScore: 1, CitedSource: "Source 1"},
			{PartID: "part_c_ii", MaxScore: 2, AIScore: 2, AppliedConcept: "retrieval"}, // banned!
		},
	}
	EnforceOfficialRules(ps, res)

	if res.RubricParts[2].AIScore != 2 {
		t.Fatalf("Testing Effect should remain 2, got %d", res.RubricParts[2].AIScore)
	}
	if res.RubricParts[4].AIScore != 1 {
		t.Fatalf("bare 'retrieval' should be capped at 1, got %d", res.RubricParts[4].AIScore)
	}
}
