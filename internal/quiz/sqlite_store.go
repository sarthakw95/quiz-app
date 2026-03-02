package quiz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		path = "quiz.db"
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) initSchema(ctx context.Context) error {
	// Schema intentionally avoids FK constraints for this demo so quiz overwrite/reset
	// flows stay simple and fully controlled by application transactions.
	statements := []string{
		`CREATE TABLE IF NOT EXISTS quizzes (
			quiz_id TEXT PRIMARY KEY,
			created_at_unix INTEGER NOT NULL,
			question_count INTEGER NOT NULL,
			locked INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS questions (
			question_id TEXT PRIMARY KEY,
			prompt TEXT NOT NULL,
			options_json TEXT NOT NULL,
			correct_index INTEGER NOT NULL,
			option_count INTEGER NOT NULL,
			source TEXT NOT NULL,
			created_at_unix INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS quiz_questions (
			quiz_id TEXT NOT NULL,
			question_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			PRIMARY KEY (quiz_id, position),
			UNIQUE (quiz_id, question_id)
		);`,
		`CREATE TABLE IF NOT EXISTS attempts (
			quiz_id TEXT NOT NULL,
			question_id TEXT NOT NULL,
			username_norm TEXT NOT NULL,
			answer_letter TEXT NOT NULL,
			-- REAL keeps scoring model expandable (partial/negative marks) without migration.
			score REAL NOT NULL,
			submitted_at_unix INTEGER NOT NULL,
			PRIMARY KEY (quiz_id, question_id, username_norm)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_quizzes_created_at ON quizzes(created_at_unix DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_attempts_quiz_user ON attempts(quiz_id, username_norm);`,
		`CREATE INDEX IF NOT EXISTS idx_attempts_quiz_submitted_at ON attempts(quiz_id, submitted_at_unix);`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) CreateQuiz(ctx context.Context, metadata QuizMetadata, questions []Question) error {
	if metadata.QuizID == "" {
		return errors.New("quiz id is required")
	}

	if metadata.QuestionCount <= 0 {
		metadata.QuestionCount = len(questions)
	}

	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM quiz_questions WHERE quiz_id = ?`, metadata.QuizID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM attempts WHERE quiz_id = ?`, metadata.QuizID); err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT OR REPLACE INTO quizzes (quiz_id, created_at_unix, question_count, locked) VALUES (?, ?, ?, 0)`,
		metadata.QuizID,
		metadata.CreatedAt.UnixNano(),
		metadata.QuestionCount,
	)
	if err != nil {
		return err
	}

	for idx := range questions {
		question := questions[idx]
		if question.QuestionID == "" {
			question.QuestionID = makeQuestionID(question)
		}

		optionsJSON, err := json.Marshal(question.Options)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO questions (question_id, prompt, options_json, correct_index, option_count, source, created_at_unix)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(question_id) DO UPDATE SET
				prompt = excluded.prompt,
				options_json = excluded.options_json,
				correct_index = excluded.correct_index,
				option_count = excluded.option_count,
				source = excluded.source`,
			question.QuestionID,
			question.Question,
			string(optionsJSON),
			question.CorrectIndex,
			len(question.Options),
			"opentdb",
			metadata.CreatedAt.UnixNano(),
		)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO quiz_questions (quiz_id, question_id, position) VALUES (?, ?, ?)`,
			metadata.QuizID,
			question.QuestionID,
			idx,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetQuizMetadata(ctx context.Context, quizID string) (QuizMetadata, error) {
	var metadata QuizMetadata
	var createdAtUnix int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT quiz_id, question_count, created_at_unix FROM quizzes WHERE quiz_id = ?`,
		quizID,
	).Scan(&metadata.QuizID, &metadata.QuestionCount, &createdAtUnix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return QuizMetadata{}, ErrQuizNotFound
		}
		return QuizMetadata{}, err
	}

	metadata.CreatedAt = time.Unix(0, createdAtUnix).UTC()
	return metadata, nil
}

