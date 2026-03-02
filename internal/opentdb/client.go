package opentdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	apiURL           = "https://opentdb.com/api.php"
	defaultAmount    = 10
	maxFetchAttempts = 3
	retryBaseDelay   = 50 * time.Millisecond
	retryMaxDelay    = 200 * time.Millisecond
)

// OpenTriviaDB question payload.
type RawQuestion struct {
	Type             string   `json:"type"`
	Difficulty       string   `json:"difficulty"`
	Category         string   `json:"category"`
	Question         string   `json:"question"`
	CorrectAnswer    string   `json:"correct_answer"`
	IncorrectAnswers []string `json:"incorrect_answers"`
}

type apiResponse struct {
	ResponseCode int           `json:"response_code"`
	Results      []RawQuestion `json:"results"`
}

type Client struct {
	httpClient *http.Client
}

var defaultHTTPClient = &http.Client{
	Timeout: 5 * time.Second,
}

var defaultClient = NewClient(nil)

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = defaultHTTPClient
	}
	return &Client{httpClient: httpClient}
}

func FetchQuestions(ctx context.Context, amount int) ([]RawQuestion, error) {
	return defaultClient.FetchQuestions(ctx, amount)
}

func (c *Client) FetchQuestions(ctx context.Context, amount int) ([]RawQuestion, error) {
	if amount <= 0 {
		amount = defaultAmount
	}

	reqURL := apiURL + "?amount=" + strconv.Itoa(amount)
	delay := retryBaseDelay
	var lastErr error

	for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
		questions, retryable, err := c.fetchQuestionsOnce(ctx, reqURL)
		if err == nil {
			return questions, nil
		}
		lastErr = err

		if !retryable || attempt == maxFetchAttempts {
			break
		}

		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, err
		}

		delay *= 2
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
	}

	return nil, lastErr
}

func (c *Client) fetchQuestionsOnce(ctx context.Context, reqURL string) ([]RawQuestion, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, shouldRetryStatus(resp.StatusCode), fmt.Errorf("opentdb returned status %d", resp.StatusCode)
	}

	var payload apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		// Decode errors are treated as non-retryable to avoid amplifying malformed payloads.
		return nil, false, err
	}

	if payload.ResponseCode != 0 {
		return nil, false, fmt.Errorf("opentdb response_code=%d", payload.ResponseCode)
	}

	return payload.Results, false, nil
}

func shouldRetryStatus(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusRequestTimeout {
		return true
	}
	return statusCode >= http.StatusInternalServerError
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
