package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxEventNameLength             = 100
	maxEventDescriptionLength      = 500
	maxContestantNameLength        = 100
	maxContestantDescriptionLength = 500
	defaultAdminVotesPageSize      = 50
	maxAdminVotesPageSize          = 200
)

type EventRequest struct {
	Name               string `json:"name" binding:"required" firestore:"name"`
	Description        string `json:"description" firestore:"description"`
	Archived           bool   `json:"archived" firestore:"archived"`
	ShowResults        bool   `json:"showResults" firestore:"showResults"`
	ResultsInitialized bool   `json:"-" firestore:"resultsInitialized"`
	ContestantCount    int64  `json:"-" firestore:"contestantCount"`
}

type ContestantRequest struct {
	Name        string `json:"name" binding:"required" firestore:"name"`
	Description string `json:"description" firestore:"description"`
}

type VotingStateRequest struct {
	Stopped bool `json:"stopped"`
}

type AdminEventResponse struct {
	Event
	ResultsInitialized bool `json:"resultsInitialized"`
	ResultsRebuilding  bool `json:"resultsRebuilding"`
	BallotsCleaned     bool `json:"ballotsCleaned"`
}

type AdminVoteOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AdminVoterBallot struct {
	VoterID  string         `json:"voterId"`
	Nickname string         `json:"nickname"`
	Scores   map[string]int `json:"scores"`
}

type AdminEventVotesResponse struct {
	Options       []AdminVoteOption  `json:"options"`
	Voters        []AdminVoterBallot `json:"voters"`
	NextPageToken string             `json:"nextPageToken,omitempty"`
}

func parseAdminVotesPage(values map[string][]string) (int, string, error) {
	pageSize := defaultAdminVotesPageSize
	if raw := strings.TrimSpace(firstQueryValue(values, "pageSize")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxAdminVotesPageSize {
			return 0, "", errors.New("invalid page size")
		}
		pageSize = parsed
	}
	rawToken := strings.TrimSpace(firstQueryValue(values, "pageToken"))
	if rawToken == "" {
		return pageSize, "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil || !validDocumentID(string(decoded)) {
		return 0, "", errors.New("invalid page token")
	}
	return pageSize, string(decoded), nil
}

func firstQueryValue(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func adminVotesPageToken(voterID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(voterID))
}

func adminVoterBallot(voterID string, data map[string]interface{}, optionIDs map[string]struct{}) AdminVoterBallot {
	nickname, _ := data["nickname"].(string)
	return AdminVoterBallot{
		VoterID:  voterID,
		Nickname: strings.TrimSpace(nickname),
		Scores:   filterScores(storedScores(data["scores"]), optionIDs),
	}
}

func adminEventResponse(event Event) AdminEventResponse {
	return AdminEventResponse{
		Event:              event,
		ResultsInitialized: event.ResultsInitialized,
		ResultsRebuilding:  event.ResultsRebuilding,
		BallotsCleaned:     event.BallotsCleaned,
	}
}

func parseAdminEmails(value string) map[string]struct{} {
	emails := make(map[string]struct{})
	for _, email := range strings.Split(value, ",") {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			emails[email] = struct{}{}
		}
	}
	return emails
}

func requireAdmin(allowedEmails map[string]struct{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, exists := c.Get(authTokenKey)
		token, ok := value.(*auth.Token)
		if !exists || !ok || token == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Přihlášení je vyžadováno"})
			return
		}

		email, emailOK := token.Claims["email"].(string)
		emailVerified, verifiedOK := token.Claims["email_verified"].(bool)
		_, allowed := allowedEmails[strings.ToLower(email)]
		if token.Firebase.SignInProvider != "google.com" || !emailOK || !verifiedOK || !emailVerified || !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Nemáte oprávnění správce"})
			return
		}

		c.Next()
	}
}

func requireAdminAuth(verifier revokingTokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Firebase Auth není připojen"})
			return
		}
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Přihlášení je vyžadováno"})
			return
		}
		token, err := verifier.VerifyIDTokenAndCheckRevoked(c.Request.Context(), parts[1])
		if err != nil || token == nil || token.UID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Neplatné nebo odvolané přihlášení"})
			return
		}
		c.Set(voterUIDKey, token.UID)
		c.Set(authTokenKey, token)
		c.Next()
	}
}

func getAdminSession(c *gin.Context) {
	token := c.MustGet(authTokenKey).(*auth.Token)
	c.JSON(http.StatusOK, gin.H{"email": token.Claims["email"]})
}

