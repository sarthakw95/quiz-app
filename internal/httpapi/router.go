package httpapi

import (
	"bytes"
	"log"
	"net/http"
	"time"

	"quiz-app/internal/quiz"
)

func NewRouter(service *quiz.Service, bank *quiz.Bank) http.Handler {
	return NewRouterWithOptions(service, bank, RouterOptions{})
}

type RouterOptions struct {
	Debug bool
}

func NewRouterWithOptions(service *quiz.Service, bank *quiz.Bank, options RouterOptions) http.Handler {
	api := NewAPI(service, bank)

	mux := http.NewServeMux()
	mux.HandleFunc("/questions", api.HandleQuestions)
	mux.HandleFunc("/responses", api.HandleResponses)
	mux.HandleFunc("/quizzes", api.HandleCreateQuiz)
	mux.HandleFunc("/quizzes/active", api.HandleActiveQuizzes)
	mux.HandleFunc("/quizzes/{quiz_id}/questions", api.HandleQuizQuestions)
	mux.HandleFunc("/quizzes/{quiz_id}/leaderboard", api.HandleLeaderboard)

	if !options.Debug {
		return mux
	}
	return debugRequestLoggingMiddleware(mux)
}

func debugRequestLoggingMiddleware(next http.Handler) http.Handler {
	const maxLoggedResponseBytes = 4096

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			maxLogBytes:    maxLoggedResponseBytes,
		}

		next.ServeHTTP(recorder, r)

		log.Printf(
			"request method=%s path=%s query=%q status=%d bytes=%d duration=%s remote=%s user_agent=%q response_body=%q truncated=%t",
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			recorder.statusCode,
			recorder.bytesWritten,
			time.Since(start).Round(time.Millisecond),
			r.RemoteAddr,
			r.UserAgent(),
			recorder.logBody.String(),
			recorder.truncated,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	logBody      bytes.Buffer
	maxLogBytes  int
	truncated    bool
}

func (s *statusRecorder) WriteHeader(statusCode int) {
	s.statusCode = statusCode
	s.ResponseWriter.WriteHeader(statusCode)
}

func (s *statusRecorder) Write(payload []byte) (int, error) {
	written, err := s.ResponseWriter.Write(payload)
	s.bytesWritten += written

	if s.maxLogBytes > 0 && !s.truncated {
		remaining := s.maxLogBytes - s.logBody.Len()
		if remaining > 0 {
			if written <= remaining {
				_, _ = s.logBody.Write(payload[:written])
			} else {
				_, _ = s.logBody.Write(payload[:remaining])
				s.truncated = true
			}
		} else {
			s.truncated = true
		}
	}

	return written, err
}
