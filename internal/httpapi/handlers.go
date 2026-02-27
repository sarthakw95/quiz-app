package httpapi

import (
	"encoding/json"
	"net/http"

	"quiz-app/internal/opentdb"
	"quiz-app/internal/quiz"
)

const questionFetchCount = 10

type API struct {
	bank *quiz.Bank
}

type questionsResponse struct {
	Questions []quiz.PublicQuestion `json:"questions"`
}

type responsesRequest struct {
	Responses []quiz.SubmittedResponse `json:"responses"`
}

type responsesResponse struct {
	Results []quiz.ResponseResult `json:"results"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewAPI(bank *quiz.Bank) *API {
	if bank == nil {
		bank = quiz.NewBank()
	}
	return &API{
		bank: bank,
	}
}

func (a *API) HandleQuestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	rawQuestions, err := opentdb.FetchQuestions(r.Context(), questionFetchCount)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch questions"})
		return
	}

	questions := a.bank.AddQuestions(rawQuestions)
	writeJSON(w, http.StatusOK, questionsResponse{
		Questions: quiz.ToPublicQuestions(questions),
	})
}

func (a *API) HandleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	defer r.Body.Close()

	var request responsesRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	if request.Responses == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "responses is required"})
		return
	}

	results := a.bank.EvaluateResponses(request.Responses)
	writeJSON(w, http.StatusOK, responsesResponse{
		Results: results,
	})
}

func writeMethodNotAllowed(w http.ResponseWriter, allowedMethod string) {
	w.Header().Set("Allow", allowedMethod)
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
