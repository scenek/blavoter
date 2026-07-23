package main

import (
	"testing"
	"time"
)

func TestConsumeVoteToken(t *testing.T) {
	now := time.Unix(1000, 0)
	ballot := map[string]interface{}{}

	for range int(voteTokenBurst) {
		tokens, allowed := consumeVoteToken(ballot, now)
		if !allowed {
			t.Fatal("initial burst was unexpectedly rejected")
		}
		ballot["voteRateTokens"] = tokens
		ballot["voteRateUpdatedAt"] = now
	}
	if _, allowed := consumeVoteToken(ballot, now); allowed {
		t.Fatal("request beyond the burst was accepted")
	}
	tokens, allowed := consumeVoteToken(ballot, now.Add(500*time.Millisecond))
	if !allowed || tokens != 0 {
		t.Fatal("refilled token was rejected")
	}
}

func TestConsumeVoteTokenAcceptsFirestoreInteger(t *testing.T) {
	now := time.Unix(1000, 0)
	tokens, allowed := consumeVoteToken(map[string]interface{}{
		"voteRateTokens":    int64(1),
		"voteRateUpdatedAt": now,
	}, now)
	if !allowed || tokens != 0 {
		t.Fatalf("consumeVoteToken() = (%v, %v), want (0, true)", tokens, allowed)
	}
}
