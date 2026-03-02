package quiz

import (
	"crypto/sha1"
	"encoding/hex"
	"html"
	"math/rand"
	"strings"
	"sync"
	"time"

	"quiz-app/internal/opentdb"
)

const (
	StatusCorrect         = "correct"
	StatusIncorrect       = "incorrect"
	StatusInvalidQuestion = "invalid_question"
	StatusInvalidLetter   = "invalid_letter"
	StatusAlreadyAnswered = "already_answered"
)

type Option struct {
	Letter string `json:"letter"`
	Text   string `json:"text"`
}

type Question struct {
	PublicQuestion
	CorrectIndex int
}

type PublicQuestion struct {
	QuestionID string   `json:"question_id"`
	Question   string   `json:"question"`
	Options    []Option `json:"options"`
}

type SubmittedResponse struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

type ResponseResult struct {
	QuestionID   string   `json:"question_id"`
	Status       string   `json:"status"`
	AttemptScore *float64 `json:"attempt_score,omitempty"`
}

type Bank struct {
	questions sync.Map
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewBank() *Bank {
	return &Bank{}
}

func BuildQuestions(raw []opentdb.RawQuestion) []Question {
	questions := make([]Question, 0, len(raw))
	for _, item := range raw {
		question := buildQuestion(item)
		question.QuestionID = makeQuestionID(question)
		questions = append(questions, question)
	}
	return questions
}

func (b *Bank) AddQuestions(raw []opentdb.RawQuestion) []Question {
	questions := make([]Question, 0, len(raw))

	for _, item := range raw {
		question := buildQuestion(item)
		question.QuestionID = makeQuestionID(question)
		b.questions.Store(question.QuestionID, question)
		questions = append(questions, question)
	}

	return questions
}

func (b *Bank) AddBuiltQuestions(questions []Question) {
	for _, question := range questions {
		if question.QuestionID == "" {
			question.QuestionID = makeQuestionID(question)
		}
		b.questions.Store(question.QuestionID, question)
	}
}

func (b *Bank) EvaluateResponses(responses []SubmittedResponse) []ResponseResult {
	results := make([]ResponseResult, 0, len(responses))

	for _, response := range responses {
		storedQuestion, ok := b.questions.Load(response.QuestionID)
		if !ok {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidQuestion,
			})
			continue
		}

		question, ok := storedQuestion.(Question)
		if !ok {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidQuestion,
			})
			continue
		}

		if len(question.Options) < 1 {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidLetter,
			})
			continue
		}

		letter := normalizeLetter(response.Answer)
		if letter == "" {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidLetter,
			})
			continue
		}

		answerIndex := int(letter[0] - 'A')
		if answerIndex < 0 || answerIndex >= len(question.Options) {
			results = append(results, ResponseResult{
				QuestionID: response.QuestionID,
				Status:     StatusInvalidLetter,
			})
			continue
		}

		status := StatusIncorrect
		if answerIndex == question.CorrectIndex {
			status = StatusCorrect
		}
		results = append(results, ResponseResult{
			QuestionID: response.QuestionID,
			Status:     status,
		})
	}

	return results
}

func ToPublicQuestions(questions []Question) []PublicQuestion {
	public := make([]PublicQuestion, 0, len(questions))
	for _, question := range questions {
		public = append(public, question.PublicQuestion)
	}
	return public
}

func makeQuestionID(question Question) string {
	var keyBuilder strings.Builder
	keyBuilder.WriteString(question.Question)
	for _, option := range question.Options {
		keyBuilder.WriteString("|")
		keyBuilder.WriteString(option.Text)
	}

	hash := sha1.Sum([]byte(keyBuilder.String()))
	return "q_" + hex.EncodeToString(hash[:])
}

func normalizeLetter(answer string) string {
	letter := strings.ToUpper(strings.TrimSpace(answer))
	if len(letter) != 1 {
		return ""
	}
	return letter
}

func buildQuestion(raw opentdb.RawQuestion) Question {
	type choice struct {
		text      string
		isCorrect bool
	}

	choices := make([]choice, 0, len(raw.IncorrectAnswers)+1)
	for _, incorrect := range raw.IncorrectAnswers {
		choices = append(choices, choice{
			text:      html.UnescapeString(incorrect),
			isCorrect: false,
		})
	}

	choices = append(choices, choice{
		text:      html.UnescapeString(raw.CorrectAnswer),
		isCorrect: true,
	})

	rand.Shuffle(len(choices), func(i, j int) {
		choices[i], choices[j] = choices[j], choices[i]
	})

	options := make([]Option, len(choices))
	correctIndex := -1

	for idx, candidate := range choices {
		letter := string(rune('A' + idx))
		options[idx] = Option{
			Letter: letter,
			Text:   candidate.text,
		}
		if candidate.isCorrect {
			correctIndex = idx
		}
	}

	return Question{
		PublicQuestion: PublicQuestion{
			Question: html.UnescapeString(raw.Question),
			Options:  options,
		},
		CorrectIndex: correctIndex,
	}
}
