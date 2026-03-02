package userclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"quiz-app/internal/quiz"
)

var ErrServiceUnavailable = errors.New("quiz service unavailable")

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("request failed with status %d", e.StatusCode)
	}
	return e.Message
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

type questionItem struct {
	QuestionID    string        `json:"question_id"`
	Question      string        `json:"question"`
	Options       []quiz.Option `json:"options"`
	CorrectIndex  int           `json:"correct_index"`
	AttemptStatus string        `json:"attempt_status"`
	AttemptScore  *float64      `json:"attempt_score,omitempty"`
}

const (
	attemptStatusAlreadyAttempt = "already_attempted"
)

type questionsResponse struct {
	QuizID        string         `json:"quiz_id"`
	QuestionCount int            `json:"question_count"`
	Questions     []questionItem `json:"questions"`
}

type activeQuizItem struct {
	QuizID        string `json:"quiz_id"`
	QuestionCount int    `json:"question_count"`
	CreatedAt     string `json:"created_at"`
}

type activeQuizzesResponse struct {
	Quizzes []activeQuizItem `json:"quizzes"`
}

type leaderboardEntryResponse struct {
	Username         string  `json:"username"`
	TotalScore       float64 `json:"total_score"`
	AnsweredCount    int     `json:"answered_count"`
	LastSubmissionAt string  `json:"last_submission_at"`
}

type leaderboardResponse struct {
	QuizID      string                     `json:"quiz_id"`
	Leaderboard []leaderboardEntryResponse `json:"leaderboard"`
}

type responsesRequest struct {
	QuizID    string                   `json:"quiz_id"`
	Username  string                   `json:"username"`
	Responses []quiz.SubmittedResponse `json:"responses"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &HTTPClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (c *HTTPClient) ListActiveQuizzes(ctx context.Context, limit int) ([]quiz.QuizMetadata, error) {
	if limit <= 0 {
		limit = 10
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))

	var payload activeQuizzesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/quizzes/active?"+query.Encode(), nil, &payload); err != nil {
		return nil, err
	}

	quizzes := make([]quiz.QuizMetadata, 0, len(payload.Quizzes))
	for _, item := range payload.Quizzes {
		createdAt, err := parseTime(item.CreatedAt)
		if err != nil {
			return nil, err
		}
		quizzes = append(quizzes, quiz.QuizMetadata{
			QuizID:        item.QuizID,
			QuestionCount: item.QuestionCount,
			CreatedAt:     createdAt,
		})
	}

	return quizzes, nil
}

func (c *HTTPClient) GetLeaderboard(ctx context.Context, quizID string, limit int) ([]quiz.LeaderboardEntry, error) {
	if strings.TrimSpace(quizID) == "" {
		return nil, errors.New("quiz_id is required")
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	path := "/quizzes/" + url.PathEscape(quizID) + "/leaderboard?" + query.Encode()

	var payload leaderboardResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}

	entries := make([]quiz.LeaderboardEntry, 0, len(payload.Leaderboard))
	for _, item := range payload.Leaderboard {
		lastSubmissionAt, err := parseTime(item.LastSubmissionAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, quiz.LeaderboardEntry{
			Username:         item.Username,
			TotalScore:       item.TotalScore,
			AnsweredCount:    item.AnsweredCount,
			LastSubmissionAt: lastSubmissionAt,
		})
	}

	return entries, nil
}

func (c *HTTPClient) GetQuizQuestions(ctx context.Context, quizID, username string, createIfMissing bool, questionCount int) (questionsResponse, error) {
	if strings.TrimSpace(quizID) == "" {
		return questionsResponse{}, errors.New("quiz_id is required")
	}

	query := url.Values{}
	query.Set("quiz_id", quizID)
	if createIfMissing {
		query.Set("create_if_missing", "true")
		if questionCount > 0 {
			query.Set("question_count", strconv.Itoa(questionCount))
		}
	}
	if trimmed := strings.TrimSpace(username); trimmed != "" {
		query.Set("username", trimmed)
	}

	var payload questionsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/questions?"+query.Encode(), nil, &payload); err != nil {
		return questionsResponse{}, err
	}
	return payload, nil
}

func (c *HTTPClient) PersistSingleResponse(ctx context.Context, quizID, username, questionID, answer string) error {
	request := responsesRequest{
		QuizID:   quizID,
		Username: username,
		Responses: []quiz.SubmittedResponse{
			{
				QuestionID: questionID,
				Answer:     answer,
			},
		},
	}

	return c.doJSON(ctx, http.MethodPost, "/responses", request, nil)
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	fullURL := c.baseURL + path

	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		apiErr := APIError{StatusCode: response.StatusCode}
		var payload errorResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err == nil && strings.TrimSpace(payload.Error) != "" {
			apiErr.Message = payload.Error
		}
		if apiErr.Message == "" {
			apiErr.Message = response.Status
		}
		return &apiErr
	}

	if responseBody == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(responseBody)
}
