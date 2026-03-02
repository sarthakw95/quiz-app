package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"quiz-app/internal/httpapi"
	"quiz-app/internal/opentdb"
	"quiz-app/internal/quiz"
	sqlitestore "quiz-app/internal/quiz/sqlite"
)

func main() {
	defaultAddr := os.Getenv("ADDR")
	if defaultAddr == "" {
		defaultAddr = ":8080"
	}

	defaultDBPath := os.Getenv("QUIZ_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = "quiz.db"
	}

	addr := flag.String("addr", defaultAddr, "HTTP listen address")
	dbPath := flag.String("db", defaultDBPath, "SQLite database path")
	debug := flag.Bool("debug", false, "enable debug request/response and outbound call logging")
	flag.Parse()

	store, err := sqlitestore.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("failed to initialize sqlite store: %v", err)
	}
	defer store.Close()

	fetcher := opentdb.FetchQuestions
	if *debug {
		fetcher = loggedFetcher(fetcher)
	}

	service := quiz.NewService(store, store, fetcher)

	server := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewRouterWithOptions(service, quiz.NewBank(), httpapi.RouterOptions{Debug: *debug}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("quiz-service listening on %s with db=%s debug=%t", *addr, *dbPath, *debug)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func loggedFetcher(fetcher quiz.QuestionsFetcher) quiz.QuestionsFetcher {
	return func(ctx context.Context, amount int) ([]opentdb.RawQuestion, error) {
		start := time.Now()
		log.Printf("outbound request provider=opentdb amount=%d", amount)

		questions, err := fetcher(ctx, amount)
		if err != nil {
			log.Printf("outbound error provider=opentdb amount=%d duration=%s err=%v", amount, time.Since(start).Round(time.Millisecond), err)
			return nil, err
		}

		log.Printf("outbound success provider=opentdb amount=%d received=%d duration=%s", amount, len(questions), time.Since(start).Round(time.Millisecond))
		return questions, nil
	}
}
