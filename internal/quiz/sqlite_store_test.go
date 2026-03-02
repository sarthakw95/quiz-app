package quiz

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(path)
		_ = os.Remove(path + "-wal")
		_ = os.Remove(path + "-shm")
		_ = os.Remove(path + "-journal")
	})
	return store
}

func sampleQuestions() []Question {
	return []Question{
		{
			PublicQuestion: PublicQuestion{
				QuestionID: "q1",
				Question:   "2+2?",
				Options: []Option{
					{Letter: "A", Text: "4"},
					{Letter: "B", Text: "3"},
				},
			},
			CorrectIndex: 0,
		},
		{
			PublicQuestion: PublicQuestion{
				QuestionID: "q2",
				Question:   "Sky color?",
				Options: []Option{
					{Letter: "A", Text: "Green"},
					{Letter: "B", Text: "Blue"},
				},
			},
			CorrectIndex: 1,
		},
	}
}

func TestSQLiteStoreCreateAndReadQuiz(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	createdAt := time.Unix(1700000000, 123).UTC()
	meta := QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 2,
		CreatedAt:     createdAt,
	}
	questions := sampleQuestions()
	if err := store.CreateQuiz(ctx, meta, questions); err != nil {
		t.Fatalf("CreateQuiz failed: %v", err)
	}

	gotMeta, err := store.GetQuizMetadata(ctx, "quiz-1")
	if err != nil {
		t.Fatalf("GetQuizMetadata failed: %v", err)
	}
	if gotMeta.QuizID != "quiz-1" || gotMeta.QuestionCount != 2 || !gotMeta.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected metadata: %+v", gotMeta)
	}

	gotQuestions, err := store.GetQuizQuestions(ctx, "quiz-1")
	if err != nil {
		t.Fatalf("GetQuizQuestions failed: %v", err)
	}
	if len(gotQuestions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(gotQuestions))
	}
	if gotQuestions[0].QuestionID != "q1" || gotQuestions[1].QuestionID != "q2" {
		t.Fatalf("question order not preserved: %+v", gotQuestions)
	}

	exists, err := store.QuizExists(ctx, "quiz-1")
	if err != nil {
		t.Fatalf("QuizExists failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected quiz to exist")
	}

	_, err = store.GetQuizMetadata(ctx, "missing")
	if !errors.Is(err, ErrQuizNotFound) {
		t.Fatalf("expected ErrQuizNotFound for missing metadata, got %v", err)
	}
	_, err = store.GetQuizQuestions(ctx, "missing")
	if !errors.Is(err, ErrQuizNotFound) {
		t.Fatalf("expected ErrQuizNotFound for missing questions, got %v", err)
	}
}

func TestSQLiteStoreCreateQuizOverwriteClearsAttempts(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	if err := store.CreateQuiz(ctx, QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 2,
		CreatedAt:     time.Unix(1700000100, 0).UTC(),
	}, sampleQuestions()); err != nil {
		t.Fatalf("CreateQuiz initial failed: %v", err)
	}

	results, err := store.SubmitResponses(ctx, "quiz-1", "alice", []SubmittedResponse{
		{QuestionID: "q1", Answer: "A"},
	})
	if err != nil {
		t.Fatalf("SubmitResponses failed: %v", err)
	}
	if len(results) != 1 || results[0].Status != StatusCorrect {
		t.Fatalf("unexpected submit results: %+v", results)
	}

	newQuestions := []Question{
		{
			PublicQuestion: PublicQuestion{
				QuestionID: "q-new",
				Question:   "New question",
				Options: []Option{
					{Letter: "A", Text: "Yes"},
					{Letter: "B", Text: "No"},
				},
			},
			CorrectIndex: 0,
		},
	}
	if err := store.CreateQuiz(ctx, QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 1,
		CreatedAt:     time.Unix(1700000200, 0).UTC(),
	}, newQuestions); err != nil {
		t.Fatalf("CreateQuiz overwrite failed: %v", err)
	}

	questions, err := store.GetQuizQuestions(ctx, "quiz-1")
	if err != nil {
		t.Fatalf("GetQuizQuestions after overwrite failed: %v", err)
	}
	if len(questions) != 1 || questions[0].QuestionID != "q-new" {
		t.Fatalf("expected overwritten quiz questions, got %+v", questions)
	}

	scores, err := store.GetAttemptScores(ctx, "quiz-1", "alice")
	if err != nil {
		t.Fatalf("GetAttemptScores failed: %v", err)
	}
	if len(scores) != 0 {
		t.Fatalf("expected attempts reset on overwrite, got %+v", scores)
	}
}

func TestSQLiteStoreSubmitResponsesStatusesAndDuplicate(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	if err := store.CreateQuiz(ctx, QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 2,
		CreatedAt:     time.Unix(1700000300, 0).UTC(),
	}, sampleQuestions()); err != nil {
		t.Fatalf("CreateQuiz failed: %v", err)
	}

	results, err := store.SubmitResponses(ctx, "quiz-1", "alice", []SubmittedResponse{
		{QuestionID: "q1", Answer: "A"},
		{QuestionID: "q2", Answer: "A"},
		{QuestionID: "q2", Answer: "ZZ"},
		{QuestionID: "missing", Answer: "A"},
	})
	if err != nil {
		t.Fatalf("SubmitResponses failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	if results[0].Status != StatusCorrect {
		t.Fatalf("expected q1 status correct, got %q", results[0].Status)
	}
	if results[1].Status != StatusIncorrect {
		t.Fatalf("expected q2 status incorrect, got %q", results[1].Status)
	}
	if results[2].Status != StatusInvalidLetter {
		t.Fatalf("expected invalid letter, got %q", results[2].Status)
	}
	if results[3].Status != StatusInvalidQuestion {
		t.Fatalf("expected invalid question, got %q", results[3].Status)
	}

	duplicate, err := store.SubmitResponses(ctx, "quiz-1", "alice", []SubmittedResponse{
		{QuestionID: "q1", Answer: "B"},
	})
	if err != nil {
		t.Fatalf("SubmitResponses duplicate failed: %v", err)
	}
	if len(duplicate) != 1 {
		t.Fatalf("expected 1 duplicate result, got %d", len(duplicate))
	}
	if duplicate[0].Status != StatusAlreadyAnswered {
		t.Fatalf("expected already_answered, got %q", duplicate[0].Status)
	}
	if duplicate[0].AttemptScore == nil || *duplicate[0].AttemptScore != 1.0 {
		t.Fatalf("expected attempt_score=1.0 for duplicate, got %+v", duplicate[0].AttemptScore)
	}
}

