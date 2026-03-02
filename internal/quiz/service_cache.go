package quiz

import "time"

// Cache-specific helpers are isolated here so service.go can focus on orchestration.

func (s *Service) getCachedQuizMetadata(quizID string) (QuizMetadata, bool) {
	metadata, ok := s.quizMetaCache[quizID]
	return metadata, ok
}

func (s *Service) setCachedQuizMetadata(metadata QuizMetadata) {
	s.quizMetaCache[metadata.QuizID] = metadata
}

func (s *Service) getCachedQuiz(quizID string) (QuizMetadata, []Question, bool) {
	metadata, metaOK := s.quizMetaCache[quizID]
	questions, questionsOK := s.quizQuestions[quizID]
	if !metaOK || !questionsOK {
		return QuizMetadata{}, nil, false
	}
	return metadata, questions, true
}

func (s *Service) setCachedQuiz(metadata QuizMetadata, questions []Question) {
	s.quizMetaCache[metadata.QuizID] = metadata
	s.quizQuestions[metadata.QuizID] = questions
}

func (s *Service) getCachedLeaderboard(quizID string) ([]LeaderboardEntry, bool) {
	cache, ok := s.leaderboardCache[quizID]
	if !ok || cache == nil {
		return nil, false
	}
	// Return direct cached memory for simplicity; caller treats result as read-only.
	return cache.ordered, true
}

func (s *Service) getCachedAttemptScores(quizID, usernameNormalized string) (map[string]float64, bool) {
	scores, ok := s.attemptScores[attemptScoresCacheKey(quizID, usernameNormalized)]
	// Map is shared cache state; callers should only read from the returned map.
	return scores, ok
}

func (s *Service) setCachedAttemptScores(quizID, usernameNormalized string, scores map[string]float64) {
	if scores == nil {
		scores = make(map[string]float64)
	}
	s.attemptScores[attemptScoresCacheKey(quizID, usernameNormalized)] = scores
}

func (s *Service) setCachedLeaderboard(quizID string, entries []LeaderboardEntry) {
	indexByUser := make(map[string]int, len(entries))
	for idx := range entries {
		indexByUser[entries[idx].Username] = idx
	}

	s.leaderboardCache[quizID] = &leaderboardCache{
		ordered:     entries,
		indexByUser: indexByUser,
	}
}

func (s *Service) updateCachedAttemptScoresAfterSubmission(quizID, usernameNormalized string, results []ResponseResult) {
	// Keep writes cheap: only patch attempt-score cache if this user+quiz cache was
	// already materialized by a previous read. Otherwise, it is rebuilt from DB on demand.
	scores, ok := s.getCachedAttemptScores(quizID, usernameNormalized)
	if !ok {
		return
	}

	for _, result := range results {
		switch result.Status {
		case StatusCorrect:
			scores[result.QuestionID] = 1.0
		case StatusIncorrect:
			scores[result.QuestionID] = 0.0
		case StatusAlreadyAnswered:
			if result.AttemptScore != nil {
				scores[result.QuestionID] = *result.AttemptScore
			}
		}
	}
}

func (s *Service) updateCachedLeaderboardAfterSubmission(quizID, username string, results []ResponseResult) {
	cache, ok := s.leaderboardCache[quizID]
	if !ok || cache == nil {
		return
	}

	// Maintain ordering incrementally so we do not rerun DB SUM/GROUP BY on every submit.
	// Current scoring model is binary (correct=1, incorrect=0), but this can be swapped
	// to use result.AttemptScore when richer per-question scoring is introduced.
	newAnswers := 0
	scoreDelta := 0.0
	for _, result := range results {
		switch result.Status {
		case StatusCorrect:
			newAnswers++
			scoreDelta += 1.0
		case StatusIncorrect:
			newAnswers++
		}
	}
	if newAnswers == 0 {
		return
	}

	now := time.Now().UTC()
	idx, exists := cache.indexByUser[username]
	if !exists {
		cache.ordered = append(cache.ordered, LeaderboardEntry{
			Username:         username,
			TotalScore:       scoreDelta,
			AnsweredCount:    newAnswers,
			LastSubmissionAt: now,
		})
		idx = len(cache.ordered) - 1
		cache.indexByUser[username] = idx
		s.bubbleLeaderboard(cache, idx)
		return
	}

	cache.ordered[idx].TotalScore += scoreDelta
	cache.ordered[idx].AnsweredCount += newAnswers
	cache.ordered[idx].LastSubmissionAt = now
	s.bubbleLeaderboard(cache, idx)
}

func attemptScoresCacheKey(quizID, usernameNormalized string) string {
	return quizID + "::" + usernameNormalized
}

func (s *Service) bubbleLeaderboard(cache *leaderboardCache, idx int) {
	// Only one user row changes per submission, so local bubbling is enough to
	// restore ordering in O(distance moved) instead of re-sorting the full slice.
	for idx > 0 && leaderboardBefore(cache.ordered[idx], cache.ordered[idx-1]) {
		s.swapLeaderboardEntries(cache, idx, idx-1)
		idx--
	}

	for idx+1 < len(cache.ordered) && leaderboardBefore(cache.ordered[idx+1], cache.ordered[idx]) {
		s.swapLeaderboardEntries(cache, idx, idx+1)
		idx++
	}
}

func (s *Service) swapLeaderboardEntries(cache *leaderboardCache, i, j int) {
	cache.ordered[i], cache.ordered[j] = cache.ordered[j], cache.ordered[i]
	cache.indexByUser[cache.ordered[i].Username] = i
	cache.indexByUser[cache.ordered[j].Username] = j
}

func leaderboardBefore(a, b LeaderboardEntry) bool {
	// Ranking policy:
	// 1) higher score first
	// 2) earlier final submission wins ties
	// 3) username lexical order for deterministic output
	if a.TotalScore != b.TotalScore {
		return a.TotalScore > b.TotalScore
	}
	if !a.LastSubmissionAt.Equal(b.LastSubmissionAt) {
		return a.LastSubmissionAt.Before(b.LastSubmissionAt)
	}
	return a.Username < b.Username
}

func applyLeaderboardLimit(entries []LeaderboardEntry, limit int) []LeaderboardEntry {
	if limit <= 0 || limit >= len(entries) {
		return entries
	}
	return entries[:limit]
}
