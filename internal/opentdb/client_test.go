package opentdb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newTestClient(rt http.RoundTripper) *Client {
	return NewClient(&http.Client{Transport: rt})
}

func TestFetchQuestionsUsesDefaultAmountWhenNonPositive(t *testing.T) {
	var seenAmount string

	client := newTestClient(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seenAmount = r.URL.Query().Get("amount")
		resp := http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"response_code":0,"results":[]}`))),
			Header:     make(http.Header),
		}
		return &resp, nil
	}))

	questions, err := client.FetchQuestions(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchQuestions returned error: %v", err)
	}
	if len(questions) != 0 {
		t.Fatalf("expected no questions, got %d", len(questions))
	}
	if seenAmount != "10" {
		t.Fatalf("expected default amount 10, got %q", seenAmount)
	}
}

func TestFetchQuestionsPropagatesNonOKStatus(t *testing.T) {
	client := newTestClient(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		resp := http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
		}
		return &resp, nil
	}))

	if _, err := client.FetchQuestions(context.Background(), 5); err == nil {
		t.Fatalf("expected error for non-200 status")
	}
}

func TestFetchQuestionsJSONDecodeError(t *testing.T) {
	client := newTestClient(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		resp := http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("not-json"))),
			Header:     make(http.Header),
		}
		return &resp, nil
	}))

	if _, err := client.FetchQuestions(context.Background(), 3); err == nil {
		t.Fatalf("expected JSON decode error")
	}
}

func TestFetchQuestionsNonZeroResponseCode(t *testing.T) {
	client := newTestClient(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		payload := apiResponse{
			ResponseCode: 1,
			Results: []RawQuestion{
				{Question: "ignored"},
			},
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}

		resp := http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(encoded)),
			Header:     make(http.Header),
		}
		return &resp, nil
	}))

	if _, err := client.FetchQuestions(context.Background(), 3); err == nil {
		t.Fatalf("expected error for non-zero response_code")
	}
}
