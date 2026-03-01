package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"quiz-app/internal/httpapi"
	"quiz-app/internal/opentdb"
	"quiz-app/internal/quiz"
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
	flag.Parse()

	store, err := quiz.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("failed to initialize sqlite store: %v", err)
	}
	defer store.Close()

	service := quiz.NewService(store, store, opentdb.FetchQuestions)

	server := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewRouter(service, quiz.NewBank()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("quiz-service listening on %s with db=%s", *addr, *dbPath)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}
