// Basic calling OpenTriviaDB
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"math/rand"
	"net/http"
	"strings"
	"time"
	"os"
)

const maxAttempts = 3

func printQuestion(num int, question string, options []string) {
	fmt.Println()
	fmt.Printf("Q%d: %s\n\n", num, question)
	for idx, opt := range options {
		fmt.Printf("%c. %s\n", 'A'+idx, opt)
	}
	fmt.Println()
}

func getAnswer(reader *bufio.Reader, numOptions int) (int, bool) {
	maxLetter := byte('A' + numOptions - 1)

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		user_answer, err := reader.ReadString('\n')
		if err != nil {
			// EOF or read error
			return -1, false
		}

		user_answer = strings.TrimSpace(strings.ToUpper(user_answer))

		if len(user_answer) == 1 {
			user_answer := user_answer[0]
			if user_answer >= 'A' && user_answer <= maxLetter {
				return int(user_answer - 'A'), true
			}
		}
		if attempt < maxAttempts {
			fmt.Printf("Invalid input Please enter a letter A-%c.\n", maxLetter)
		}
	}

	// Too many invalid attempts
	return -1, false
}

func main() {
	rand.Seed(time.Now().UnixNano())
	resp, err := http.Get("https://opentdb.com/api.php?amount=10")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]any
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	results, ok := data["results"].([]any)
	if !ok {
		fmt.Println("unexpected response")
		return
	}

	reader := bufio.NewReader(os.Stdin)
	score := 0

	for i, item := range results {
		q := item.(map[string]any)

		question, _ := q["question"].(string)
		question = html.UnescapeString(question)

		options := []string{}

		incorrect, _ := q["incorrect_answers"].([]any)
		for _, v := range incorrect {
			s, _ := v.(string)
			options = append(options, html.UnescapeString(s))
		}

		correct, _ := q["correct_answer"].(string)
		correct = html.UnescapeString(correct)
		options = append(options, correct)

		// Shuffle options
		rand.Shuffle(len(options), func(a, b int) {
			options[a], options[b] = options[b], options[a]
		})

		printQuestion(i+1, question, options)

		chosen_index, ok := getAnswer(reader, len(options))
		if !ok {
			fmt.Printf("Skipping. Correct answer was %s\n\n", correct)
			continue
		}

		chosen_answer := options[chosen_index]
		if chosen_answer == correct {
			fmt.Println("Correct!")
			score++
		} else {
			fmt.Printf("Wrong. Correct answer was %s\n", correct)
		}

		fmt.Println()
	}

	fmt.Printf("\nFinal score: %d/%d\n", score, len(results))
}