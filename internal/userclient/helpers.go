package userclient

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

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

func correctAnswerDisplay(question questionItem) string {
	if question.CorrectIndex < 0 || question.CorrectIndex >= len(question.Options) {
		return "unknown"
	}

	option := question.Options[question.CorrectIndex]
	if strings.TrimSpace(option.Letter) == "" {
		return option.Text
	}
	if strings.TrimSpace(option.Text) == "" {
		return option.Letter
	}
	return fmt.Sprintf("%s. %s", option.Letter, option.Text)
}
