package main

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func getEvent(c *gin.Context) {
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}

	event, exists, err := getActiveEvent(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo načíst"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	c.JSON(http.StatusOK, event)
}

func serveEventPage(c *gin.Context) {
	serveCanonicalEventFile(c, "./static/index.html", "")
}

func serveEventProfilePage(c *gin.Context) {
	serveCanonicalEventFile(c, "./static/profile.html", "/profile")
}

func serveEventResultsPage(c *gin.Context) {
	serveCanonicalEventFile(c, "./static/results.html", "/results")
}

func getPublicResults(c *gin.Context) {
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Neplatné ID události"})
		return
	}
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firestore client není připojen"})
		return
	}
	event, exists, err := getActiveEvent(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Událost se nepodařilo načíst"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Událost nebyla nalezena"})
		return
	}
	if !event.ShowResults {
		c.JSON(http.StatusForbidden, gin.H{"error": "Výsledky této události nejsou veřejné"})
		return
	}
	serveContestantList(c, client.Collection("events").Doc(eventID), event, false)
}

func serveCanonicalEventFile(c *gin.Context, filename string, suffix string) {
	eventID := c.Param("eventId")
	if !validDocumentID(eventID) {
		c.Status(http.StatusNotFound)
		return
	}
	if client == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	event, exists, err := getActiveEvent(c.Request.Context(), eventID)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if !exists {
		c.Status(http.StatusNotFound)
		return
	}

	if c.Param("slug") != event.Slug {
		c.Redirect(http.StatusMovedPermanently, "/event/"+url.PathEscape(event.ID)+"/"+url.PathEscape(event.Slug)+suffix)
		return
	}
	c.File(filename)
}

func getActiveEvent(ctx context.Context, eventID string) (Event, bool, error) {
	doc, err := client.Collection("events").Doc(eventID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}

	var event Event
	if err := doc.DataTo(&event); err != nil {
		return Event{}, false, err
	}
	if event.Archived {
		return Event{}, false, nil
	}
	event.ID = doc.Ref.ID
	event.Slug = eventSlug(event.Name)
	return event, true, nil
}

func activeEventExists(ctx context.Context, eventID string) (bool, error) {
	_, exists, err := getActiveEvent(ctx, eventID)
	return exists, err
}
