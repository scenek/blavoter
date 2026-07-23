package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Contestant struct {
	ID          string  `json:"id" firestore:"-"`
	Name        string  `json:"name" firestore:"name"`
	Description string  `json:"description" firestore:"description"`
	AvgScore    float64 `json:"avgScore" firestore:"-"`
	VoteCount   int     `json:"voteCount" firestore:"-"`
}

type Event struct {
	ID                 string `json:"id" firestore:"-"`
	Name               string `json:"name" firestore:"name"`
	Description        string `json:"description" firestore:"description"`
	Archived           bool   `json:"archived" firestore:"archived"`
	ShowResults        bool   `json:"showResults" firestore:"showResults"`
	VotingStopped      bool   `json:"votingStopped" firestore:"votingStopped"`
	ResultsInitialized bool   `json:"-" firestore:"resultsInitialized"`
	ResultsRebuilding  bool   `json:"-" firestore:"resultsRebuilding"`
	BallotsCleaned     bool   `json:"-" firestore:"ballotsCleaned"`
	ContestantCount    int64  `json:"-" firestore:"contestantCount"`
	Slug               string `json:"slug" firestore:"-"`
}

type ResultAggregate struct {
	TotalScore int64 `firestore:"totalScore"`
	VoteCount  int64 `firestore:"voteCount"`
}

type ResultDelta struct {
	TotalScore int64
	VoteCount  int64
}

type VoteRequest struct {
	Scores map[string]int `json:"scores"`
}

