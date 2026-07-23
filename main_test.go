package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
)

type fakeTokenVerifier struct {
	token      *auth.Token
	err        error
	revokedErr error
}

func (f fakeTokenVerifier) VerifyIDToken(context.Context, string) (*auth.Token, error) {
	return f.token, f.err
}

func (f fakeTokenVerifier) VerifyIDTokenAndCheckRevoked(context.Context, string) (*auth.Token, error) {
	if f.revokedErr != nil {
		return nil, f.revokedErr
	}
	return f.token, f.err
}

func TestRequireAuthRejectsMissingBearerToken(t *testing.T) {
	response := runAuthRequest(fakeTokenVerifier{token: &auth.Token{UID: "voter-1"}}, "")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthRejectsInvalidToken(t *testing.T) {
	response := runAuthRequest(fakeTokenVerifier{err: errors.New("invalid token")}, "Bearer invalid")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthUsesVerifiedUID(t *testing.T) {
	response := runAuthRequest(fakeTokenVerifier{token: &auth.Token{UID: "voter-1"}}, "Bearer valid")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "voter-1" {
		t.Fatalf("body = %q, want verified UID", response.Body.String())
	}
}

func TestRequireAuthReportsUnavailableVerifier(t *testing.T) {
	response := runAuthRequest(nil, "Bearer valid")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestValidVoteShape(t *testing.T) {
	tooManyScores := make(map[string]int, maxScoresPerVote+1)
	for i := 0; i <= maxScoresPerVote; i++ {
		tooManyScores[string(rune(i+1))] = 5
	}

	tests := []struct {
		name string
		req  VoteRequest
		want bool
	}{
		{
			name: "valid",
			req:  VoteRequest{Scores: map[string]int{"sample-1": 10}},
			want: true,
		},
		{
			name: "zero score",
			req:  VoteRequest{Scores: map[string]int{"sample-1": 0}},
			want: true,
		},
		{
			name: "empty scores",
			req:  VoteRequest{Scores: map[string]int{}},
			want: true,
		},
		{
			name: "missing scores",
			req:  VoteRequest{},
		},
		{
			name: "too many scores",
			req:  VoteRequest{Scores: tooManyScores},
		},
		{
			name: "score below range",
			req:  VoteRequest{Scores: map[string]int{"sample-1": -1}},
		},
		{
			name: "score above range",
			req:  VoteRequest{Scores: map[string]int{"sample-1": 11}},
		},
		{
			name: "nested contestant path",
			req:  VoteRequest{Scores: map[string]int{"sample/1": 5}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validVoteShape(test.req); got != test.want {
				t.Fatalf("validVoteShape() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestVoteRequestTreatsNullAsNoVote(t *testing.T) {
	var request VoteRequest
	if err := json.Unmarshal([]byte(`{"scores":{"rated":0,"unrated":null}}`), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Scores) != 1 || request.Scores["rated"] != 0 {
		t.Fatalf("Scores = %#v, want zero rating retained and null omitted", request.Scores)
	}
	if _, exists := request.Scores["unrated"]; exists {
		t.Fatal("null score was incorrectly retained as a vote")
	}
}

func TestValidNickname(t *testing.T) {
	if !validNickname("Štěpán") {
		t.Fatal("valid nickname was rejected")
	}
	if validNickname("   ") {
		t.Fatal("blank nickname was accepted")
	}
	if validNickname(strings.Repeat("x", maxNicknameLength+1)) {
		t.Fatal("overlong nickname was accepted")
	}
}

func TestValidStoredScore(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		wantScore int
		wantOK    bool
	}{
		{name: "valid", value: int64(7), wantScore: 7, wantOK: true},
		{name: "zero is valid", value: int64(0), wantScore: 0, wantOK: true},
		{name: "below range", value: int64(-1)},
		{name: "above range", value: int64(11)},
		{name: "wrong type", value: "7"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			score, ok := validStoredScore(test.value)
			if score != test.wantScore || ok != test.wantOK {
				t.Fatalf("validStoredScore() = (%d, %v), want (%d, %v)", score, ok, test.wantScore, test.wantOK)
			}
		})
	}
}

func TestResultDeltas(t *testing.T) {
	got := resultDeltas(
		map[string]int{
			"changed": 3,
			"removed": 8,
			"same":    5,
		},
		map[string]int{
			"changed": 7,
			"added":   0,
			"same":    5,
		},
	)

	want := map[string]ResultDelta{
		"changed": {TotalScore: 4},
		"removed": {TotalScore: -8, VoteCount: -1},
		"added":   {TotalScore: 0, VoteCount: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("resultDeltas() = %#v, want %#v", got, want)
	}
	for id, expected := range want {
		if got[id] != expected {
			t.Errorf("resultDeltas()[%q] = %#v, want %#v", id, got[id], expected)
		}
	}
	if _, exists := got["same"]; exists {
		t.Fatal("unchanged score produced an aggregate update")
	}
}

func TestStoredScoresFiltersInvalidValues(t *testing.T) {
	got := storedScores(map[string]interface{}{
		"valid":   int64(10),
		"zero":    int64(0),
		"invalid": int64(11),
		"string":  "5",
	})

	if len(got) != 2 || got["valid"] != 10 || got["zero"] != 0 {
		t.Fatalf("storedScores() = %#v, want valid scores including zero", got)
	}
}

func TestFilterScoresRemovesDeletedContestants(t *testing.T) {
	got := filterScores(
		map[string]int{"existing": 8, "deleted": 6},
		map[string]struct{}{"existing": {}},
	)

	if len(got) != 1 || got["existing"] != 8 {
		t.Fatalf("filterScores() = %#v, want only the existing contestant", got)
	}
}

func TestValidDocumentID(t *testing.T) {
	tests := map[string]bool{
		"event-1": true,
		"":        false,
		".":       false,
		"..":      false,
		"a/b":     false,
	}
	for id, want := range tests {
		if got := validDocumentID(id); got != want {
			t.Errorf("validDocumentID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestEventSlug(t *testing.T) {
	tests := map[string]string{
		"Letní degustace 2026": "letní-degustace-2026",
		"  Beer & Wine  ":      "beer-wine",
		"***":                  "event",
	}
	for name, want := range tests {
		if got := eventSlug(name); got != want {
			t.Errorf("eventSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestExistingEventDefaultsToOpenVoting(t *testing.T) {
	var event Event
	if err := json.Unmarshal([]byte(`{"name":"Existing event"}`), &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.VotingStopped {
		t.Fatal("event without votingStopped should remain open for voting")
	}
}

func TestMyVotesResponseIncludesNickname(t *testing.T) {
	response := myVotesResponse(map[string]interface{}{
		"nickname": "Štěpán",
		"scores":   map[string]interface{}{"sample-1": int64(8)},
	})

	if response.Nickname != "Štěpán" {
		t.Fatalf("Nickname = %q, want saved nickname", response.Nickname)
	}
	if response.Scores["sample-1"] != int64(8) {
		t.Fatalf("Scores = %#v, want saved score", response.Scores)
	}
}

func TestMyVotesResponseDefaultsToEmptyValues(t *testing.T) {
	response := myVotesResponse(nil)

	if response.Nickname != "" {
		t.Fatalf("Nickname = %q, want empty nickname", response.Nickname)
	}
	if response.Scores == nil || len(response.Scores) != 0 {
		t.Fatalf("Scores = %#v, want non-nil empty scores", response.Scores)
	}
}

func runAuthRequest(verifier tokenVerifier, authorization string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requireAuth(verifier))
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(voterUIDKey))
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
