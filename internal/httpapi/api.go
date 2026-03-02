package httpapi

import "quiz-app/internal/quiz"

type API struct {
	bank    *quiz.Bank
	service *quiz.Service
}

func NewAPI(service *quiz.Service, bank *quiz.Bank) *API {
	if bank == nil {
		bank = quiz.NewBank()
	}
	return &API{
		bank:    bank,
		service: service,
	}
}