func getAdminEvents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}

	iter := client.Collection("events").Documents(c.Request.Context())
	defer iter.Stop()
	events := make([]AdminEventResponse, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Události se nepodařilo načíst"})
			return
		}
		var event Event
		if err := doc.DataTo(&event); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data události"})
			return
		}
		event.ID = doc.Ref.ID
		event.Slug = eventSlug(event.Name)
		events = append(events, adminEventResponse(event))
	}
	c.JSON(http.StatusOK, events)
}

func getAdminEventVotes(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	pageSize, cursor, err := parseAdminVotesPage(c.Request.URL.Query())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné stránkování"})
		return
	}

	ctx := c.Request.Context()
	eventRef := client.Collection("events").Doc(eventID)
	eventDoc, err := eventRef.Get(ctx)
	if status.Code(err) == codes.NotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo načíst"})
		return
	}
	var event Event
	if err := eventDoc.DataTo(&event); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data události"})
		return
	}
	if event.BallotsCleaned {
		c.JSON(http.StatusGone, gin.H{"error": "Hlasovací lístky této události již byly odstraněny"})
		return
	}

	optionIDs := make(map[string]struct{})
	options := make([]AdminVoteOption, 0)
	optionsIter := eventRef.Collection("contestants").Documents(ctx)
	defer optionsIter.Stop()
	for {
		doc, err := optionsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Hlasovací položky se nepodařilo načíst"})
			return
		}
		var contestant Contestant
		if err := doc.DataTo(&contestant); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data hlasovací položky"})
			return
		}
		optionIDs[doc.Ref.ID] = struct{}{}
		options = append(options, AdminVoteOption{
			ID:   doc.Ref.ID,
			Name: strings.TrimSpace(contestant.Name),
		})
	}

	voters := make([]AdminVoterBallot, 0, pageSize)
	votesQuery := eventRef.Collection("votes").
		OrderBy(firestore.DocumentID, firestore.Asc).
		Limit(pageSize + 1)
	if cursor != "" {
		votesQuery = votesQuery.StartAfter(cursor)
	}
	votesIter := votesQuery.Documents(ctx)
	defer votesIter.Stop()
	hasMore := false
	for {
		doc, err := votesIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Hlasovací lístky se nepodařilo načíst"})
			return
		}
		if len(voters) == pageSize {
			hasMore = true
			break
		}
		voters = append(voters, adminVoterBallot(doc.Ref.ID, doc.Data(), optionIDs))
	}

	// Cleanup first marks the event and only then deletes ballots. Re-reading
	// the marker prevents a concurrent cleanup from producing a partial 200.
	eventDoc, err = eventRef.Get(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo ověřit"})
		return
	}
	var refreshed Event
	if err := eventDoc.DataTo(&refreshed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data události"})
		return
	}
	if refreshed.BallotsCleaned {
		c.JSON(http.StatusGone, gin.H{"error": "Hlasovací lístky této události již byly odstraněny"})
		return
	}

	response := AdminEventVotesResponse{Options: options, Voters: voters}
	if hasMore {
		response.NextPageToken = adminVotesPageToken(voters[len(voters)-1].VoterID)
	}
	c.JSON(http.StatusOK, response)
}

func createEvent(c *gin.Context) {
	req, ok := bindEventRequest(c)
	if !ok {
		return
	}
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}

	ref := client.Collection("events").NewDoc()
	req.ResultsInitialized = true
	if _, err := ref.Set(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo vytvořit"})
		return
	}
	c.JSON(http.StatusCreated, adminEventResponse(Event{
		ID:                 ref.ID,
		Name:               req.Name,
		Description:        req.Description,
		Archived:           req.Archived,
		ShowResults:        req.ShowResults,
		ResultsInitialized: true,
		Slug:               eventSlug(req.Name),
	}))
}

