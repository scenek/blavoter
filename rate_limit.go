package main

import (
	"time"
)

const (
	voteTokenRefillRate = 2.0
	voteTokenBurst      = 10.0
)

// consumeVoteToken uses state stored on the ballot itself. Because saveVote
// reads and updates that ballot in a Firestore transaction, this limit is
// authoritative across all application instances.
func consumeVoteToken(ballot map[string]interface{}, now time.Time) (float64, bool) {
	tokens := voteTokenBurst
	updatedAt, hasUpdatedAt := ballot["voteRateUpdatedAt"].(time.Time)
	if hasUpdatedAt {
		if storedTokens, ok := firestoreNumber(ballot["voteRateTokens"]); ok {
			tokens = storedTokens
		}
		if elapsed := now.Sub(updatedAt).Seconds(); elapsed > 0 {
			tokens = min(voteTokenBurst, tokens+elapsed*voteTokenRefillRate)
		}
	}
	if tokens < 1 {
		return tokens, false
	}
	return tokens - 1, true
}

func firestoreNumber(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}
