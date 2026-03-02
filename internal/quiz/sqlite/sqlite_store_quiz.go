package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"quiz-app/internal/quiz"
)

func (s *SQLiteStore) CreateQuiz(ctx context.Context, metadata quiz.QuizMetadata, questions []quiz.Question) error {
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
			question.QuestionID = quiz.MakeQuestionID(question)
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

func (s *SQLiteStore) GetQuizMetadata(ctx context.Context, quizID string) (quiz.QuizMetadata, error) {
	var metadata quiz.QuizMetadata
	var createdAtUnix int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT quiz_id, question_count, created_at_unix FROM quizzes WHERE quiz_id = ?`,
		quizID,
	).Scan(&metadata.QuizID, &metadata.QuestionCount, &createdAtUnix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return quiz.QuizMetadata{}, quiz.ErrQuizNotFound
		}
		return quiz.QuizMetadata{}, err
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

func (s *SQLiteStore) GetQuizQuestions(ctx context.Context, quizID string) ([]quiz.Question, error) {
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

	questions := make([]quiz.Question, 0)
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

		var options []quiz.Option
		if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
			return nil, err
		}

		questions = append(questions, quiz.Question{
			PublicQuestion: quiz.PublicQuestion{
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
			return nil, quiz.ErrQuizNotFound
		}
	}

	return questions, nil
}

func (s *SQLiteStore) ListActiveQuizzes(ctx context.Context, limit int) ([]quiz.QuizMetadata, error) {
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

	active := make([]quiz.QuizMetadata, 0)
	for rows.Next() {
		var (
			item          quiz.QuizMetadata
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
