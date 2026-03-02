package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"quiz-app/internal/quiz"
)

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, quiz.ErrQuizNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "quiz not found"})
	case errors.Is(err, quiz.ErrInvalidUsername):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "username is required to link responses to leaderboard"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "request failed"})
	}
}

func toQuestionResponses(questions []quiz.Question, attemptScores map[string]float64) []questionResponse {
	response := make([]questionResponse, 0, len(questions))
	for _, question := range questions {
		// Intentionally expose correct_index because the current user client scores
		// locally and persists answers asynchronously. This is simpler for this demo
		// but not suitable for adversarial clients.
		item := questionResponse{
			QuestionID:    question.QuestionID,
			Question:      question.Question,
			Options:       question.Options,
			CorrectIndex:  question.CorrectIndex,
			AttemptStatus: "not_attempted",
		}
		if score, ok := attemptScores[question.QuestionID]; ok {
			scoreCopy := score
			item.AttemptScore = &scoreCopy
			item.AttemptStatus = "already_attempted"
		}
		response = append(response, item)
	}
	return response
}

func parseBoolParam(r *http.Request, key string) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	return value == "1" || value == "true" || value == "yes"
}

func parseIntParam(r *http.Request, key string, defaultValue int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New(key + " must be a positive integer")
	}
	return parsed, nil
}

func parseLeaderboardLimit(r *http.Request, defaultValue int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	// <=0 means "entire leaderboard".
	return parsed, nil
}

func writeMethodNotAllowed(w http.ResponseWriter, allowedMethod string) {
	w.Header().Set("Allow", allowedMethod)
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
