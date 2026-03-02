package userclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDoJSONReturnsServiceUnavailable(t *testing.T) {
	client := NewHTTPClient("http://example.test", &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial error")
		}),
	})

	err := client.doJSON(context.Background(), http.MethodGet, "/health", nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("expected ErrServiceUnavailable wrapper, got %v", err)
	}
}

func TestDoJSONReturnsAPIErrorMessageFromBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "bad request payload"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, server.Client())
	err := client.doJSON(context.Background(), http.MethodGet, "/anything", nil, nil)
	if err == nil {
		t.Fatalf("expected API error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if apiErr.Message != "bad request payload" {
		t.Fatalf("message = %q, want %q", apiErr.Message, "bad request payload")
	}
}

func TestGetQuizQuestionsBuildsQueryAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("quiz_id") != "quiz-123" {
			t.Fatalf("quiz_id query = %q", query.Get("quiz_id"))
		}
		if query.Get("create_if_missing") != "true" {
			t.Fatalf("create_if_missing query = %q", query.Get("create_if_missing"))
		}
		if query.Get("question_count") != "5" {
			t.Fatalf("question_count query = %q", query.Get("question_count"))
		}
		if query.Get("username") != "alice" {
			t.Fatalf("username query = %q", query.Get("username"))
		}

		_ = json.NewEncoder(w).Encode(questionsResponse{
			QuizID:        "quiz-123",
			QuestionCount: 1,
			Questions: []questionItem{
				{
					QuestionID:    "q1",
					Question:      "Q?",
					CorrectIndex:  0,
					AttemptStatus: "not_attempted",
				},
			},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, server.Client())
	payload, err := client.GetQuizQuestions(context.Background(), "quiz-123", " alice ", true, 5)
	if err != nil {
		t.Fatalf("GetQuizQuestions failed: %v", err)
	}
	if payload.QuizID != "quiz-123" {
		t.Fatalf("quiz id = %q, want %q", payload.QuizID, "quiz-123")
	}
	if len(payload.Questions) != 1 || payload.Questions[0].QuestionID != "q1" {
		t.Fatalf("unexpected questions payload: %+v", payload.Questions)
	}
}

func TestParseTimeRFC3339(t *testing.T) {
	if _, err := parseTime("2026-03-01T10:20:30Z"); err != nil {
		t.Fatalf("expected RFC3339 time to parse, got %v", err)
	}
}

func TestParseTimeInvalid(t *testing.T) {
	if _, err := parseTime("not-a-time"); err == nil {
		t.Fatalf("expected invalid parse error")
	}
}
