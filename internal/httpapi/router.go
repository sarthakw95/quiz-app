package httpapi

import (
	"net/http"

	"quiz-app/internal/quiz"
)

func NewRouter(service *quiz.Service, bank *quiz.Bank) http.Handler {
	api := NewAPI(service, bank)

	mux := http.NewServeMux()
	mux.HandleFunc("/questions", api.HandleQuestions)
	mux.HandleFunc("/responses", api.HandleResponses)
	mux.HandleFunc("/quizzes", api.HandleCreateQuiz)
	mux.HandleFunc("/quizzes/active", api.HandleActiveQuizzes)
	mux.HandleFunc("/quizzes/{quiz_id}/questions", api.HandleQuizQuestions)
	mux.HandleFunc("/quizzes/{quiz_id}/leaderboard", api.HandleLeaderboard)

	return mux
}