func updateEvent(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	req, ok := bindEventRequest(c)
	if !ok {
		return
	}

	ref := client.Collection("events").Doc(eventID)
	var updated Event
	err := client.RunTransaction(c.Request.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			return err
		}
		if err := doc.DataTo(&updated); err != nil {
			return err
		}
		if updated.BallotsCleaned && !req.Archived {
			return errBallotsCleaned
		}
		if updated.ResultsRebuilding && !req.Archived {
			return errResultsRebuild
		}
		updated.Name = req.Name
		updated.Description = req.Description
		updated.Archived = req.Archived
		updated.ShowResults = req.ShowResults
		return tx.Update(ref, []firestore.Update{
			{Path: "name", Value: req.Name},
			{Path: "description", Value: req.Description},
			{Path: "archived", Value: req.Archived},
			{Path: "showResults", Value: req.ShowResults},
		})
	})
	if status.Code(err) == codes.NotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	if errors.Is(err, errBallotsCleaned) {
		c.JSON(http.StatusConflict, gin.H{"error": "Událost po odstranění hlasovacích lístků nelze obnovit"})
		return
	}
	if errors.Is(err, errResultsRebuild) {
		c.JSON(http.StatusConflict, gin.H{"error": "Událost během přepočítávání výsledků nelze obnovit"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo upravit"})
		return
	}
	updated.ID = eventID
	updated.Slug = eventSlug(updated.Name)
	c.JSON(http.StatusOK, adminEventResponse(updated))
}

func archiveEvent(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}

	_, err := client.Collection("events").Doc(eventID).Update(c.Request.Context(), []firestore.Update{
		{Path: "archived", Value: true},
	})
	if status.Code(err) == codes.NotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo archivovat"})
		return
	}
	c.Status(http.StatusNoContent)
}

func createContestant(c *gin.Context) {
	req, ok := bindContestantRequest(c)
	if !ok {
		return
	}
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}

	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	eventRef := client.Collection("events").Doc(eventID)
	contestants := eventRef.Collection("contestants")
	ref := contestants.NewDoc()
	err := client.RunTransaction(c.Request.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		eventDoc, err := getMutableEvent(tx, eventRef)
		if err != nil {
			return err
		}
		count, err := transactionContestantCount(tx, contestants, eventDoc)
		if err != nil {
			return err
		}
		if count >= maxScoresPerVote {
			return errContestantLimit
		}
		if err := tx.Update(eventRef, []firestore.Update{
			{Path: "contestantCount", Value: count + 1},
		}); err != nil {
			return err
		}
		return tx.Create(ref, req)
	})
	if errors.Is(err, errContestantLimit) {
		c.JSON(http.StatusConflict, gin.H{"error": "Událost může mít nejvýše 100 hlasovacích položek"})
		return
	}
	if respondToImmutableEvent(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Soutěžícího se nepodařilo vytvořit"})
		return
	}

	c.JSON(http.StatusCreated, Contestant{
		ID:          ref.ID,
		Name:        req.Name,
		Description: req.Description,
	})
}

func updateContestant(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	contestantID := c.Param("id")
	if !validDocumentID(contestantID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID soutěžícího"})
		return
	}

	req, ok := bindContestantRequest(c)
	if !ok {
		return
	}

	eventRef := client.Collection("events").Doc(eventID)
	contestantRef := eventRef.Collection("contestants").Doc(contestantID)
	err := client.RunTransaction(c.Request.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		if err := requireMutableEvent(tx, eventRef); err != nil {
			return err
		}
		return tx.Update(contestantRef, []firestore.Update{
			{Path: "name", Value: req.Name},
			{Path: "description", Value: req.Description},
		})
	})
	if status.Code(err) == codes.NotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Soutěžící nebyl nalezen"})
		return
	}
	if respondToImmutableEvent(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Soutěžícího se nepodařilo upravit"})
		return
	}

	c.JSON(http.StatusOK, Contestant{
		ID:          contestantID,
		Name:        req.Name,
		Description: req.Description,
	})
}

func deleteContestant(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	contestantID := c.Param("id")
	if !validDocumentID(contestantID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID soutěžícího"})
		return
	}

	eventRef := client.Collection("events").Doc(eventID)
	contestants := eventRef.Collection("contestants")
	contestantRef := contestants.Doc(contestantID)
	err := client.RunTransaction(c.Request.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		eventDoc, err := getMutableEvent(tx, eventRef)
		if err != nil {
			return err
		}
		if _, err := tx.Get(contestantRef); err != nil {
			return err
		}
		count, err := transactionContestantCount(tx, contestants, eventDoc)
		if err != nil {
			return err
		}
		if err := tx.Update(eventRef, []firestore.Update{
			{Path: "contestantCount", Value: max(int64(0), count-1)},
		}); err != nil {
			return err
		}
		if err := tx.Delete(contestantRef); err != nil {
			return err
		}
		return tx.Delete(eventRef.Collection("results").Doc(contestantID))
	})
	if respondToImmutableEvent(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Soutěžícího se nepodařilo odstranit"})
		return
	}
	c.Status(http.StatusNoContent)
}

