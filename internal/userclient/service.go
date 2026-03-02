package userclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServer            = "http://127.0.0.1:8080"
	defaultListLimit         = 10
	defaultLeaderboardLimit  = 10
	defaultQuestionCount     = 10
	defaultHTTPTimeout       = 5 * time.Second
	defaultPersistTimeout    = 2 * time.Second
	defaultMaxInvalidAnswers = 3
)

type Config struct {
	Username          string
	ServerURL         string
	ListLimit         int
	LeaderboardLimit  int
	MaxInvalidAnswers int
	HTTPTimeout       time.Duration
}

func Run(ctx context.Context, in io.Reader, out io.Writer, cfg Config) error {
	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		return errors.New("username is required")
	}

	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		serverURL = defaultServer
	}

	listLimit := cfg.ListLimit
	if listLimit <= 0 {
		listLimit = defaultListLimit
	}
	leaderboardLimit := cfg.LeaderboardLimit
	if leaderboardLimit == 0 {
		leaderboardLimit = defaultLeaderboardLimit
	}
	maxInvalidAnswers := cfg.MaxInvalidAnswers
	if maxInvalidAnswers <= 0 {
		maxInvalidAnswers = defaultMaxInvalidAnswers
	}
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	client := NewHTTPClient(serverURL, &http.Client{Timeout: timeout})
	reader := bufio.NewReader(in)

	fmt.Fprintf(out, "quiz-user-service\nusername=%s\nserver=%s\n\n", username, serverURL)
	printHelp(out)

	for {
		fmt.Fprint(out, "\n> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(out)
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		args := strings.Fields(line)
		command := strings.ToLower(args[0])

		switch command {
		case "help":
			printHelp(out)
		case "exit":
			return nil
		case "quizzes":
			limit, parseErr := parsePositiveLimit(args, 1, listLimit)
			if parseErr != nil {
				fmt.Fprintf(out, "invalid quizzes limit: %v\n", parseErr)
				continue
			}
			if err := runList(ctx, out, client, limit, serverURL); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		case "leaderboard":
			if len(args) < 2 {
				fmt.Fprintln(out, "usage: leaderboard <quiz_id> [limit]")
				continue
			}
			limit, parseErr := parseSignedLimit(args, 2, leaderboardLimit)
			if parseErr != nil {
				fmt.Fprintf(out, "invalid leaderboard limit: %v\n", parseErr)
				continue
			}
			if err := runLeaderboard(ctx, out, client, args[1], limit, serverURL); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		case "play":
			if len(args) != 2 {
				fmt.Fprintln(out, "usage: play <quiz_id>")
				continue
			}
			if err := runPlay(ctx, reader, out, client, username, args[1], maxInvalidAnswers, serverURL); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		default:
			fmt.Fprintln(out, "unknown command. type 'help' for usage.")
		}
	}
}

func runList(ctx context.Context, out io.Writer, client *HTTPClient, limit int, serverURL string) error {
	quizzes, err := client.ListActiveQuizzes(ctx, limit)
	if err != nil {
		return describeClientError(err, serverURL)
	}

	if len(quizzes) == 0 {
		fmt.Fprintln(out, "No active quizzes.")
		return nil
	}

	fmt.Fprintln(out, "Active quizzes:")
	for idx, item := range quizzes {
		fmt.Fprintf(out, "%d. %s (%d questions, created %s)\n",
			idx+1,
			item.QuizID,
			item.QuestionCount,
			item.CreatedAt.Format(time.RFC3339),
		)
	}
	return nil
}

func runLeaderboard(ctx context.Context, out io.Writer, client *HTTPClient, quizID string, limit int, serverURL string) error {
	entries, err := client.GetLeaderboard(ctx, quizID, limit)
	if err != nil {
		return describeClientError(err, serverURL)
	}

	if len(entries) == 0 {
		fmt.Fprintf(out, "No leaderboard entries for quiz %s.\n", quizID)
		return nil
	}

	fmt.Fprintf(out, "Leaderboard for %s:\n", quizID)
	for idx, entry := range entries {
		fmt.Fprintf(out, "%d. %s score=%s answered=%d last=%s\n",
			idx+1,
			entry.Username,
			formatScore(entry.TotalScore),
			entry.AnsweredCount,
			entry.LastSubmissionAt.Format(time.RFC3339),
		)
	}
	return nil
}

func runPlay(ctx context.Context, reader *bufio.Reader, out io.Writer, client *HTTPClient, username, quizID string, maxInvalidAnswers int, serverURL string) error {
	payload, err := client.GetQuizQuestions(ctx, quizID, username, false, 0)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			createNew, promptErr := promptYesNo(reader, out, "quiz not found. create a new quiz? (yes/no): ")
			if promptErr != nil {
				return promptErr
			}
			if !createNew {
				return nil
			}

			// Reuse the requested quiz_id so multiple users can converge on the same
			// shareable quiz identifier after a coordinated "create if missing" flow.
			payload, err = client.GetQuizQuestions(ctx, quizID, username, true, defaultQuestionCount)
			if err != nil {
				return describeClientError(err, serverURL)
			}
			return runPlayWithPayload(reader, out, client, username, payload, maxInvalidAnswers)
		}
		return describeClientError(err, serverURL)
	}
	return runPlayWithPayload(reader, out, client, username, payload, maxInvalidAnswers)
}

