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

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	rawQuestions, err := opentdb.FetchQuestions(ctx, questionCount)
	if err != nil {
		return err
	}

	questions := quiz.BuildQuestions(rawQuestions)
	reader := bufio.NewReader(in)
	score := 0

	for idx, question := range questions {
		printQuestion(out, idx+1, question)

		chosenIndex, ok := getAnswer(reader, out, len(question.Options))
		fmt.Fprintln(out)
		correctText := optionTextForIndex(question.Options, question.CorrectIndex)
		if !ok {
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

func printQuestion(out io.Writer, number int, question quiz.Question) {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Q%d: %s\n\n", number, question.Question)
	for _, option := range question.Options {
		fmt.Fprintf(out, "%s. %s\n", option.Letter, option.Text)
	}
	fmt.Fprintln(out)
}

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

func optionTextForIndex(options []quiz.Option, index int) string {
	if index < 0 || index >= len(options) {
		return ""
	}
	return options[index].Text
}
