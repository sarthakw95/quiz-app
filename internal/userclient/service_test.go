package userclient

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"quiz-app/internal/quiz"
)

func float64Pointer(v float64) *float64 {
	return &v
}

func TestParsePositiveLimit(t *testing.T) {
	if got, err := parsePositiveLimit([]string{"quizzes"}, 1, 10); err != nil || got != 10 {
		t.Fatalf("default parsePositiveLimit = (%d, %v), want (10, nil)", got, err)
	}
	if got, err := parsePositiveLimit([]string{"quizzes", "3"}, 1, 10); err != nil || got != 3 {
		t.Fatalf("valid parsePositiveLimit = (%d, %v), want (3, nil)", got, err)
	}
	if _, err := parsePositiveLimit([]string{"quizzes", "0"}, 1, 10); err == nil {
		t.Fatalf("expected validation error for non-positive limit")
	}
}

func TestParseSignedLimit(t *testing.T) {
	if got, err := parseSignedLimit([]string{"leaderboard", "quiz-1"}, 2, 10); err != nil || got != 10 {
		t.Fatalf("default parseSignedLimit = (%d, %v), want (10, nil)", got, err)
	}
	if got, err := parseSignedLimit([]string{"leaderboard", "quiz-1", "-1"}, 2, 10); err != nil || got != -1 {
		t.Fatalf("negative parseSignedLimit = (%d, %v), want (-1, nil)", got, err)
	}
	if _, err := parseSignedLimit([]string{"leaderboard", "quiz-1", "abc"}, 2, 10); err == nil {
		t.Fatalf("expected parse error for non-integer limit")
	}
}

func TestPromptAnswer(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(" b \n"))
	var out bytes.Buffer

	answer, ok := promptAnswer(reader, &out, 2)
	if !ok || answer != "B" {
		t.Fatalf("promptAnswer valid = (%q, %t), want (B, true)", answer, ok)
	}

	reader = bufio.NewReader(strings.NewReader("z\n"))
	answer, ok = promptAnswer(reader, &out, 2)
	if ok || answer != "" {
		t.Fatalf("promptAnswer invalid = (%q, %t), want (\"\", false)", answer, ok)
	}
}

func TestPromptYesNoRetriesUntilValid(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("maybe\nyes\n"))
	var out bytes.Buffer

	ok, err := promptYesNo(reader, &out, "continue? ")
	if err != nil {
		t.Fatalf("promptYesNo returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected yes result")
	}
	if !strings.Contains(out.String(), "Please answer yes or no.") {
		t.Fatalf("expected retry hint in output, got: %s", out.String())
	}
}

func TestRunPlayWithPayloadAllAttemptedPrintsScore(t *testing.T) {
	payload := questionsResponse{
		QuizID: "quiz-1",
		Questions: []questionItem{
			{QuestionID: "q1", AttemptStatus: attemptStatusAlreadyAttempt, AttemptScore: float64Pointer(1.0)},
			{QuestionID: "q2", AttemptScore: float64Pointer(0.0)},
		},
	}

	reader := bufio.NewReader(strings.NewReader(""))
	var out bytes.Buffer
	err := runPlayWithPayload(reader, &out, nil, "alice", payload, 3)
	if err != nil {
		t.Fatalf("runPlayWithPayload failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "quiz quiz-1 is already attempted.") {
		t.Fatalf("expected already attempted message, got: %s", text)
	}
	if !strings.Contains(text, "Score: 1/2") {
		t.Fatalf("expected combined historical score, got: %s", text)
	}
}

func TestRunPlayWithPayloadCombinesOldAndNewScore(t *testing.T) {
	persisted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/responses" && r.Method == http.MethodPost {
			select {
			case persisted <- struct{}{}:
			default:
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, server.Client())
	payload := questionsResponse{
		QuizID: "quiz-1",
		Questions: []questionItem{
			{QuestionID: "q-old", AttemptStatus: attemptStatusAlreadyAttempt, AttemptScore: float64Pointer(1.0)},
			{
				QuestionID:   "q-new",
				Question:     "2 + 2?",
				CorrectIndex: 0,
				Options: []quiz.Option{
					{Letter: "A", Text: "4"},
					{Letter: "B", Text: "5"},
				},
			},
		},
	}

	reader := bufio.NewReader(strings.NewReader("A\n"))
	var out bytes.Buffer
	err := runPlayWithPayload(reader, &out, client, "alice", payload, 3)
	if err != nil {
		t.Fatalf("runPlayWithPayload failed: %v", err)
	}

	select {
	case <-persisted:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected async persistence call to /responses")
	}

	text := out.String()
	if !strings.Contains(text, "Correct!") {
		t.Fatalf("expected correctness feedback, got: %s", text)
	}
	if !strings.Contains(text, "Score: 2/2") {
		t.Fatalf("expected combined score output, got: %s", text)
	}
}
