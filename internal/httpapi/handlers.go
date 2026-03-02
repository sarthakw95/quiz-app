package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"quiz-app/internal/quiz"
)

const (
	defaultQuestionCount = 10
	defaultListLimit     = 10
)

func (a *API) HandleQuestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.service == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "quiz service unavailable"})
		return
	}

	quizID := strings.TrimSpace(r.URL.Query().Get("quiz_id"))
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	createIfMissing := parseBoolParam(r, "create_if_missing")
	questionCount, err := parseIntParam(r, "question_count", defaultQuestionCount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	var (
		metadata  quiz.QuizMetadata
		questions []quiz.Question
	)

	if quizID == "" {
		metadata, err = a.service.CreateQuiz(r.Context(), questionCount)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch questions"})
			return
		}
		_, questions, err = a.service.GetQuizQuestions(r.Context(), metadata.QuizID, false, 0)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	} else {
		metadata, questions, err = a.service.GetQuizQuestions(r.Context(), quizID, createIfMissing, questionCount)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	}

	a.bank.AddBuiltQuestions(questions)

	var attemptScores map[string]float64
	if quizID != "" && username != "" {
		attemptScores, err = a.service.GetAttemptScores(r.Context(), metadata.QuizID, username)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, questionsResponse{
		QuizID:        metadata.QuizID,
		QuestionCount: len(questions),
		Questions:     toQuestionResponses(questions, attemptScores),
	})
}

func (a *API) HandleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	defer r.Body.Close()

	var request responsesRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	if request.Responses == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "responses is required"})
		return
	}

	quizID := strings.TrimSpace(request.QuizID)
	username := strings.TrimSpace(request.Username)
	var (
		results  []quiz.ResponseResult
		err      error
		warnings []string
	)

	if quizID != "" && username != "" {
		results, err = a.service.SubmitResponses(r.Context(), quizID, username, request.Responses)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	} else if quizID != "" {
		// Preserve useful quiz-scoped validation even when caller is unauthenticated.
		results, err = a.service.EvaluateResponsesForQuiz(r.Context(), quizID, request.Responses)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	} else {
		results = a.bank.EvaluateResponses(request.Responses)
	}

	if quizID == "" || username == "" {
		// Explicitly signal that answers were processed but not persisted for leaderboard usage.
		warnings = append(warnings, "responses are not linked to leaderboard unless both quiz_id and username are provided")
	}

	writeJSON(w, http.StatusOK, responsesResponse{
		Results:  results,
		Warnings: warnings,
	})
}

func (a *API) HandleCreateQuiz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if a.service == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "quiz service unavailable"})
		return
	}

	request := createQuizRequest{}
	if r.ContentLength > 0 {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
			return
		}
	}

	metadata, err := a.service.CreateQuiz(r.Context(), request.QuestionCount)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to create quiz"})
		return
	}

	_, questions, err := a.service.GetQuizQuestions(r.Context(), metadata.QuizID, false, 0)
	if err == nil {
		a.bank.AddBuiltQuestions(questions)
	}

	writeJSON(w, http.StatusCreated, createQuizResponse{
		QuizID:        metadata.QuizID,
		QuestionCount: metadata.QuestionCount,
		CreatedAt:     metadata.CreatedAt,
	})
}

func (a *API) HandleQuizQuestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.service == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "quiz service unavailable"})
		return
	}

	quizID := strings.TrimSpace(r.PathValue("quiz_id"))
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	createIfMissing := parseBoolParam(r, "create_if_missing")
	questionCount, err := parseIntParam(r, "question_count", defaultQuestionCount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	metadata, questions, serviceErr := a.service.GetQuizQuestions(r.Context(), quizID, createIfMissing, questionCount)
	if serviceErr != nil {
		writeServiceError(w, serviceErr)
		return
	}

	a.bank.AddBuiltQuestions(questions)

	var attemptScores map[string]float64
	if username != "" {
		attemptScores, err = a.service.GetAttemptScores(r.Context(), metadata.QuizID, username)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, questionsResponse{
		QuizID:        metadata.QuizID,
		QuestionCount: len(questions),
		Questions:     toQuestionResponses(questions, attemptScores),
	})
}

func (a *API) HandleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.service == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "quiz service unavailable"})
		return
	}

	quizID := strings.TrimSpace(r.PathValue("quiz_id"))
	if quizID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "quiz_id is required"})
		return
	}

	limit, err := parseLeaderboardLimit(r, 10)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	entries, err := a.service.GetLeaderboard(r.Context(), quizID, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	items := make([]leaderboardEntryResponse, 0, len(entries))
	for _, entry := range entries {
		items = append(items, leaderboardEntryResponse{
			Username:         entry.Username,
			TotalScore:       entry.TotalScore,
			AnsweredCount:    entry.AnsweredCount,
			LastSubmissionAt: entry.LastSubmissionAt,
		})
	}

	writeJSON(w, http.StatusOK, leaderboardResponse{
		QuizID:      quizID,
		Leaderboard: items,
	})
}

func (a *API) HandleActiveQuizzes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.service == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "quiz service unavailable"})
		return
	}

	limit, err := parseIntParam(r, "limit", defaultListLimit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	active, err := a.service.ListActiveQuizzes(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list active quizzes"})
		return
	}

	response := activeQuizzesResponse{
		Quizzes: make([]activeQuizResponse, 0, len(active)),
	}
	for _, item := range active {
		response.Quizzes = append(response.Quizzes, activeQuizResponse{
			QuizID:        item.QuizID,
			QuestionCount: item.QuestionCount,
			CreatedAt:     item.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, response)
}