func (request *VoteRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		Scores map[string]*int `json:"scores"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Scores == nil {
		request.Scores = nil
		return nil
	}
	request.Scores = make(map[string]int, len(raw.Scores))
	for contestantID, score := range raw.Scores {
		if score != nil {
			request.Scores[contestantID] = *score
		}
	}
	return nil
}

type ProfileRequest struct {
	Nickname string `json:"nickname" binding:"required,max=80"`
}

type MyVotesResponse struct {
	Nickname string                 `json:"nickname"`
	Scores   map[string]interface{} `json:"scores"`
}

var client *firestore.Client
var authClient firebaseTokenVerifier

var (
	errVotingStopped   = errors.New("voting is stopped")
	errVotingActive    = errors.New("voting is active")
	errEventInactive   = errors.New("event is inactive")
	errMissingProfile  = errors.New("voter profile is missing")
	errAccountCleaned  = errors.New("anonymous account has been cleaned")
	errBallotsCleaned  = errors.New("ballots have been cleaned")
	errInvalidVote     = errors.New("vote contains an unknown contestant")
	errResultsRebuild  = errors.New("results rebuild is in progress")
	errContestantLimit = errors.New("contestant limit reached")
	errVoteRateLimited = errors.New("vote rate limit exceeded")
)

const (
	voterUIDKey                     = "voterUID"
	authTokenKey                    = "authToken"
	maxNicknameLength               = 80
	maxScoresPerVote                = 100
	cleanedAnonymousUsersCollection = "cleanedAnonymousUsers"
	voterBallotIndexesCollection    = "anonymousVoterBallots"
)

type tokenVerifier interface {
	VerifyIDToken(context.Context, string) (*auth.Token, error)
}

type revokingTokenVerifier interface {
	VerifyIDTokenAndCheckRevoked(context.Context, string) (*auth.Token, error)
}

type firebaseTokenVerifier interface {
	tokenVerifier
	revokingTokenVerifier
}

type FirebaseConfig struct {
	APIKey     string `json:"apiKey"`
	AuthDomain string `json:"authDomain"`
	ProjectID  string `json:"projectId"`
	AppID      string `json:"appId,omitempty"`
}

func main() {
	ctx := context.Background()
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Println("VAROVÁNÍ: GOOGLE_CLOUD_PROJECT není nastavena. Pro lokální test bez GCP nastavte tuto proměnnou.")
	}

	if projectID != "" {
		var err error
		client, err = firestore.NewClient(ctx, projectID)
		if err != nil {
			log.Fatalf("Chyba při připojování k Firestore: %v", err)
		}

		app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
		if err != nil {
			log.Fatalf("Chyba při inicializaci Firebase: %v", err)
		}
		authClient, err = app.Auth(ctx)
		if err != nil {
			log.Fatalf("Chyba při inicializaci Firebase Auth: %v", err)
		}
	}
	if client != nil {
		defer client.Close()
	}

	r := gin.Default()

	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.File("./static/landing.html")
	})
	r.GET("/event/:eventId/:slug", serveEventPage)
	r.GET("/event/:eventId/:slug/profile", serveEventProfilePage)
	r.GET("/event/:eventId/:slug/results", serveEventResultsPage)
	r.GET("/admin", func(c *gin.Context) {
		c.File("./static/admin.html")
	})
	r.GET("/admin/event/:eventId/results", func(c *gin.Context) {
		c.File("./static/results.html")
	})
	r.GET("/admin/event/:eventId/votes", func(c *gin.Context) {
		c.File("./static/votes.html")
	})

	r.GET("/api/events/:eventId", getEvent)
	r.GET("/api/events/:eventId/contestants", getContestants)
	r.GET("/api/events/:eventId/results", getPublicResults)
	r.GET("/api/firebase-config", getFirebaseConfig)

	authorized := r.Group("/api/events/:eventId")
	authorized.Use(requireAuth(authClient))
	authorized.POST("/vote", saveVote)
	authorized.GET("/my-votes", getMyVotes)
	authorized.PUT("/profile", saveProfile)

	admin := r.Group("/api/admin")
	admin.Use(requireAdminAuth(authClient), requireAdmin(parseAdminEmails(os.Getenv("ADMIN_EMAILS"))))
	admin.GET("/me", getAdminSession)
	admin.GET("/events", getAdminEvents)
	admin.GET("/events/:eventId/votes", getAdminEventVotes)
	admin.POST("/events", createEvent)
	admin.PUT("/events/:eventId", updateEvent)
	admin.DELETE("/events/:eventId", archiveEvent)
	admin.PUT("/events/:eventId/voting", setEventVotingState)
	admin.POST("/events/:eventId/results/rebuild", rebuildEventResults)
	admin.GET("/events/:eventId/contestants", getAdminContestants)
	admin.POST("/events/:eventId/contestants", createContestant)
	admin.PUT("/events/:eventId/contestants/:id", updateContestant)
	admin.DELETE("/events/:eventId/contestants/:id", deleteContestant)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server běží na portu %s...", port)
	r.Run(":" + port)
}

func getFirebaseConfig(c *gin.Context) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	apiKey := os.Getenv("FIREBASE_API_KEY")
	if projectID == "" || apiKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firebase není nakonfigurován"})
		return
	}

	authDomain := os.Getenv("FIREBASE_AUTH_DOMAIN")
	if authDomain == "" {
		authDomain = projectID + ".firebaseapp.com"
	}

	c.JSON(http.StatusOK, FirebaseConfig{
		APIKey:     apiKey,
		AuthDomain: authDomain,
		ProjectID:  projectID,
		AppID:      os.Getenv("FIREBASE_APP_ID"),
	})
}

func requireAuth(verifier tokenVerifier) gin.HandlerFunc {
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

		token, err := verifier.VerifyIDToken(c.Request.Context(), parts[1])
		if err != nil || token == nil || token.UID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Neplatné přihlášení"})
			return
		}

		c.Set(voterUIDKey, token.UID)
		c.Set(authTokenKey, token)
		c.Next()
	}
}

func authenticatedUID(c *gin.Context) (string, error) {
	uid, ok := c.Get(voterUIDKey)
	if !ok {
		return "", errors.New("authenticated voter UID missing")
	}
	value, ok := uid.(string)
	if !ok || value == "" {
		return "", errors.New("authenticated voter UID invalid")
	}
	return value, nil
}

func getContestants(c *gin.Context) {
	getEventContestants(c, false)
}

func getAdminContestants(c *gin.Context) {
	getEventContestants(c, true)
}

func getEventContestants(c *gin.Context, adminView bool) {
	ctx := c.Request.Context()
	eventID := c.Param("eventId")

	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	eventRef := client.Collection("events").Doc(eventID)
	var event Event
	if adminView {
		doc, err := eventRef.Get(ctx)
		if status.Code(err) == codes.NotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo načíst"})
			return
		}
		if err := doc.DataTo(&event); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data události"})
			return
		}
	} else {
		var exists bool
		var err error
		event, exists, err = getActiveEvent(ctx, eventID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při načítání události"})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
			return
		}
	}
	serveContestantList(c, eventRef, event, adminView)
}

func serveContestantList(c *gin.Context, eventRef *firestore.DocumentRef, event Event, adminView bool) {
	ctx := c.Request.Context()
	showResults := adminView || event.ShowResults

	contestantsMap := make(map[string]*Contestant)
	iter := eventRef.Collection("contestants").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při načítání soutěžících"})
			return
		}
		var contestant Contestant
		if err := doc.DataTo(&contestant); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Neplatná data soutěžícího"})
			return
		}
		contestant.ID = doc.Ref.ID
		contestantsMap[doc.Ref.ID] = &contestant
	}

	statsMap := make(map[string]ResultAggregate)
	if showResults {
		if event.ResultsRebuilding {
			c.Header("Retry-After", "5")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Výsledky se právě přepočítávají"})
			return
		}
		var err error
		if event.ResultsInitialized {
			statsMap, err = loadAggregateStats(ctx, eventRef, contestantsMap)
		} else {
			statsMap, err = loadLegacyStats(ctx, eventRef, contestantsMap)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při načítání výsledků"})
			return
		}
	}

	result := make([]*Contestant, 0)
	for id, contestant := range contestantsMap {
		if stat, ok := statsMap[id]; ok && stat.VoteCount > 0 {
			contestant.AvgScore = float64(stat.TotalScore) / float64(stat.VoteCount)
			contestant.VoteCount = int(stat.VoteCount)
		}
		result = append(result, contestant)
	}

	c.JSON(http.StatusOK, result)
}

func loadAggregateStats(ctx context.Context, eventRef *firestore.DocumentRef, contestants map[string]*Contestant) (map[string]ResultAggregate, error) {
	stats := make(map[string]ResultAggregate)
	if len(contestants) == 0 {
		return stats, nil
	}

	ids := make([]string, 0, len(contestants))
	refs := make([]*firestore.DocumentRef, 0, len(contestants))
	for id := range contestants {
		ids = append(ids, id)
		refs = append(refs, eventRef.Collection("results").Doc(id))
	}
	docs, err := client.GetAll(ctx, refs)
	if err != nil {
		return nil, err
	}
	for index, doc := range docs {
		if !doc.Exists() {
			continue
		}
		var aggregate ResultAggregate
		if err := doc.DataTo(&aggregate); err != nil {
			return nil, err
		}
		if aggregate.VoteCount < 0 {
			continue
		}
		stats[ids[index]] = aggregate
	}
	return stats, nil
}

func loadLegacyStats(ctx context.Context, eventRef *firestore.DocumentRef, contestants map[string]*Contestant) (map[string]ResultAggregate, error) {
	stats := make(map[string]ResultAggregate)
	iter := eventRef.Collection("votes").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		for contestantID, score := range storedScores(doc.Data()["scores"]) {
			if _, exists := contestants[contestantID]; !exists {
				continue
			}
			aggregate := stats[contestantID]
			aggregate.TotalScore += int64(score)
			aggregate.VoteCount++
			stats[contestantID] = aggregate
		}
	}
	return stats, nil
}

func saveVote(c *gin.Context) {
	ctx := c.Request.Context()
	eventID := c.Param("eventId")
	var req VoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatný požadavek"})
		return
	}

	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	event, active, err := getActiveEvent(ctx, eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při ověřování události"})
		return
	}
	if !active {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	if event.VotingStopped {
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasování je zastaveno"})
		return
	}

	if !validVoteShape(req) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Hodnocení obsahuje neplatná data"})
		return
	}

	voterUID, err := authenticatedUID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Přihlášení je vyžadováno"})
		return
	}

	eventRef := client.Collection("events").Doc(eventID)
	ballotRef := eventRef.Collection("votes").Doc(voterUID)
	tombstoneRef := client.Collection(cleanedAnonymousUsersCollection).Doc(voterUID)
	err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		eventDoc, err := tx.Get(eventRef)
		if status.Code(err) == codes.NotFound {
			return errEventInactive
		}
		if err != nil {
			return err
		}
		var currentEvent Event
		if err := eventDoc.DataTo(&currentEvent); err != nil {
			return err
		}
		if currentEvent.Archived {
			return errEventInactive
		}
		if currentEvent.VotingStopped {
			return errVotingStopped
		}
		if err := ensureVoterNotCleaned(tx, tombstoneRef); err != nil {
			return err
		}

		ballot, err := tx.Get(ballotRef)
		if status.Code(err) == codes.NotFound {
			return errMissingProfile
		}
		if err != nil {
			return err
		}
		nickname, ok := ballot.Data()["nickname"].(string)
		if !ok || strings.TrimSpace(nickname) == "" {
			return errMissingProfile
		}
		rateUpdatedAt := time.Now()
		rateTokens, allowed := consumeVoteToken(ballot.Data(), rateUpdatedAt)
		if !allowed {
			return errVoteRateLimited
		}

		stored := storedScores(ballot.Data()["scores"])
		contestantRefs := make([]*firestore.DocumentRef, 0, len(stored)+len(req.Scores))
		seenContestants := make(map[string]struct{}, len(stored)+len(req.Scores))
		for contestantID := range stored {
			seenContestants[contestantID] = struct{}{}
		}
		for contestantID := range req.Scores {
			seenContestants[contestantID] = struct{}{}
		}
		for contestantID := range seenContestants {
			contestantRefs = append(contestantRefs, eventRef.Collection("contestants").Doc(contestantID))
		}
		currentContestantIDs := make(map[string]struct{}, len(contestantRefs))
		if len(contestantRefs) > 0 {
			contestantDocs, err := tx.GetAll(contestantRefs)
			if err != nil {
				return err
			}
			for _, contestantDoc := range contestantDocs {
				if contestantDoc.Exists() {
					currentContestantIDs[contestantDoc.Ref.ID] = struct{}{}
				}
			}
		}
		for contestantID := range req.Scores {
			if _, exists := currentContestantIDs[contestantID]; !exists {
				return errInvalidVote
			}
		}

		if err := tx.Set(ballotRef, map[string]interface{}{
			"scores":            req.Scores,
			"voteRateTokens":    rateTokens,
			"voteRateUpdatedAt": rateUpdatedAt,
		}, firestore.MergeAll); err != nil {
			return err
		}
		if err := indexVoterBallot(tx, voterUID, eventID); err != nil {
			return err
		}
		if currentEvent.ResultsInitialized {
			oldScores := filterScores(stored, currentContestantIDs)
			for contestantID, delta := range resultDeltas(oldScores, req.Scores) {
				if err := tx.Set(eventRef.Collection("results").Doc(contestantID), map[string]interface{}{
					"totalScore": firestore.Increment(delta.TotalScore),
					"voteCount":  firestore.Increment(delta.VoteCount),
				}, firestore.MergeAll); err != nil {
					return err
				}
			}
		}
		return nil
	})

	switch {
	case errors.Is(err, errVotingStopped):
		c.JSON(http.StatusConflict, gin.H{"error": "Hlasování je zastaveno"})
		return
	case errors.Is(err, errEventInactive):
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	case errors.Is(err, errMissingProfile):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nejdříve nastavte přezdívku"})
		return
	case errors.Is(err, errAccountCleaned):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Anonymní účet již není platný; obnovte stránku"})
		return
	case errors.Is(err, errVoteRateLimited):
		c.Header("Retry-After", "1")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Příliš mnoho změn; zkuste to za okamžik"})
		return
	case errors.Is(err, errInvalidVote):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Hodnocení obsahuje odstraněnou hlasovací položku; obnovte stránku"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při ukládání"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func getMyVotes(c *gin.Context) {
	ctx := c.Request.Context()
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	voterUID, err := authenticatedUID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Přihlášení je vyžadováno"})
		return
	}

	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	active, err := activeEventExists(ctx, eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při ověřování události"})
		return
	}
	if !active {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}

	doc, err := client.Collection("events").Doc(eventID).Collection("votes").Doc(voterUID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			c.JSON(http.StatusOK, myVotesResponse(nil))
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Chyba při načítání hodnocení"})
		return
	}

	c.JSON(http.StatusOK, myVotesResponse(doc.Data()))
}

func myVotesResponse(data map[string]interface{}) MyVotesResponse {
	if data == nil {
		return MyVotesResponse{Scores: map[string]interface{}{}}
	}
	nickname, _ := data["nickname"].(string)
	scores, _ := data["scores"].(map[string]interface{})
	if scores == nil {
		scores = map[string]interface{}{}
	}
	return MyVotesResponse{
		Nickname: nickname,
		Scores:   scores,
	}
}

func saveProfile(c *gin.Context) {
	ctx := c.Request.Context()
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	var req ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatný požadavek"})
		return
	}
	req.Nickname = strings.TrimSpace(req.Nickname)
	if !validNickname(req.Nickname) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatná přezdívka"})
		return
	}

	voterUID, err := authenticatedUID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Přihlášení je vyžadováno"})
		return
	}
	eventRef := client.Collection("events").Doc(eventID)
	ballotRef := eventRef.Collection("votes").Doc(voterUID)
	tombstoneRef := client.Collection(cleanedAnonymousUsersCollection).Doc(voterUID)
	err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		eventDoc, err := tx.Get(eventRef)
		if status.Code(err) == codes.NotFound {
			return errEventInactive
		}
		if err != nil {
			return err
		}
		var event Event
		if err := eventDoc.DataTo(&event); err != nil {
			return err
		}
		if event.Archived {
			return errEventInactive
		}
		if err := ensureVoterNotCleaned(tx, tombstoneRef); err != nil {
			return err
		}
		if err := tx.Set(ballotRef, map[string]interface{}{
			"nickname": req.Nickname,
		}, firestore.MergeAll); err != nil {
			return err
		}
		return indexVoterBallot(tx, voterUID, eventID)
	})
	switch {
	case errors.Is(err, errEventInactive):
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	case errors.Is(err, errAccountCleaned):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Anonymní účet již není platný; obnovte stránku"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Přezdívku se nepodařilo uložit"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nickname": req.Nickname})
}

func ensureVoterNotCleaned(tx *firestore.Transaction, tombstoneRef *firestore.DocumentRef) error {
	_, err := tx.Get(tombstoneRef)
	if status.Code(err) == codes.NotFound {
		return nil
	}
	if err != nil {
		return err
	}
	return errAccountCleaned
}

func indexVoterBallot(tx *firestore.Transaction, voterUID, eventID string) error {
	return tx.Set(client.Collection(voterBallotIndexesCollection).Doc(voterUID), map[string]interface{}{
		"eventIds": firestore.ArrayUnion(eventID),
	}, firestore.MergeAll)
}

func validVoteShape(req VoteRequest) bool {
	if req.Scores == nil || len(req.Scores) > maxScoresPerVote {
		return false
	}

	for contestantID, score := range req.Scores {
		if !validDocumentID(contestantID) || score < 0 || score > 10 {
			return false
		}
	}
	return true
}

func validNickname(nickname string) bool {
	nickname = strings.TrimSpace(nickname)
	return nickname != "" && utf8.RuneCountInString(nickname) <= maxNicknameLength
}

func filterScores(scores map[string]int, allowedIDs map[string]struct{}) map[string]int {
	filtered := make(map[string]int)
	for contestantID, score := range scores {
		if _, allowed := allowedIDs[contestantID]; allowed {
			filtered[contestantID] = score
		}
	}
	return filtered
}

func validDocumentID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 1500 && !strings.Contains(id, "/")
}

func eventSlug(name string) string {
	var slug strings.Builder
	pendingSeparator := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if pendingSeparator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(r)
			pendingSeparator = false
		} else {
			pendingSeparator = true
		}
	}
	if slug.Len() == 0 {
		return "event"
	}
	return slug.String()
}

func validStoredScore(value interface{}) (int, bool) {
	score, ok := value.(int64)
	if !ok || score < 0 || score > 10 {
		return 0, false
	}
	return int(score), true
}

func storedScores(value interface{}) map[string]int {
	result := make(map[string]int)
	scores, ok := value.(map[string]interface{})
	if !ok {
		return result
	}
	for contestantID, value := range scores {
		if score, ok := validStoredScore(value); ok {
			result[contestantID] = score
		}
	}
	return result
}

func resultDeltas(oldScores, newScores map[string]int) map[string]ResultDelta {
	deltas := make(map[string]ResultDelta)
	ids := make(map[string]struct{}, len(oldScores)+len(newScores))
	for id := range oldScores {
		ids[id] = struct{}{}
	}
	for id := range newScores {
		ids[id] = struct{}{}
	}

	for id := range ids {
		oldScore, hadOld := oldScores[id]
		newScore, hasNew := newScores[id]
		delta := ResultDelta{}
		switch {
		case hadOld && hasNew:
			delta.TotalScore = int64(newScore - oldScore)
		case hadOld:
			delta.TotalScore = -int64(oldScore)
			delta.VoteCount = -1
		case hasNew:
			delta.TotalScore = int64(newScore)
			delta.VoteCount = 1
		}
		if delta.TotalScore != 0 || delta.VoteCount != 0 {
			deltas[id] = delta
		}
	}
	return deltas
}
