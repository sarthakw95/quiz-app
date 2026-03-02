package sqlite

import (
	"context"
	"time"

	"quiz-app/internal/quiz"
)

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
func (s *SQLiteStore) SubmitResponses(ctx context.Context, quizID, usernameNormalized string, responses []quiz.SubmittedResponse) ([]quiz.ResponseResult, error) {
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
		return nil, quiz.ErrQuizNotFound
	}

	results := make([]quiz.ResponseResult, 0, len(responses))
	for _, response := range responses {
		key, ok := questionLookup[response.QuestionID]
		if !ok {
			results = append(results, quiz.ResponseResult{
				QuestionID: response.QuestionID,
				Status:     quiz.StatusInvalidQuestion,
			})
			continue
		}

		letter := quiz.NormalizeLetter(response.Answer)
		if letter == "" {
			results = append(results, quiz.ResponseResult{
				QuestionID: response.QuestionID,
				Status:     quiz.StatusInvalidLetter,
			})
			continue
		}

		answerIndex := int(letter[0] - 'A')
		if answerIndex < 0 || answerIndex >= key.optionCount {
			results = append(results, quiz.ResponseResult{
				QuestionID: response.QuestionID,
				Status:     quiz.StatusInvalidLetter,
			})
			continue
		}

		status := quiz.StatusIncorrect
		score := 0.0
		if answerIndex == key.correctIndex {
			status = quiz.StatusCorrect
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
			status = quiz.StatusAlreadyAnswered

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

		results = append(results, quiz.ResponseResult{
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

func (s *SQLiteStore) GetLeaderboard(ctx context.Context, quizID string) ([]quiz.LeaderboardEntry, error) {
	exists, err := s.QuizExists(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, quiz.ErrQuizNotFound
	}

	/// Returning all leaderboard entries is intentional for this demo. This simplifies
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

	leaderboard := make([]quiz.LeaderboardEntry, 0)
	for rows.Next() {
		var (
			entry            quiz.LeaderboardEntry
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
