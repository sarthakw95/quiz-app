package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"quiz-app/internal/userclient"
)

func main() {
	username := flag.String("username", "", "username for quiz attempts (required)")
	server := flag.String("server", "http://127.0.0.1:8080", "quiz service base URL")
	timeout := flag.Duration("timeout", 5*time.Second, "HTTP timeout")
	flag.Parse()

	if *username == "" {
		fmt.Fprintln(os.Stderr, "error: --username is required")
		os.Exit(1)
	}

	err := userclient.Run(context.Background(), os.Stdin, os.Stdout, userclient.Config{
		Username:    *username,
		ServerURL:   *server,
		HTTPTimeout: *timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
