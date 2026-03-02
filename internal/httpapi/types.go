package httpapi

import (
	"time"

	"quiz-app/internal/quiz"
)

type questionsResponse struct {
	QuizID        string             `json:"quiz_id"`
	QuestionCount int                `json:"question_count"`
	Questions     []questionResponse `json:"questions"`
}

type questionResponse struct {
	QuestionID    string        `json:"question_id"`
	Question      string        `json:"question"`
	Options       []quiz.Option `json:"options"`
	CorrectIndex  int           `json:"correct_index"`
	AttemptStatus string        `json:"attempt_status"`
	AttemptScore  *float64      `json:"attempt_score,omitempty"`
}

type responsesRequest struct {
	QuizID    string                   `json:"quiz_id,omitempty"`
	Username  string                   `json:"username,omitempty"`
	Responses []quiz.SubmittedResponse `json:"responses"`
}

type responsesResponse struct {
	Results  []quiz.ResponseResult `json:"results"`
	Warnings []string              `json:"warnings,omitempty"`
}

type createQuizRequest struct {
	QuestionCount int `json:"question_count"`
}

type createQuizResponse struct {
	QuizID        string    `json:"quiz_id"`
	QuestionCount int       `json:"question_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type leaderboardEntryResponse struct {
	Username         string    `json:"username"`
	TotalScore       float64   `json:"total_score"`
	AnsweredCount    int       `json:"answered_count"`
	LastSubmissionAt time.Time `json:"last_submission_at"`
}

type leaderboardResponse struct {
	QuizID      string                     `json:"quiz_id"`
	Leaderboard []leaderboardEntryResponse `json:"leaderboard"`
}

type activeQuizResponse struct {
	QuizID        string    `json:"quiz_id"`
	QuestionCount int       `json:"question_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type activeQuizzesResponse struct {
	Quizzes []activeQuizResponse `json:"quizzes"`
}

type errorResponse struct {
	Error string `json:"error"`
}