func runPlayWithPayload(reader *bufio.Reader, out io.Writer, client *HTTPClient, username string, payload questionsResponse, maxInvalidAnswers int) error {
	fmt.Fprintf(out, "quiz_id=%s\n", payload.QuizID)

	// Intentional tradeoff: score is computed client-side for a simpler demo flow.
	// The server still persists attempts, but this local score is treated as UX-only.
	oldPossible := 0.0
	oldScore := 0.0
	fresh := make([]questionItem, 0, len(payload.Questions))
	for _, item := range payload.Questions {
		// Treat either signal as attempted to remain compatible with incremental API evolution.
		attempted := item.AttemptStatus == attemptStatusAlreadyAttempt || item.AttemptScore != nil
		if attempted {
			oldPossible += 1.0
			if item.AttemptScore != nil {
				oldScore += *item.AttemptScore
			}
			continue
		}
		fresh = append(fresh, item)
	}

	if len(fresh) == 0 {
		fmt.Fprintf(out, "quiz %s is already attempted.\n", payload.QuizID)
		if oldPossible > 0 {
			fmt.Fprintf(out, "Score: %s/%s\n", formatScore(oldScore), formatScore(oldPossible))
		} else {
			fmt.Fprintln(out, "No scored attempts in this run.")
		}
		return nil
	}

	newPossible := 0.0
	newScore := 0.0

	for _, question := range fresh {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "%s\n\n", question.Question)
		for _, option := range question.Options {
			fmt.Fprintf(out, "%s. %s\n", option.Letter, option.Text)
		}
		fmt.Fprintln(out)

		invalidCount := 0
		for {
			answer, ok := promptAnswer(reader, out, len(question.Options))
			if !ok {
				invalidCount++
				if invalidCount >= maxInvalidAnswers {
					fmt.Fprintln(out, "Skipping question after multiple invalid responses.")
					break
				}
				fmt.Fprintf(out, "Invalid input. Attempts remaining: %d\n", maxInvalidAnswers-invalidCount)
				continue
			}

			answerIndex := int(answer[0] - 'A')
			// Invalid/auto-skipped questions are excluded from denominator by design.
			newPossible += 1.0
			if answerIndex == question.CorrectIndex {
				newScore += 1.0
				fmt.Fprintln(out, "Correct!")
			} else {
				fmt.Fprintln(out, "Wrong.")
			}

			fireAndForgetPersistence(client, payload.QuizID, username, question.QuestionID, answer)
			break
		}
	}

	combinedPossible := oldPossible + newPossible
	combinedScore := oldScore + newScore
	fmt.Fprintln(out)
	if combinedPossible > 0 {
		fmt.Fprintf(out, "Score: %s/%s\n", formatScore(combinedScore), formatScore(combinedPossible))
	} else {
		fmt.Fprintln(out, "No scored attempts in this run.")
	}
	return nil
}

func fireAndForgetPersistence(client *HTTPClient, quizID, username, questionID, answer string) {
	// Intentional tradeoff: best-effort persistence per question to reduce loss on mid-quiz disconnects.
	// These async writes can complete out of order, but each (quiz,question,user) key is idempotent on server.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultPersistTimeout)
		defer cancel()
		_ = client.PersistSingleResponse(ctx, quizID, username, questionID, answer)
	}()
}

func promptAnswer(reader *bufio.Reader, out io.Writer, optionCount int) (string, bool) {
	if optionCount < 1 {
		return "", false
	}

	maxLetter := byte('A' + optionCount - 1)
	fmt.Fprintf(out, "Your answer (A-%c): ", maxLetter)

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", false
	}

	answer := strings.ToUpper(strings.TrimSpace(line))
	if len(answer) != 1 {
		return "", false
	}
	letter := answer[0]
	if letter < 'A' || letter > maxLetter {
		return "", false
	}

	return answer, true
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  help")
	fmt.Fprintln(out, "  quizzes [limit]")
	fmt.Fprintln(out, "  leaderboard <quiz_id> [limit]")
	fmt.Fprintln(out, "  play <quiz_id>")
	fmt.Fprintln(out, "  exit")
}

func parsePositiveLimit(args []string, index int, defaultValue int) (int, error) {
	if len(args) <= index {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil || value <= 0 {
		return 0, errors.New("must be a positive integer")
	}
	return value, nil
}

func parseSignedLimit(args []string, index int, defaultValue int) (int, error) {
	if len(args) <= index {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, errors.New("must be an integer")
	}
	return value, nil
}

func formatScore(score float64) string {
	return strconv.FormatFloat(score, 'f', -1, 64)
}

func promptYesNo(reader *bufio.Reader, out io.Writer, prompt string) (bool, error) {
	for {
		fmt.Fprint(out, prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "Please answer yes or no.")
		}
	}
}

func describeClientError(err error, serverURL string) error {
	if errors.Is(err, ErrServiceUnavailable) {
		return fmt.Errorf("quiz service unavailable at %s", serverURL)
	}
	return err
}
