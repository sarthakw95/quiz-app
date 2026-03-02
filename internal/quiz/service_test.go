package quiz

import (
	"context"
	"testing"
	"time"
)

type fakeQuizRepo struct {
	metadataByQuiz  map[string]QuizMetadata
	questionsByQuiz map[string][]Question

	createCalls       int
	getMetadataCalls  int
	getQuestionsCalls int
	listCalls         int
}

func newFakeQuizRepo() *fakeQuizRepo {
	return &fakeQuizRepo{
		metadataByQuiz:  make(map[string]QuizMetadata),
		questionsByQuiz: make(map[string][]Question),
	}
}

func (f *fakeQuizRepo) CreateQuiz(_ context.Context, metadata QuizMetadata, questions []Question) error {
	f.createCalls++
	f.metadataByQuiz[metadata.QuizID] = metadata
	f.questionsByQuiz[metadata.QuizID] = questions
	return nil
}

func (f *fakeQuizRepo) GetQuizMetadata(_ context.Context, quizID string) (QuizMetadata, error) {
	f.getMetadataCalls++
	item, ok := f.metadataByQuiz[quizID]
	if !ok {
		return QuizMetadata{}, ErrQuizNotFound
	}
	return item, nil
}

func (f *fakeQuizRepo) GetQuizQuestions(_ context.Context, quizID string) ([]Question, error) {
	f.getQuestionsCalls++
	items, ok := f.questionsByQuiz[quizID]
	if !ok {
		return nil, ErrQuizNotFound
	}
	return items, nil
}

func (f *fakeQuizRepo) QuizExists(_ context.Context, quizID string) (bool, error) {
	_, ok := f.metadataByQuiz[quizID]
	return ok, nil
}

func (f *fakeQuizRepo) ListActiveQuizzes(_ context.Context, limit int) ([]QuizMetadata, error) {
	f.listCalls++
	out := make([]QuizMetadata, 0, len(f.metadataByQuiz))
	for _, item := range f.metadataByQuiz {
		out = append(out, item)
	}
	if limit > 0 && limit < len(out) {
		return out[:limit], nil
	}
	return out, nil
}

type fakeAttemptRepo struct {
	submitResults []ResponseResult
	submitErr     error
	submitCalls   int

	lastSubmitQuizID   string
	lastSubmitUsername string

	leaderboard      []LeaderboardEntry
	leaderboardErr   error
	leaderboardCalls int

	attemptScores      map[string]float64
	attemptScoresErr   error
	attemptScoresCalls int

	lastAttemptQuizID   string
	lastAttemptUsername string
}

func (f *fakeAttemptRepo) SubmitResponses(_ context.Context, quizID, usernameNormalized string, _ []SubmittedResponse) ([]ResponseResult, error) {
	f.submitCalls++
	f.lastSubmitQuizID = quizID
	f.lastSubmitUsername = usernameNormalized
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return f.submitResults, nil
}

func (f *fakeAttemptRepo) GetLeaderboard(_ context.Context, quizID string) ([]LeaderboardEntry, error) {
	f.leaderboardCalls++
	f.lastAttemptQuizID = quizID
	if f.leaderboardErr != nil {
		return nil, f.leaderboardErr
	}
	return f.leaderboard, nil
}

func (f *fakeAttemptRepo) GetAttemptScores(_ context.Context, quizID, usernameNormalized string) (map[string]float64, error) {
	f.attemptScoresCalls++
	f.lastAttemptQuizID = quizID
	f.lastAttemptUsername = usernameNormalized
	if f.attemptScoresErr != nil {
		return nil, f.attemptScoresErr
	}
	return f.attemptScores, nil
}

func float64Ptr(v float64) *float64 {
	return &v
}

