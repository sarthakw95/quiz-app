// Basic calling OpenTriviaDB
package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
)

func main() {
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

	for _, item := range results {
		q := item.(map[string]any)

		question, _ := q["question"].(string)
		fmt.Println(html.UnescapeString(question))
	}
}