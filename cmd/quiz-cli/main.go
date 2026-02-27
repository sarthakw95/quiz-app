package main

import (
	"context"
	"fmt"
	"os"

	"quiz-app/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