func TestServiceGetQuizQuestionsCachesRepoReads(t *testing.T) {
	repo := newFakeQuizRepo()
	repo.metadataByQuiz["quiz-1"] = QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 1,
		CreatedAt:     time.Unix(1, 0).UTC(),
	}
	repo.questionsByQuiz["quiz-1"] = []Question{
		{
			PublicQuestion: PublicQuestion{
				QuestionID: "q1",
				Question:   "Question",
				Options:    []Option{{Letter: "A", Text: "One"}},
			},
			CorrectIndex: 0,
		},
	}

	attempts := &fakeAttemptRepo{}
	service := NewService(repo, attempts, nil)

	_, gotQuestions, err := service.GetQuizQuestions(context.Background(), "quiz-1", false, 0)
	if err != nil {
		t.Fatalf("first GetQuizQuestions failed: %v", err)
	}
	if len(gotQuestions) != 1 {
		t.Fatalf("expected one question, got %d", len(gotQuestions))
	}
	if repo.getMetadataCalls != 1 || repo.getQuestionsCalls != 1 {
		t.Fatalf("unexpected repository calls after first lookup: metadata=%d questions=%d", repo.getMetadataCalls, repo.getQuestionsCalls)
	}

	_, _, err = service.GetQuizQuestions(context.Background(), "quiz-1", false, 0)
	if err != nil {
		t.Fatalf("second GetQuizQuestions failed: %v", err)
	}
	if repo.getMetadataCalls != 1 || repo.getQuestionsCalls != 1 {
		t.Fatalf("expected second lookup to be cache-only, got metadata=%d questions=%d", repo.getMetadataCalls, repo.getQuestionsCalls)
	}
}

func TestServiceGetAttemptScoresCachesAndNormalizesUsername(t *testing.T) {
	repo := newFakeQuizRepo()
	repo.metadataByQuiz["quiz-1"] = QuizMetadata{QuizID: "quiz-1"}

	attempts := &fakeAttemptRepo{
		attemptScores: map[string]float64{
			"q1": 1.0,
		},
	}
	service := NewService(repo, attempts, nil)

	scores, err := service.GetAttemptScores(context.Background(), "quiz-1", " Alice ")
	if err != nil {
		t.Fatalf("first GetAttemptScores failed: %v", err)
	}
	if got := scores["q1"]; got != 1.0 {
		t.Fatalf("unexpected attempt score for q1: %v", got)
	}
	if attempts.attemptScoresCalls != 1 {
		t.Fatalf("expected one repository attempt-score read, got %d", attempts.attemptScoresCalls)
	}
	if attempts.lastAttemptUsername != "alice" {
		t.Fatalf("username not normalized before repository call: got %q", attempts.lastAttemptUsername)
	}

	_, err = service.GetAttemptScores(context.Background(), "quiz-1", "alice")
	if err != nil {
		t.Fatalf("second GetAttemptScores failed: %v", err)
	}
	if attempts.attemptScoresCalls != 1 {
		t.Fatalf("expected cached attempt scores on second read, got calls=%d", attempts.attemptScoresCalls)
	}
}

