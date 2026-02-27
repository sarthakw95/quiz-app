package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"quiz-app/internal/httpapi"
	"quiz-app/internal/quiz"
)

func main() {
	defaultAddr := os.Getenv("ADDR")
	if defaultAddr == "" {
		defaultAddr = ":8080"
	}

	addr := flag.String("addr", defaultAddr, "HTTP listen address")
	flag.Parse()

	server := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewRouter(quiz.NewBank()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("quiz-service listening on %s", *addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}
