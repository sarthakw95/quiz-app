package quiz

import (
	"strings"
	"testing"

	"quiz-app/internal/opentdb"
)

func TestBuildQuestionsUnescapesAndAssignsID(t *testing.T) {
	raw := []opentdb.RawQuestion{
		{
			Question:         "2 &amp; 2 = ?",
			CorrectAnswer:    "4 &lt; 5",
			IncorrectAnswers: []string{"1", "2", "3"},
		},
	}

	questions := BuildQuestions(raw)
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}

	item := questions[0]
	if item.Question != "2 & 2 = ?" {
		t.Fatalf("question not unescaped, got %q", item.Question)
	}
	if !strings.HasPrefix(item.QuestionID, "q_") || len(item.QuestionID) != 14 {
		t.Fatalf("unexpected question id format: %q", item.QuestionID)
	}
	if item.CorrectIndex < 0 || item.CorrectIndex >= len(item.Options) {
		t.Fatalf("correct index out of range: %d", item.CorrectIndex)
	}

	foundCorrectOption := false
	for _, option := range item.Options {
		if option.Text == "4 < 5" {
			foundCorrectOption = true
			break
		}
	}
	if !foundCorrectOption {
		t.Fatalf("correct option text not found in options: %+v", item.Options)
	}
}

func TestMakeQuestionIDDiffersWhenOptionOrderDiffers(t *testing.T) {
	q1 := Question{
		PublicQuestion: PublicQuestion{
			Question: "Ordering matters",
			Options: []Option{
				{Letter: "A", Text: "One"},
				{Letter: "B", Text: "Two"},
			},
		},
	}
	q2 := Question{
		PublicQuestion: PublicQuestion{
			Question: "Ordering matters",
			Options: []Option{
				{Letter: "A", Text: "Two"},
				{Letter: "B", Text: "One"},
			},
		},
	}

	id1 := MakeQuestionID(q1)
	id2 := MakeQuestionID(q2)
	if id1 == id2 {
		t.Fatalf("expected different IDs for different option ordering, got %q", id1)
	}
}

func TestBankEvaluateResponsesStatuses(t *testing.T) {
	bank := NewBank()
	bank.AddBuiltQuestions([]Question{
		{
			PublicQuestion: PublicQuestion{
				QuestionID: "q1",
				Question:   "Capital of France?",
				Options: []Option{
					{Letter: "A", Text: "Berlin"},
					{Letter: "B", Text: "Paris"},
				},
			},
			CorrectIndex: 1,
		},
	})

	results := bank.EvaluateResponses([]SubmittedResponse{
		{QuestionID: "q1", Answer: "B"},
		{QuestionID: "q1", Answer: "a"},
		{QuestionID: "q1", Answer: "Z"},
		{QuestionID: "missing", Answer: "A"},
		{QuestionID: "q1", Answer: "AB"},
	})

	want := []string{
		StatusCorrect,
		StatusIncorrect,
		StatusInvalidLetter,
		StatusInvalidQuestion,
		StatusInvalidLetter,
	}

	if len(results) != len(want) {
		t.Fatalf("expected %d results, got %d", len(want), len(results))
	}
	for idx := range want {
		if results[idx].Status != want[idx] {
			t.Fatalf("result %d status = %q, want %q", idx, results[idx].Status, want[idx])
		}
	}
}

func TestNormalizeLetter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "trim and uppercase", input: " a ", want: "A"},
		{name: "already uppercase", input: "B", want: "B"},
		{name: "empty", input: "", want: ""},
		{name: "multiple chars", input: "AB", want: ""},
		{name: "whitespace", input: "   ", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeLetter(tc.input); got != tc.want {
				t.Fatalf("NormalizeLetter(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
