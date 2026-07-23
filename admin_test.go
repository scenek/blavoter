package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
)

func TestRequireAdmin(t *testing.T) {
	tests := []struct {
		name    string
		token   *auth.Token
		allowed string
		want    int
	}{
		{
			name:    "allowed Google account",
			token:   adminToken("admin@example.com", true, "google.com"),
			allowed: "admin@example.com",
			want:    http.StatusOK,
		},
		{
			name:    "email comparison is case insensitive",
			token:   adminToken("Admin@Example.com", true, "google.com"),
			allowed: "admin@example.com",
			want:    http.StatusOK,
		},
		{
			name:    "email not allowlisted",
			token:   adminToken("other@example.com", true, "google.com"),
			allowed: "admin@example.com",
			want:    http.StatusForbidden,
		},
		{
			name:    "unverified email",
			token:   adminToken("admin@example.com", false, "google.com"),
			allowed: "admin@example.com",
			want:    http.StatusForbidden,
		},
		{
			name:    "anonymous provider",
			token:   adminToken("admin@example.com", true, "anonymous"),
			allowed: "admin@example.com",
			want:    http.StatusForbidden,
		},
		{
			name:    "empty allowlist denies all",
			token:   adminToken("admin@example.com", true, "google.com"),
			allowed: "",
			want:    http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(
				requireAdminAuth(fakeTokenVerifier{token: test.token}),
				requireAdmin(parseAdminEmails(test.allowed)),
			)
			router.GET("/", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer valid")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestRequireAdminAuthRejectsRevokedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requireAdminAuth(fakeTokenVerifier{
		token:      adminToken("admin@example.com", true, "google.com"),
		revokedErr: errors.New("revoked"),
	}))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer revoked")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestParseAdminEmails(t *testing.T) {
	emails := parseAdminEmails(" First@Example.com,second@example.com, ,SECOND@example.com ")

	if len(emails) != 2 {
		t.Fatalf("len(emails) = %d, want 2", len(emails))
	}
	if _, ok := emails["first@example.com"]; !ok {
		t.Fatal("first normalized email is missing")
	}
	if _, ok := emails["second@example.com"]; !ok {
		t.Fatal("second normalized email is missing")
	}
}

func TestEventAndContestantValidation(t *testing.T) {
	if !validEventShape(EventRequest{Name: "Degustace 2026", Description: "Letní soutěž"}) {
		t.Fatal("valid event was rejected")
	}
	if validEventShape(EventRequest{Name: "   "}) {
		t.Fatal("blank event name was accepted")
	}
	if validEventShape(EventRequest{Name: strings.Repeat("x", maxEventNameLength+1)}) {
		t.Fatal("overlong event name was accepted")
	}
	if !validContestantShape(ContestantRequest{Name: "Vzorek 1"}) {
		t.Fatal("valid contestant was rejected")
	}
	if validContestantShape(ContestantRequest{
		Name:        "Vzorek 1",
		Description: strings.Repeat("x", maxContestantDescriptionLength+1),
	}) {
		t.Fatal("overlong contestant description was accepted")
	}
}

func TestAdminEventResponseExposesPrivateLifecycleState(t *testing.T) {
	data, err := json.Marshal(adminEventResponse(Event{
		ID:                 "event-1",
		Name:               "Event",
		ResultsInitialized: true,
		ResultsRebuilding:  true,
		BallotsCleaned:     true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"resultsInitialized":true`) {
		t.Fatalf("admin response does not expose resultsInitialized: %s", body)
	}
	if !strings.Contains(body, `"ballotsCleaned":true`) {
		t.Fatalf("admin response does not expose ballotsCleaned: %s", body)
	}
	if !strings.Contains(body, `"resultsRebuilding":true`) {
		t.Fatalf("admin response does not expose resultsRebuilding: %s", body)
	}

	publicData, err := json.Marshal(Event{ResultsInitialized: true, ResultsRebuilding: true, BallotsCleaned: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicData), "resultsInitialized") ||
		strings.Contains(string(publicData), "resultsRebuilding") ||
		strings.Contains(string(publicData), "ballotsCleaned") {
		t.Fatalf("public event leaked private lifecycle state: %s", publicData)
	}
}

func TestAdminVoterBallotFiltersDeletedOptions(t *testing.T) {
	ballot := adminVoterBallot(
		"voter-1",
		map[string]interface{}{
			"nickname": "  Alice  ",
			"scores": map[string]interface{}{
				"current": int64(8),
				"deleted": int64(4),
			},
		},
		map[string]struct{}{"current": {}},
	)

	if ballot.VoterID != "voter-1" || ballot.Nickname != "Alice" {
		t.Fatalf("adminVoterBallot() identity = %#v", ballot)
	}
	if len(ballot.Scores) != 1 || ballot.Scores["current"] != 8 {
		t.Fatalf("adminVoterBallot() scores = %#v", ballot.Scores)
	}
}

func TestAdminVotesPagination(t *testing.T) {
	token := adminVotesPageToken("firebase-uid")
	pageSize, cursor, err := parseAdminVotesPage(map[string][]string{
		"pageSize":  {"75"},
		"pageToken": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pageSize != 75 || cursor != "firebase-uid" {
		t.Fatalf("parseAdminVotesPage() = (%d, %q), want (75, firebase-uid)", pageSize, cursor)
	}

	for _, values := range []map[string][]string{
		{"pageSize": {"0"}},
		{"pageSize": {"201"}},
		{"pageSize": {"not-a-number"}},
		{"pageToken": {"not valid base64!"}},
		{"pageToken": {adminVotesPageToken("../invalid")}},
	} {
		if _, _, err := parseAdminVotesPage(values); err == nil {
			t.Fatalf("parseAdminVotesPage(%v) unexpectedly succeeded", values)
		}
	}
}

func TestStoredContestantCountSupportsLegacyEvents(t *testing.T) {
	count, exists, err := storedContestantCount(map[string]interface{}{})
	if err != nil || exists || count != 0 {
		t.Fatalf("legacy count = (%d, %v, %v), want (0, false, nil)", count, exists, err)
	}

	count, exists, err = storedContestantCount(map[string]interface{}{"contestantCount": int64(17)})
	if err != nil || !exists || count != 17 {
		t.Fatalf("stored count = (%d, %v, %v), want (17, true, nil)", count, exists, err)
	}

	if _, _, err := storedContestantCount(map[string]interface{}{"contestantCount": int64(-1)}); err == nil {
		t.Fatal("negative contestant count was accepted")
	}
}

func adminToken(email string, verified bool, provider string) *auth.Token {
	return &auth.Token{
		UID: "admin-uid",
		Firebase: auth.FirebaseInfo{
			SignInProvider: provider,
		},
		Claims: map[string]interface{}{
			"email":          email,
			"email_verified": verified,
		},
	}
}