func TestSQLiteStoreGetLeaderboardOrdering(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	if err := store.CreateQuiz(ctx, QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 2,
		CreatedAt:     time.Unix(1700000400, 0).UTC(),
	}, sampleQuestions()); err != nil {
		t.Fatalf("CreateQuiz failed: %v", err)
	}

	// Insert deterministic leaderboard data directly to control timestamps/order.
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO attempts (quiz_id, question_id, username_norm, answer_letter, score, submitted_at_unix) VALUES
		('quiz-1', 'q1', 'bob',   'A', 1.0, 300),
		('quiz-1', 'q2', 'bob',   'B', 1.0, 400),
		('quiz-1', 'q1', 'alice', 'A', 1.0, 100),
		('quiz-1', 'q2', 'alice', 'B', 1.0, 200),
		('quiz-1', 'q1', 'carol', 'A', 1.0, 500),
		('quiz-1', 'q2', 'dave',  'A', 1.0, 500)
	`)
	if err != nil {
		t.Fatalf("seed attempts failed: %v", err)
	}

	board, err := store.GetLeaderboard(ctx, "quiz-1")
	if err != nil {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	if len(board) != 4 {
		t.Fatalf("expected 4 leaderboard rows, got %d", len(board))
	}

	// Scores: alice=2, bob=2, carol=1, dave=1.
	// Tie 2 points: earlier last_submission first => alice(200) before bob(400).
	// Tie 1 point + same timestamp(500): lexical username => carol before dave.
	gotOrder := []string{board[0].Username, board[1].Username, board[2].Username, board[3].Username}
	wantOrder := []string{"alice", "bob", "carol", "dave"}
	for idx := range wantOrder {
		if gotOrder[idx] != wantOrder[idx] {
			t.Fatalf("unexpected leaderboard order: got %v want %v", gotOrder, wantOrder)
		}
	}

	_, err = store.GetLeaderboard(ctx, "missing")
	if !errors.Is(err, ErrQuizNotFound) {
		t.Fatalf("expected ErrQuizNotFound for missing quiz leaderboard, got %v", err)
	}
}

func TestSQLiteStoreGetAttemptScores(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	if err := store.CreateQuiz(ctx, QuizMetadata{
		QuizID:        "quiz-1",
		QuestionCount: 2,
		CreatedAt:     time.Unix(1700000500, 0).UTC(),
	}, sampleQuestions()); err != nil {
		t.Fatalf("CreateQuiz failed: %v", err)
	}

	_, err := store.SubmitResponses(ctx, "quiz-1", "alice", []SubmittedResponse{
		{QuestionID: "q1", Answer: "A"},
		{QuestionID: "q2", Answer: "A"},
	})
	if err != nil {
		t.Fatalf("SubmitResponses failed: %v", err)
	}

	scores, err := store.GetAttemptScores(ctx, "quiz-1", "alice")
	if err != nil {
		t.Fatalf("GetAttemptScores failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	if scores["q1"] != 1.0 {
		t.Fatalf("expected q1 score 1.0, got %v", scores["q1"])
	}
	if scores["q2"] != 0.0 {
		t.Fatalf("expected q2 score 0.0, got %v", scores["q2"])
	}
}

func TestSQLiteStoreListActiveQuizzes(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	for idx := 0; idx < 12; idx++ {
		quizID := "quiz-" + time.Unix(int64(idx), 0).UTC().Format("150405.000000000")
		err := store.CreateQuiz(ctx, QuizMetadata{
			QuizID:        quizID,
			QuestionCount: 1,
			CreatedAt:     time.Unix(int64(100+idx), 0).UTC(),
		}, []Question{
			{
				PublicQuestion: PublicQuestion{
					QuestionID: "q" + quizID,
					Question:   "Prompt",
					Options: []Option{
						{Letter: "A", Text: "One"},
					},
				},
				CorrectIndex: 0,
			},
		})
		if err != nil {
			t.Fatalf("CreateQuiz #%d failed: %v", idx, err)
		}
	}

	// limit<=0 defaults to 10 rows.
	active, err := store.ListActiveQuizzes(ctx, 0)
	if err != nil {
		t.Fatalf("ListActiveQuizzes default failed: %v", err)
	}
	if len(active) != 10 {
		t.Fatalf("expected default 10 quizzes, got %d", len(active))
	}

	// Ensure descending creation order.
	for idx := 1; idx < len(active); idx++ {
		if active[idx-1].CreatedAt.Before(active[idx].CreatedAt) {
			t.Fatalf("active quizzes not sorted desc by created_at: %+v", active)
		}
	}

	top3, err := store.ListActiveQuizzes(ctx, 3)
	if err != nil {
		t.Fatalf("ListActiveQuizzes(3) failed: %v", err)
	}
	if len(top3) != 3 {
		t.Fatalf("expected 3 quizzes, got %d", len(top3))
	}
}
