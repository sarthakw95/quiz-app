package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"quiz-app/internal/opentdb"
	"quiz-app/internal/quiz"
)

const (
	maxAttempts   = 3
	questionCount = 10
)

// Run executes a complete single-player quiz session in the terminal.
//
// Why this function is structured as an orchestration flow:
//   - It keeps domain transformation (`quiz.BuildQuestions`) separate from transport
//     concerns (`opentdb.FetchQuestions`) and presentation (`printQuestion`).
//   - It keeps scoring local and explicit (`score` integer) so the session behavior
//     is easy to reason about and explain during review/presentation.
//   - It treats invalid/failed input for a single question as a skip (not fatal),
//     but treats upstream fetch failure as fatal because no quiz can proceed without
//     source questions.
//
// Behavior summary:
// 1. Fetch and normalize questions.
// 2. Iterate question-by-question, prompting for one option letter.
// 3. Allow up to maxAttempts invalid inputs per question.
// 4. Score only successfully answered questions; skipped questions reveal the answer.
// 5. Print final score against total fetched questions.
func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	// The CLI intentionally fetches fresh questions for each run instead of caching.
	// This keeps the command stateless and avoids persistence concerns in this mode.
	rawQuestions, err := opentdb.FetchQuestions(ctx, questionCount)
	if err != nil {
		return err
	}

	// Transform third-party response shape into local domain shape once, so the rest
	// of the flow only depends on internal quiz models.
	questions := quiz.BuildQuestions(rawQuestions)
	reader := bufio.NewReader(in)
	score := 0

	for idx, question := range questions {
		printQuestion(out, idx+1, question)

		chosenIndex, ok := getAnswer(reader, out, len(question.Options))
		fmt.Fprintln(out)
		correctText := optionTextForIndex(question.Options, question.CorrectIndex)
		if !ok {
			// After repeated invalid input, treat the question as skipped to keep quiz
			// progress moving rather than blocking the session.
			fmt.Fprintf(out, "Skipping. Correct answer was %s\n\n", correctText)
			continue
		}

		if chosenIndex == question.CorrectIndex {
			fmt.Fprintln(out, "Correct!")
			score++
		} else {
			fmt.Fprintf(out, "Wrong. Correct answer was %s\n", correctText)
		}

		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "\nFinal score: %d/%d\n", score, len(questions))
	return nil
}

// printQuestion renders one question and its options in a consistent format.
func printQuestion(out io.Writer, number int, question quiz.Question) {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Q%d: %s\n\n", number, question.Question)
	for _, option := range question.Options {
		fmt.Fprintf(out, "%s. %s\n", option.Letter, option.Text)
	}
	fmt.Fprintln(out)
}

// getAnswer reads a single-letter option from stdin and validates it against the
// available option range (A..max). It returns (index, true) on success.
// maxAttempts deliberately caps retries so malformed input cannot trap the CLI in
// an infinite prompt loop. On repeated invalid input or read failure it returns
// (-1, false).
func getAnswer(reader *bufio.Reader, out io.Writer, optionCount int) (int, bool) {
	if optionCount < 1 {
		return -1, false
	}

	maxLetter := byte('A' + optionCount - 1)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		userAnswer, err := reader.ReadString('\n')
		if err != nil {
			return -1, false
		}

		userAnswer = strings.ToUpper(strings.TrimSpace(userAnswer))
		if len(userAnswer) == 1 {
			letter := userAnswer[0]
			if letter >= 'A' && letter <= maxLetter {
				return int(letter - 'A'), true
			}
		}

		if attempt < maxAttempts {
			fmt.Fprintf(out, "\nInvalid input. Please enter a letter A-%c.\n", maxLetter)
		}
	}

	return -1, false
}

// optionTextForIndex safely resolves option text by index.
func optionTextForIndex(options []quiz.Option, index int) string {
	if index < 0 || index >= len(options) {
		return ""
	}
	return options[index].Text
}
