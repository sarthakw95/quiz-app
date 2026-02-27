package httpapi

import (
	"net/http"

	"quiz-app/internal/quiz"
)

func NewRouter(bank *quiz.Bank) http.Handler {
	api := NewAPI(bank)

	mux := http.NewServeMux()
	mux.HandleFunc("/questions", api.HandleQuestions)
	mux.HandleFunc("/responses", api.HandleResponses)

	return mux
}
