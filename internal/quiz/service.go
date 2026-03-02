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

	quizMetaCache    map[string]QuizMetadata
	quizQuestions    map[string][]Question
	leaderboardCache map[string]*leaderboardCache
	attemptScores    map[string]map[string]float64
}

type leaderboardCache struct {
	ordered     []LeaderboardEntry
	indexByUser map[string]int
}

func NewService(quizzes QuizRepository, attempts AttemptRepository, fetcher QuestionsFetcher) *Service {
	return &Service{
		quizzes:          quizzes,
		attempts:         attempts,
		fetcher:          fetcher,
		quizMetaCache:    make(map[string]QuizMetadata),
		quizQuestions:    make(map[string][]Question),
		leaderboardCache: make(map[string]*leaderboardCache),
		attemptScores:    make(map[string]map[string]float64),
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

	if metadata, ok := s.getCachedQuizMetadata(quizID); ok {
		return metadata, nil
	}

	metadata, err := s.quizzes.GetQuizMetadata(ctx, quizID)
	if err == nil {
		s.setCachedQuizMetadata(metadata)
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
	if metadata, questions, ok := s.getCachedQuiz(quizID); ok {
		return metadata, questions, nil
	}

	metadata, err := s.EnsureQuiz(ctx, quizID, createIfMissing, questionCount)
	if err != nil {
		return QuizMetadata{}, nil, err
	}

	questions, err := s.quizzes.GetQuizQuestions(ctx, metadata.QuizID)
	if err != nil {
		return QuizMetadata{}, nil, err
	}
	s.setCachedQuiz(metadata, questions)
	return metadata, questions, nil
}

func (s *Service) EvaluateResponsesForQuiz(ctx context.Context, quizID string, responses []SubmittedResponse) ([]ResponseResult, error) {
	_, questions, err := s.GetQuizQuestions(ctx, quizID, false, 0)
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

	results, err := s.attempts.SubmitResponses(ctx, metadata.QuizID, usernameNormalized, responses)
	if err != nil {
		return nil, err
	}

	s.updateCachedLeaderboardAfterSubmission(metadata.QuizID, usernameNormalized, results)
	s.updateCachedAttemptScoresAfterSubmission(metadata.QuizID, usernameNormalized, results)
	return results, nil
}

func (s *Service) GetLeaderboard(ctx context.Context, quizID string, limit int) ([]LeaderboardEntry, error) {
	metadata, err := s.EnsureQuiz(ctx, quizID, false, 0)
	if err != nil {
		return nil, err
	}

	if entries, ok := s.getCachedLeaderboard(metadata.QuizID); ok {
		return applyLeaderboardLimit(entries, limit), nil
	}

	entries, err := s.attempts.GetLeaderboard(ctx, metadata.QuizID)
	if err != nil {
		return nil, err
	}

	s.setCachedLeaderboard(metadata.QuizID, entries)
	return applyLeaderboardLimit(entries, limit), nil
}

func (s *Service) GetAttemptScores(ctx context.Context, quizID, username string) (map[string]float64, error) {
	metadata, err := s.EnsureQuiz(ctx, quizID, false, 0)
	if err != nil {
		return nil, err
	}

	usernameNormalized, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}

	if scores, ok := s.getCachedAttemptScores(metadata.QuizID, usernameNormalized); ok {
		return scores, nil
	}

	scores, err := s.attempts.GetAttemptScores(ctx, metadata.QuizID, usernameNormalized)
	if err != nil {
		return nil, err
	}
	s.setCachedAttemptScores(metadata.QuizID, usernameNormalized, scores)
	return scores, nil
}

func (s *Service) ListActiveQuizzes(ctx context.Context, limit int) ([]QuizMetadata, error) {
	return s.quizzes.ListActiveQuizzes(ctx, limit)
}

func (s *Service) createQuizWithID(ctx context.Context, quizID string, questionCount int) (QuizMetadata, error) {
	if s.fetcher == nil {
		return QuizMetadata{}, errors.New("question fetcher is not configured")
	}

	if metadata, ok := s.getCachedQuizMetadata(quizID); ok {
		return metadata, nil
	}

	existing, err := s.quizzes.GetQuizMetadata(ctx, quizID)
	if err == nil {
		s.setCachedQuizMetadata(existing)
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
			s.setCachedQuizMetadata(existing)
			return existing, nil
		}
		return QuizMetadata{}, err
	}

	s.setCachedQuiz(metadata, questions)
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

func (s *Service) getCachedQuizMetadata(quizID string) (QuizMetadata, bool) {
	metadata, ok := s.quizMetaCache[quizID]
	return metadata, ok
}

func (s *Service) setCachedQuizMetadata(metadata QuizMetadata) {
	s.quizMetaCache[metadata.QuizID] = metadata
}

func (s *Service) getCachedQuiz(quizID string) (QuizMetadata, []Question, bool) {
	metadata, metaOK := s.quizMetaCache[quizID]
	questions, questionsOK := s.quizQuestions[quizID]
	if !metaOK || !questionsOK {
		return QuizMetadata{}, nil, false
	}
	return metadata, questions, true
}

func (s *Service) setCachedQuiz(metadata QuizMetadata, questions []Question) {
	s.quizMetaCache[metadata.QuizID] = metadata
	s.quizQuestions[metadata.QuizID] = questions
}

func (s *Service) getCachedLeaderboard(quizID string) ([]LeaderboardEntry, bool) {
	cache, ok := s.leaderboardCache[quizID]
	if !ok || cache == nil {
		return nil, false
	}
	return cache.ordered, true
}

func (s *Service) getCachedAttemptScores(quizID, usernameNormalized string) (map[string]float64, bool) {
	scores, ok := s.attemptScores[attemptScoresCacheKey(quizID, usernameNormalized)]
	return scores, ok
}

func (s *Service) setCachedAttemptScores(quizID, usernameNormalized string, scores map[string]float64) {
	if scores == nil {
		scores = make(map[string]float64)
	}
	s.attemptScores[attemptScoresCacheKey(quizID, usernameNormalized)] = scores
}

func (s *Service) setCachedLeaderboard(quizID string, entries []LeaderboardEntry) {
	indexByUser := make(map[string]int, len(entries))
	for idx := range entries {
		indexByUser[entries[idx].Username] = idx
	}

	s.leaderboardCache[quizID] = &leaderboardCache{
		ordered:     entries,
		indexByUser: indexByUser,
	}
}

func (s *Service) updateCachedAttemptScoresAfterSubmission(quizID, usernameNormalized string, results []ResponseResult) {
	scores, ok := s.getCachedAttemptScores(quizID, usernameNormalized)
	if !ok {
		return
	}

	for _, result := range results {
		switch result.Status {
		case StatusCorrect:
			scores[result.QuestionID] = 1.0
		case StatusIncorrect:
			scores[result.QuestionID] = 0.0
		case StatusAlreadyAnswered:
			if result.AttemptScore != nil {
				scores[result.QuestionID] = *result.AttemptScore
			}
		}
	}
}

func (s *Service) updateCachedLeaderboardAfterSubmission(quizID, username string, results []ResponseResult) {
	cache, ok := s.leaderboardCache[quizID]
	if !ok || cache == nil {
		return
	}

	newAnswers := 0
	scoreDelta := 0.0
	for _, result := range results {
		switch result.Status {
		case StatusCorrect:
			newAnswers++
			scoreDelta += 1.0
		case StatusIncorrect:
			newAnswers++
		}
	}
	if newAnswers == 0 {
		return
	}

	now := time.Now().UTC()
	idx, exists := cache.indexByUser[username]
	if !exists {
		cache.ordered = append(cache.ordered, LeaderboardEntry{
			Username:         username,
			TotalScore:       scoreDelta,
			AnsweredCount:    newAnswers,
			LastSubmissionAt: now,
		})
		idx = len(cache.ordered) - 1
		cache.indexByUser[username] = idx
		s.bubbleLeaderboard(cache, idx)
		return
	}

	cache.ordered[idx].TotalScore += scoreDelta
	cache.ordered[idx].AnsweredCount += newAnswers
	cache.ordered[idx].LastSubmissionAt = now
	s.bubbleLeaderboard(cache, idx)
}

func attemptScoresCacheKey(quizID, usernameNormalized string) string {
	return quizID + "::" + usernameNormalized
}

func (s *Service) bubbleLeaderboard(cache *leaderboardCache, idx int) {
	for idx > 0 && leaderboardBefore(cache.ordered[idx], cache.ordered[idx-1]) {
		s.swapLeaderboardEntries(cache, idx, idx-1)
		idx--
	}

	for idx+1 < len(cache.ordered) && leaderboardBefore(cache.ordered[idx+1], cache.ordered[idx]) {
		s.swapLeaderboardEntries(cache, idx, idx+1)
		idx++
	}
}

func (s *Service) swapLeaderboardEntries(cache *leaderboardCache, i, j int) {
	cache.ordered[i], cache.ordered[j] = cache.ordered[j], cache.ordered[i]
	cache.indexByUser[cache.ordered[i].Username] = i
	cache.indexByUser[cache.ordered[j].Username] = j
}

func leaderboardBefore(a, b LeaderboardEntry) bool {
	if a.TotalScore != b.TotalScore {
		return a.TotalScore > b.TotalScore
	}
	if !a.LastSubmissionAt.Equal(b.LastSubmissionAt) {
		return a.LastSubmissionAt.Before(b.LastSubmissionAt)
	}
	return a.Username < b.Username
}

func applyLeaderboardLimit(entries []LeaderboardEntry, limit int) []LeaderboardEntry {
	if limit <= 0 || limit >= len(entries) {
		return entries
	}
	return entries[:limit]
}