func (s *SQLiteStore) QuizExists(ctx context.Context, quizID string) (bool, error) {
	var found int
	err := s.db.QueryRowContext(
		ctx,
		`SELECT 1 FROM quizzes WHERE quiz_id = ? LIMIT 1`,
		quizID,
	).Scan(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) GetQuizQuestions(ctx context.Context, quizID string) ([]Question, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT q.question_id, q.prompt, q.options_json, q.correct_index
		 FROM quiz_questions qq
		 JOIN questions q ON q.question_id = qq.question_id
		 WHERE qq.quiz_id = ?
		 ORDER BY qq.position ASC`,
		quizID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	questions := make([]Question, 0)
	for rows.Next() {
		var (
			questionID   string
			prompt       string
			optionsJSON  string
			correctIndex int
		)
		if err := rows.Scan(&questionID, &prompt, &optionsJSON, &correctIndex); err != nil {
			return nil, err
		}

		var options []Option
		if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
			return nil, err
		}

		questions = append(questions, Question{
			PublicQuestion: PublicQuestion{
				QuestionID: questionID,
				Question:   prompt,
				Options:    options,
			},
			CorrectIndex: correctIndex,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(questions) == 0 {
		exists, err := s.QuizExists(ctx, quizID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrQuizNotFound
		}
	}

	return questions, nil
}

func (s *SQLiteStore) ListActiveQuizzes(ctx context.Context, limit int) ([]QuizMetadata, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT quiz_id, question_count, created_at_unix
		 FROM quizzes
		 ORDER BY created_at_unix DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	active := make([]QuizMetadata, 0)
	for rows.Next() {
		var (
			item          QuizMetadata
			createdAtUnix int64
		)
		if err := rows.Scan(&item.QuizID, &item.QuestionCount, &createdAtUnix); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(0, createdAtUnix).UTC()
		active = append(active, item)
	}

	return active, rows.Err()
}

type answerKey struct {
	correctIndex int
	optionCount  int
}

// SubmitResponses runs as a single transaction so each request gets consistent
// duplicate detection and score evaluation.
//
// Invariants:
//   - (quiz_id, question_id, username_norm) is unique in attempts.
//   - An existing attempt must never be overwritten.
//   - Unknown questions are ignored, invalid letters are rejected, and valid
//     first-time submissions are scored and persisted.
//
// Transaction rationale:
// We load quiz question metadata and insert attempts in one transaction so
// concurrent submits for the same key resolve deterministically using the
// primary-key constraint + INSERT OR IGNORE.
func (s *SQLiteStore) SubmitResponses(ctx context.Context, quizID, usernameNormalized string, responses []SubmittedResponse) ([]ResponseResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(
		ctx,
		`SELECT q.question_id, q.correct_index, q.option_count
		 FROM quiz_questions qq
		 JOIN questions q ON q.question_id = qq.question_id
		 WHERE qq.quiz_id = ?`,
		quizID,
	)
	if err != nil {
		return nil, err
	}

	questionLookup := make(map[string]answerKey)
	for rows.Next() {
		var (
			questionID   string
			correctIndex int
			optionCount  int
		)
		if err := rows.Scan(&questionID, &correctIndex, &optionCount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		questionLookup[questionID] = answerKey{
			correctIndex: correctIndex,
			optionCount:  optionCount,
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	if len(questionLookup) == 0 {
		return nil, ErrQuizNotFound
	}

	results := make([]ResponseResult, 0, len(responses))
	for _, response := range responses {
		key, ok := questionLookup[response.QuestionID]
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
		if answerIndex < 0 || answerIndex >= key.optionCount {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidLetter,
			})
			continue
		}

		status := StatusIncorrect
		score := 0.0
		if answerIndex == key.correctIndex {
			status = StatusCorrect
			score = 1.0
		}
		var attemptScore *float64

		insertResult, err := tx.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO attempts (quiz_id, question_id, username_norm, answer_letter, score, submitted_at_unix)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			quizID,
			response.QuestionID,
			usernameNormalized,
			letter,
			score,
			time.Now().UTC().UnixNano(),
		)
		if err != nil {
			return nil, err
		}

		inserted, err := insertResult.RowsAffected()
		if err != nil {
			return nil, err
		}
		if inserted == 0 {
			// Duplicate answer for (quiz, question, user): keep original row unchanged
			// and return previously persisted score for consistent client reconciliation.
			status = StatusAlreadyAnswered

			var existingScore float64
			if err := tx.QueryRowContext(
				ctx,
				`SELECT score FROM attempts
				 WHERE quiz_id = ? AND question_id = ? AND username_norm = ?
				 LIMIT 1`,
				quizID,
				response.QuestionID,
				usernameNormalized,
			).Scan(&existingScore); err != nil {
				return nil, err
			}
			attemptScore = &existingScore
		}

		results = append(results, ResponseResult{
			QuestionID:   response.QuestionID,
			Status:       status,
			AttemptScore: attemptScore,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return results, nil
}

func (s *SQLiteStore) GetLeaderboard(ctx context.Context, quizID string) ([]LeaderboardEntry, error) {
	exists, err := s.QuizExists(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrQuizNotFound
	}

	// Returning all leaderboard entries is intentional for this demo. This simplifies 
	// the leaderboard display logic and avoids pagination complexity and cache compatibility. 
	// It is possible that the size becomes very large, and the limit is used only to limit the number of entries displayed.
	// In production, it is recommended to use pagination to limit the number of entries displayed.
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT username_norm, SUM(score) AS total_score, COUNT(*) AS answered_count, MAX(submitted_at_unix) AS last_submission
		 FROM attempts
		 WHERE quiz_id = ?
		 GROUP BY username_norm
		 -- Keep ordering deterministic and aligned with in-memory cache comparison.
		 ORDER BY total_score DESC, last_submission ASC, username_norm ASC`,
		quizID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leaderboard := make([]LeaderboardEntry, 0)
	for rows.Next() {
		var (
			entry            LeaderboardEntry
			lastSubmissionNs int64
		)
		if err := rows.Scan(&entry.Username, &entry.TotalScore, &entry.AnsweredCount, &lastSubmissionNs); err != nil {
			return nil, err
		}
		entry.LastSubmissionAt = time.Unix(0, lastSubmissionNs).UTC()
		leaderboard = append(leaderboard, entry)
	}

	return leaderboard, rows.Err()
}

func (s *SQLiteStore) GetAttemptScores(ctx context.Context, quizID, usernameNormalized string) (map[string]float64, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT question_id, score
		 FROM attempts
		 WHERE quiz_id = ? AND username_norm = ?`,
		quizID,
		usernameNormalized,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scores := make(map[string]float64)
	for rows.Next() {
		var (
			questionID string
			score      float64
		)
		if err := rows.Scan(&questionID, &score); err != nil {
			return nil, err
		}
		scores[questionID] = score
	}

	return scores, rows.Err()
}

func (s *SQLiteStore) String() string {
	return fmt.Sprintf("sqlite_store(%T)", s.db)
}
