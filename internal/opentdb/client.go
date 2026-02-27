package opentdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

const (
	apiURL        = "https://opentdb.com/api.php"
	defaultAmount = 10
)

// RawQuestion mirrors the OpenTriviaDB question payload.
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

func FetchQuestions(ctx context.Context, amount int) ([]RawQuestion, error) {
	if amount <= 0 {
		amount = defaultAmount
	}

	reqURL := apiURL + "?amount=" + strconv.Itoa(amount)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opentdb returned status %d", resp.StatusCode)
	}

	var payload apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if payload.ResponseCode != 0 {
		return nil, fmt.Errorf("opentdb response_code=%d", payload.ResponseCode)
	}

	return payload.Results, nil
}
