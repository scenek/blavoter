package main

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestPublicLandingPagePresentsThe2026EventActionsAndMenu(t *testing.T) {
	file, err := os.Open("docs/index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	document, err := html.Parse(file)
	if err != nil {
		t.Fatal(err)
	}
	headings := htmlElements(document, "h1")
	if len(headings) != 1 || normalizedHTMLText(headings[0]) != "Pivní cirkus Blažovice 2026" {
		t.Fatal("landing page must lead with the 2026 event heading")
	}
	titles := htmlElements(document, "title")
	if len(titles) != 1 || normalizedHTMLText(titles[0]) != "Pivní cirkus Blažovice 2026" {
		t.Fatal("browser title must match the visible event heading")
	}
	mainElements := htmlElements(document, "main")
	if len(mainElements) != 1 {
		t.Fatal("landing page must have one main content area")
	}
	wantOrder := []string{"h1", "a", "a", "img"}
	children := directHTMLElementChildren(mainElements[0])
	if len(children) != len(wantOrder) {
		t.Fatalf("main content has %d sections, want heading, two actions, and menu image", len(children))
	}
	for index, wantTag := range wantOrder {
		if children[index].Data != wantTag {
			t.Fatalf("main section %d = %s, want %s", index, children[index].Data, wantTag)
		}
	}

	anchors := htmlElements(document, "a")
	if len(anchors) != 2 {
		t.Fatalf("landing page has %d links, want voting and Facebook links", len(anchors))
	}
	votingLink := anchors[0]
	if got := normalizedHTMLText(votingLink); got != "Hlasování" {
		t.Fatalf("first link text = %q, want voting call to action", got)
	}
	if got := htmlAttribute(votingLink, "href"); got != "/event/9JNv5iAost2izyYxJaff/pivn%C3%AD-cirkus-2026" {
		t.Fatalf("voting link href = %q", got)
	}

	facebookLink := anchors[1]
	if got := normalizedHTMLText(facebookLink); got != "Facebook" {
		t.Fatalf("Facebook link text = %q", got)
	}
	if got := htmlAttribute(facebookLink, "href"); got != "https://www.facebook.com/events/2145978482970443?" {
		t.Fatalf("Facebook link href = %q", got)
	}
	if htmlAttribute(facebookLink, "target") != "_blank" || htmlAttribute(facebookLink, "rel") != "noopener noreferrer" {
		t.Fatal("Facebook link must open safely in a new tab")
	}
	if icons := htmlElements(facebookLink, "svg"); len(icons) != 1 {
		t.Fatal("Facebook link must include one inline icon")
	}

	images := htmlElements(document, "img")
	if len(images) != 1 || htmlAttribute(images[0], "src") != "stepis.jpg" {
		t.Fatal("landing page must show stepis.jpg as its only image")
	}
	if strings.TrimSpace(htmlAttribute(images[0], "alt")) == "" {
		t.Fatal("stepis.jpg must have descriptive alternative text")
	}
}

func directHTMLElementChildren(node *html.Node) []*html.Node {
	var elements []*html.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			elements = append(elements, child)
		}
	}
	return elements
}

func htmlElements(node *html.Node, tag string) []*html.Node {
	var elements []*html.Node
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == tag {
			elements = append(elements, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return elements
}

func htmlAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func htmlText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(htmlText(child))
	}
	return text.String()
}

func normalizedHTMLText(node *html.Node) string {
	return strings.Join(strings.Fields(htmlText(node)), " ")
}