func TestServiceSubmitResponsesUpdatesCachedLeaderboardAndAttemptScores(t *testing.T) {
	repo := newFakeQuizRepo()
	repo.metadataByQuiz["quiz-1"] = QuizMetadata{QuizID: "quiz-1"}

	attempts := &fakeAttemptRepo{
		submitResults: []ResponseResult{
			{QuestionID: "q1", Status: StatusCorrect},
			{QuestionID: "q2", Status: StatusIncorrect},
			{QuestionID: "q3", Status: StatusAlreadyAnswered, AttemptScore: float64Ptr(0.5)},
		},
	}

	service := NewService(repo, attempts, nil)
	service.setCachedLeaderboard("quiz-1", []LeaderboardEntry{
		{
			Username:         "bob",
			TotalScore:       2.0,
			AnsweredCount:    2,
			LastSubmissionAt: time.Unix(100, 0).UTC(),
		},
	})
	service.setCachedAttemptScores("quiz-1", "alice", map[string]float64{"old": 1.0})

	_, err := service.SubmitResponses(context.Background(), "quiz-1", " Alice ", []SubmittedResponse{
		{QuestionID: "q1", Answer: "A"},
	})
	if err != nil {
		t.Fatalf("SubmitResponses failed: %v", err)
	}

	if attempts.submitCalls != 1 {
		t.Fatalf("expected one submit call, got %d", attempts.submitCalls)
	}
	if attempts.lastSubmitUsername != "alice" {
		t.Fatalf("username not normalized before submit: %q", attempts.lastSubmitUsername)
	}

	leaderboard, ok := service.getCachedLeaderboard("quiz-1")
	if !ok {
		t.Fatalf("expected leaderboard to stay cached")
	}
	var alice *LeaderboardEntry
	for idx := range leaderboard {
		if leaderboard[idx].Username == "alice" {
			alice = &leaderboard[idx]
			break
		}
	}
	if alice == nil {
		t.Fatalf("alice not added to cached leaderboard: %+v", leaderboard)
	}
	if alice.TotalScore != 1.0 {
		t.Fatalf("unexpected cached score for alice: %v", alice.TotalScore)
	}
	if alice.AnsweredCount != 2 {
		t.Fatalf("unexpected answered count for alice: %d", alice.AnsweredCount)
	}

	scores, ok := service.getCachedAttemptScores("quiz-1", "alice")
	if !ok {
		t.Fatalf("expected attempt score cache for alice")
	}
	if scores["q1"] != 1.0 {
		t.Fatalf("expected q1 score=1.0, got %v", scores["q1"])
	}
	if scores["q2"] != 0.0 {
		t.Fatalf("expected q2 score=0.0, got %v", scores["q2"])
	}
	if scores["q3"] != 0.5 {
		t.Fatalf("expected q3 score=0.5, got %v", scores["q3"])
	}
}

func TestServiceSubmitResponsesDoesNotCreateAttemptScoreCacheWhenMissing(t *testing.T) {
	repo := newFakeQuizRepo()
	repo.metadataByQuiz["quiz-1"] = QuizMetadata{QuizID: "quiz-1"}

	attempts := &fakeAttemptRepo{
		submitResults: []ResponseResult{
			{QuestionID: "q1", Status: StatusCorrect},
		},
	}
	service := NewService(repo, attempts, nil)

	_, err := service.SubmitResponses(context.Background(), "quiz-1", "alice", []SubmittedResponse{
		{QuestionID: "q1", Answer: "A"},
	})
	if err != nil {
		t.Fatalf("SubmitResponses failed: %v", err)
	}

	if len(service.attemptScores) != 0 {
		t.Fatalf("expected no attempt-score cache creation on write path, got %d entries", len(service.attemptScores))
	}
}

func TestServiceGetLeaderboardCachesAndAppliesLimit(t *testing.T) {
	repo := newFakeQuizRepo()
	repo.metadataByQuiz["quiz-1"] = QuizMetadata{QuizID: "quiz-1"}

	attempts := &fakeAttemptRepo{
		leaderboard: []LeaderboardEntry{
			{Username: "a", TotalScore: 3},
			{Username: "b", TotalScore: 2},
			{Username: "c", TotalScore: 1},
		},
	}
	service := NewService(repo, attempts, nil)

	topTwo, err := service.GetLeaderboard(context.Background(), "quiz-1", 2)
	if err != nil {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	if len(topTwo) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(topTwo))
	}
	if attempts.leaderboardCalls != 1 {
		t.Fatalf("expected one repository leaderboard read, got %d", attempts.leaderboardCalls)
	}

	topOne, err := service.GetLeaderboard(context.Background(), "quiz-1", 1)
	if err != nil {
		t.Fatalf("second GetLeaderboard failed: %v", err)
	}
	if len(topOne) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(topOne))
	}
	if attempts.leaderboardCalls != 1 {
		t.Fatalf("expected cached leaderboard on second read, got calls=%d", attempts.leaderboardCalls)
	}

	allEntries, err := service.GetLeaderboard(context.Background(), "quiz-1", -1)
	if err != nil {
		t.Fatalf("GetLeaderboard(all) failed: %v", err)
	}
	if len(allEntries) != 3 {
		t.Fatalf("expected all entries when limit <= 0, got %d", len(allEntries))
	}
}
