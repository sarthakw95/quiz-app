package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-app/internal/quiz"
)

func TestParseIntParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	if got, err := parseIntParam(req, "question_count", 10); err != nil || got != 10 {
		t.Fatalf("default parseIntParam = (%d, %v), want (10, nil)", got, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/questions?question_count=25", nil)
	if got, err := parseIntParam(req, "question_count", 10); err != nil || got != 25 {
		t.Fatalf("valid parseIntParam = (%d, %v), want (25, nil)", got, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/questions?question_count=0", nil)
	if _, err := parseIntParam(req, "question_count", 10); err == nil {
		t.Fatalf("expected error for non-positive question_count")
	}
}

func TestParseLeaderboardLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	if got, err := parseLeaderboardLimit(req, 10); err != nil || got != 10 {
		t.Fatalf("default parseLeaderboardLimit = (%d, %v), want (10, nil)", got, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/leaderboard?limit=-1", nil)
	if got, err := parseLeaderboardLimit(req, 10); err != nil || got != -1 {
		t.Fatalf("negative parseLeaderboardLimit = (%d, %v), want (-1, nil)", got, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/leaderboard?limit=abc", nil)
	if _, err := parseLeaderboardLimit(req, 10); err == nil {
		t.Fatalf("expected integer validation error")
	}
}

func TestParseBoolParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/questions?create_if_missing=true", nil)
	if !parseBoolParam(req, "create_if_missing") {
		t.Fatalf("expected true for create_if_missing=true")
	}

	req = httptest.NewRequest(http.MethodGet, "/questions?create_if_missing=yes", nil)
	if !parseBoolParam(req, "create_if_missing") {
		t.Fatalf("expected true for create_if_missing=yes")
	}

	req = httptest.NewRequest(http.MethodGet, "/questions?create_if_missing=0", nil)
	if parseBoolParam(req, "create_if_missing") {
		t.Fatalf("expected false for create_if_missing=0")
	}
}

func TestToQuestionResponsesAddsAttemptMetadata(t *testing.T) {
	questions := []quiz.Question{
		{
			PublicQuestion: quiz.PublicQuestion{
				QuestionID: "q1",
				Question:   "Q1",
				Options:    []quiz.Option{{Letter: "A", Text: "A1"}},
			},
			CorrectIndex: 0,
		},
		{
			PublicQuestion: quiz.PublicQuestion{
				QuestionID: "q2",
				Question:   "Q2",
				Options:    []quiz.Option{{Letter: "A", Text: "A2"}},
			},
			CorrectIndex: 0,
		},
	}

	got := toQuestionResponses(questions, map[string]float64{"q1": 0.0})
	if len(got) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(got))
	}

	if got[0].AttemptStatus != "already_attempted" || got[0].AttemptScore == nil || *got[0].AttemptScore != 0.0 {
		t.Fatalf("unexpected attempted mapping for q1: %+v", got[0])
	}
	if got[1].AttemptStatus != "not_attempted" || got[1].AttemptScore != nil {
		t.Fatalf("unexpected mapping for unattempted q2: %+v", got[1])
	}
}

func TestWriteMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	writeMethodNotAllowed(rec, http.MethodPost)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("allow header = %q, want %q", got, http.MethodPost)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error != "method not allowed" {
		t.Fatalf("error payload = %q", payload.Error)
	}
}

func TestHandleQuestionsServiceUnavailable(t *testing.T) {
	api := NewAPI(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	rec := httptest.NewRecorder()

	api.HandleQuestions(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "quiz service unavailable") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestHandleResponsesWithoutQuizOrUsernameAddsWarning(t *testing.T) {
	bank := quiz.NewBank()
	bank.AddBuiltQuestions([]quiz.Question{
		{
			PublicQuestion: quiz.PublicQuestion{
				QuestionID: "q1",
				Question:   "Q1",
				Options: []quiz.Option{
					{Letter: "A", Text: "Correct"},
					{Letter: "B", Text: "Wrong"},
				},
			},
			CorrectIndex: 0,
		},
	})
	api := NewAPI(nil, bank)

	body := bytes.NewBufferString(`{"responses":[{"question_id":"q1","answer":"A"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/responses", body)
	rec := httptest.NewRecorder()

	api.HandleResponses(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload responsesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Results) != 1 || payload.Results[0].Status != quiz.StatusCorrect {
		t.Fatalf("unexpected results payload: %+v", payload.Results)
	}
	if len(payload.Warnings) != 1 {
		t.Fatalf("expected warning for non-leaderboard submission, got %+v", payload.Warnings)
	}
}
