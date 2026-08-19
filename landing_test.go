package main

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestPublicLandingPageLeadsWithVotingLink(t *testing.T) {
	file, err := os.Open("docs/index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	document, err := html.Parse(file)
	if err != nil {
		t.Fatal(err)
	}
	anchors := htmlElements(document, "a")
	if len(anchors) < 2 {
		t.Fatalf("landing page has %d links, want voting link followed by poster link", len(anchors))
	}

	votingLink := anchors[0]
	if got := strings.TrimSpace(htmlText(votingLink)); got != "Hlasování Pivní cirkus 2026" {
		t.Fatalf("first link text = %q, want voting call to action", got)
	}
	if got := htmlAttribute(votingLink, "href"); got != "/event/9JNv5iAost2izyYxJaff/pivn%C3%AD-cirkus-2026" {
		t.Fatalf("voting link href = %q", got)
	}
	if images := htmlElements(anchors[1], "img"); len(images) != 1 || htmlAttribute(images[0], "src") != "pivni-cirkus.jpg" {
		t.Fatal("poster link must remain immediately after the voting link")
	}
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
