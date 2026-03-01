package quiz

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"quiz-app/internal/opentdb"
)

type QuestionsFetcher func(ctx context.Context, amount int) ([]opentdb.RawQuestion, error)

type Service struct {
	quizzes  QuizRepository
	attempts AttemptRepository
	fetcher  QuestionsFetcher
}

func NewService(quizzes QuizRepository, attempts AttemptRepository, fetcher QuestionsFetcher) *Service {
	return &Service{
		quizzes:  quizzes,
		attempts: attempts,
		fetcher:  fetcher,
	}
}

func (s *Service) CreateQuiz(ctx context.Context, questionCount int) (QuizMetadata, error) {
	quizID := generateQuizID()
	return s.createQuizWithID(ctx, quizID, questionCount)
}

func (s *Service) EnsureQuiz(ctx context.Context, quizID string, createIfMissing bool, questionCount int) (QuizMetadata, error) {
	quizID = strings.TrimSpace(quizID)
	if quizID == "" {
		return QuizMetadata{}, ErrQuizNotFound
	}

	metadata, err := s.quizzes.GetQuizMetadata(ctx, quizID)
	if err == nil {
		return metadata, nil
	}
	if !errors.Is(err, ErrQuizNotFound) {
		return QuizMetadata{}, err
	}
	if !createIfMissing {
		return QuizMetadata{}, ErrQuizNotFound
	}

	return s.createQuizWithID(ctx, quizID, questionCount)
}

func (s *Service) GetQuizQuestions(ctx context.Context, quizID string, createIfMissing bool, questionCount int) (QuizMetadata, []Question, error) {
	metadata, err := s.EnsureQuiz(ctx, quizID, createIfMissing, questionCount)
	if err != nil {
		return QuizMetadata{}, nil, err
	}

	questions, err := s.quizzes.GetQuizQuestions(ctx, metadata.QuizID)
	if err != nil {
		return QuizMetadata{}, nil, err
	}
	return metadata, questions, nil
}

func (s *Service) EvaluateResponsesForQuiz(ctx context.Context, quizID string, responses []SubmittedResponse) ([]ResponseResult, error) {
	metadata, err := s.EnsureQuiz(ctx, quizID, false, 0)
	if err != nil {
		return nil, err
	}

	questions, err := s.quizzes.GetQuizQuestions(ctx, metadata.QuizID)
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]Question, len(questions))
	for _, question := range questions {
		lookup[question.QuestionID] = question
	}

	results := make([]ResponseResult, 0, len(responses))
	for _, response := range responses {
		question, ok := lookup[response.QuestionID]
		if !ok {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidQuestion,
			})
			continue
		}

		letter := normalizeLetter(response.Answer)
		if letter == "" {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidLetter,
			})
			continue
		}

		answerIndex := int(letter[0] - 'A')
		if answerIndex < 0 || answerIndex >= len(question.Options) {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidLetter,
			})
			continue
		}

		status := StatusIncorrect
		if answerIndex == question.CorrectIndex {
			status = StatusCorrect
		}
		results = append(results, ResponseResult{
			QuestionID: response.QuestionID,
			Status:     status,
		})
	}

	return results, nil
}

func (s *Service) SubmitResponses(ctx context.Context, quizID, username string, responses []SubmittedResponse) ([]ResponseResult, error) {
	metadata, err := s.EnsureQuiz(ctx, quizID, false, 0)
	if err != nil {
		return nil, err
	}

	usernameNormalized, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}

	return s.attempts.SubmitResponses(ctx, metadata.QuizID, usernameNormalized, responses)
}

func (s *Service) GetLeaderboard(ctx context.Context, quizID string) ([]LeaderboardEntry, error) {
	metadata, err := s.EnsureQuiz(ctx, quizID, false, 0)
	if err != nil {
		return nil, err
	}

	return s.attempts.GetLeaderboard(ctx, metadata.QuizID)
}

func (s *Service) ListActiveQuizzes(ctx context.Context, limit int) ([]QuizMetadata, error) {
	return s.quizzes.ListActiveQuizzes(ctx, limit)
}

func (s *Service) createQuizWithID(ctx context.Context, quizID string, questionCount int) (QuizMetadata, error) {
	if s.fetcher == nil {
		return QuizMetadata{}, errors.New("question fetcher is not configured")
	}

	existing, err := s.quizzes.GetQuizMetadata(ctx, quizID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrQuizNotFound) {
		return QuizMetadata{}, err
	}

	rawQuestions, err := s.fetcher(ctx, questionCount)
	if err != nil {
		return QuizMetadata{}, err
	}

	questions := BuildQuestions(rawQuestions)
	now := time.Now().UTC()
	metadata := QuizMetadata{
		QuizID:        quizID,
		QuestionCount: len(questions),
		CreatedAt:     now,
	}

	if err := s.quizzes.CreateQuiz(ctx, metadata, questions); err != nil {
		existing, lookupErr := s.quizzes.GetQuizMetadata(ctx, quizID)
		if lookupErr == nil {
			return existing, nil
		}
		return QuizMetadata{}, err
	}

	return metadata, nil
}

func normalizeUsername(username string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(username))
	if normalized == "" {
		return "", ErrInvalidUsername
	}
	return normalized, nil
}

func generateQuizID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 10

	var builder strings.Builder
	builder.Grow(len("qz_") + length)
	builder.WriteString("qz_")
	for idx := 0; idx < length; idx++ {
		builder.WriteByte(alphabet[rand.Intn(len(alphabet))])
	}
	return builder.String()
}