func setEventVotingState(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}

	var req VotingStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatný požadavek"})
		return
	}

	ref := client.Collection("events").Doc(eventID)
	err := client.RunTransaction(c.Request.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var event Event
		if err := doc.DataTo(&event); err != nil {
			return err
		}
		if event.Archived {
			return errEventInactive
		}
		if event.BallotsCleaned {
			return errBallotsCleaned
		}
		if event.ResultsRebuilding && !req.Stopped {
			return errResultsRebuild
		}
		return tx.Update(ref, []firestore.Update{
			{Path: "votingStopped", Value: req.Stopped},
		})
	})
	switch {
	case status.Code(err) == codes.NotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	case errors.Is(err, errEventInactive):
		c.JSON(http.StatusConflict, gin.H{"error": "Archivované události nelze změnit stav hlasování"})
		return
	case errors.Is(err, errBallotsCleaned):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasování po odstranění hlasovacích lístků nelze obnovit"})
		return
	case errors.Is(err, errResultsRebuild):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasování nelze obnovit během přepočítávání výsledků"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Stav hlasování se nepodařilo změnit"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"votingStopped": req.Stopped})
}

func rebuildEventResults(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}

	ctx := c.Request.Context()
	eventRef := client.Collection("events").Doc(eventID)
	eventDoc, err := eventRef.Get(ctx)
	if status.Code(err) == codes.NotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo načíst"})
		return
	}
	var event Event
	if err := eventDoc.DataTo(&event); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data události"})
		return
	}
	if !event.Archived && !event.VotingStopped {
		c.JSON(http.StatusConflict, gin.H{"error": "Před přepočítáním nejdříve zastavte hlasování"})
		return
	}
	if event.BallotsCleaned {
		c.JSON(http.StatusConflict, gin.H{"error": "Výsledky po odstranění hlasovacích lístků nelze přepočítat"})
		return
	}
	err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(eventRef)
		if err != nil {
			return err
		}
		var current Event
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		if !current.Archived && !current.VotingStopped {
			return errVotingActive
		}
		if current.BallotsCleaned {
			return errBallotsCleaned
		}
		return tx.Update(eventRef, []firestore.Update{
			{Path: "resultsRebuilding", Value: true},
		})
	})
	switch {
	case errors.Is(err, errVotingActive):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasování bylo obnoveno; nejdříve je znovu zastavte"})
		return
	case errors.Is(err, errBallotsCleaned):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasovací lístky již byly odstraněny"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Přepočítání se nepodařilo zahájit"})
		return
	}

	// Detach from the request context so a client disconnect does not abort the
	// rebuild mid-way. If any later step fails, resultsRebuilding deliberately
	// remains true: cleanup must not trust potentially partial aggregates. A
	// successful retry replaces every aggregate and clears the marker.
	ctx = context.WithoutCancel(ctx)

	contestants := make(map[string]*Contestant)
	contestantsIter := eventRef.Collection("contestants").Documents(ctx)
	defer contestantsIter.Stop()
	for {
		doc, err := contestantsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Hlasovací položky se nepodařilo načíst"})
			return
		}
		contestants[doc.Ref.ID] = &Contestant{ID: doc.Ref.ID}
	}

	stats, err := loadLegacyStats(ctx, eventRef, contestants)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Hlasy se nepodařilo přepočítat"})
		return
	}

	type resultWrite struct {
		ref       *firestore.DocumentRef
		aggregate ResultAggregate
		delete    bool
	}
	writes := make([]resultWrite, 0, len(contestants))
	for contestantID := range contestants {
		writes = append(writes, resultWrite{
			ref:       eventRef.Collection("results").Doc(contestantID),
			aggregate: stats[contestantID],
		})
	}

	resultsIter := eventRef.Collection("results").Documents(ctx)
	defer resultsIter.Stop()
	for {
		doc, err := resultsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Staré výsledky se nepodařilo načíst"})
			return
		}
		if _, exists := contestants[doc.Ref.ID]; !exists {
			writes = append(writes, resultWrite{ref: doc.Ref, delete: true})
		}
	}

	const rebuildBatchSize = 400
	for start := 0; start < len(writes); start += rebuildBatchSize {
		end := min(start+rebuildBatchSize, len(writes))
		batch := client.Batch()
		for _, write := range writes[start:end] {
			if write.delete {
				batch.Delete(write.ref)
			} else {
				batch.Set(write.ref, write.aggregate)
			}
		}
		if _, err := batch.Commit(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Výsledky se nepodařilo uložit"})
			return
		}
	}

	err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(eventRef)
		if err != nil {
			return err
		}
		var current Event
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		if !current.Archived && !current.VotingStopped {
			return errVotingActive
		}
		if current.BallotsCleaned {
			return errBallotsCleaned
		}
		return tx.Update(eventRef, []firestore.Update{
			{Path: "resultsInitialized", Value: true},
			{Path: "resultsRebuilding", Value: false},
		})
	})
	switch {
	case status.Code(err) == codes.NotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	case errors.Is(err, errVotingActive):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasování bylo obnoveno; přepočítání zopakujte po jeho zastavení"})
		return
	case errors.Is(err, errBallotsCleaned):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasovací lístky již byly odstraněny; zachované souhrny nelze přepsat"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Přepočítání se nepodařilo dokončit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"contestants": len(contestants),
	})
}

