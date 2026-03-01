package quiz

import (
	"context"
	"errors"
	"time"
)

var (
	ErrQuizNotFound    = errors.New("quiz not found")
	ErrInvalidUsername = errors.New("invalid username")
)

type QuizMetadata struct {
	QuizID        string
	QuestionCount int
	CreatedAt     time.Time
}

type LeaderboardEntry struct {
	Username         string    `json:"username"`
	TotalScore       int       `json:"total_score"`
	AnsweredCount    int       `json:"answered_count"`
	LastSubmissionAt time.Time `json:"last_submission_at"`
}

type QuizRepository interface {
	CreateQuiz(ctx context.Context, metadata QuizMetadata, questions []Question) error
	GetQuizMetadata(ctx context.Context, quizID string) (QuizMetadata, error)
	GetQuizQuestions(ctx context.Context, quizID string) ([]Question, error)
	QuizExists(ctx context.Context, quizID string) (bool, error)
	ListActiveQuizzes(ctx context.Context, limit int) ([]QuizMetadata, error)
}

type AttemptRepository interface {
	SubmitResponses(ctx context.Context, quizID, usernameNormalized string, responses []SubmittedResponse) ([]ResponseResult, error)
	GetLeaderboard(ctx context.Context, quizID string) ([]LeaderboardEntry, error)
}
