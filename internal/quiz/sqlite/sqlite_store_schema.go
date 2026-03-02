package sqlite

import (
	"context"
)

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
