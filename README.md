# quiz-app

A Go backend quiz service that:

1. Calls OpenTriviaDB for question data.
2. Transforms questions into internal quiz models.
3. Persists reusable quizzes and leaderboard.
4. Supports answer submissions.

### Start service

```bash
go run ./cmd/quiz-service -addr :8080 -db quiz.db
```

### Run CLI

```bash
go run ./cmd/quiz-cli
```