func bindEventRequest(c *gin.Context) (EventRequest, bool) {
	var req EventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatný požadavek"})
		return EventRequest{}, false
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if !validEventShape(req) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné údaje události"})
		return EventRequest{}, false
	}
	return req, true
}

func getMutableEvent(tx *firestore.Transaction, eventRef *firestore.DocumentRef) (*firestore.DocumentSnapshot, error) {
	doc, err := tx.Get(eventRef)
	if err != nil {
		return nil, err
	}
	var event Event
	if err := doc.DataTo(&event); err != nil {
		return nil, err
	}
	if event.Archived || event.BallotsCleaned {
		return nil, errEventInactive
	}
	if event.ResultsRebuilding {
		return nil, errResultsRebuild
	}
	return doc, nil
}

func requireMutableEvent(tx *firestore.Transaction, eventRef *firestore.DocumentRef) error {
	_, err := getMutableEvent(tx, eventRef)
	return err
}

func transactionContestantCount(
	tx *firestore.Transaction,
	contestants *firestore.CollectionRef,
	eventDoc *firestore.DocumentSnapshot,
) (int64, error) {
	if count, exists, err := storedContestantCount(eventDoc.Data()); err != nil {
		return 0, err
	} else if exists {
		return count, nil
	}

	// Existing events predate contestantCount. The first mutation backfills it
	// while also updating the event document, so concurrent mutations conflict
	// on that document and Firestore retries them against the new counter.
	iter := tx.Documents(contestants.Limit(maxScoresPerVote + 1))
	defer iter.Stop()
	var count int64
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			return count, nil
		}
		if err != nil {
			return 0, err
		}
		count++
	}
}

func storedContestantCount(data map[string]interface{}) (int64, bool, error) {
	value, exists := data["contestantCount"]
	if !exists {
		return 0, false, nil
	}
	count, ok := value.(int64)
	if !ok || count < 0 {
		return 0, true, errors.New("invalid contestant count")
	}
	return count, true, nil
}

func respondToImmutableEvent(c *gin.Context, err error) bool {
	switch {
	case status.Code(err) == codes.NotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebo hlasovací položka nebyla nalezena"})
		return true
	case errors.Is(err, errEventInactive):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasovací položky archivované události nelze měnit"})
		return true
	case errors.Is(err, errResultsRebuild):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasovací položky nelze měnit během přepočítávání výsledků"})
		return true
	default:
		return false
	}
}

func bindContestantRequest(c *gin.Context) (ContestantRequest, bool) {
	var req ContestantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatný požadavek"})
		return ContestantRequest{}, false
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if !validContestantShape(req) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné údaje soutěžícího"})
		return ContestantRequest{}, false
	}
	return req, true
}

func validEventShape(req EventRequest) bool {
	return strings.TrimSpace(req.Name) != "" &&
		utf8.RuneCountInString(req.Name) <= maxEventNameLength &&
		utf8.RuneCountInString(req.Description) <= maxEventDescriptionLength
}

func validContestantShape(req ContestantRequest) bool {
	return strings.TrimSpace(req.Name) != "" &&
		utf8.RuneCountInString(req.Name) <= maxContestantNameLength &&
		utf8.RuneCountInString(req.Description) <= maxContestantDescriptionLength
}
