package main

import (
	"os"
	"strings"
	"testing"
)

func readUIFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func requireUIContains(t *testing.T, body string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(body, fragment) {
			t.Errorf("page is missing %q", fragment)
		}
	}
}

func TestApplicationPagesLoadSharedTheme(t *testing.T) {
	pages := []string{
		"static/index.html",
		"static/landing.html",
		"static/profile.html",
		"static/results.html",
		"static/admin.html",
		"static/votes.html",
	}
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			body := readUIFile(t, page)
			requireUIContains(t, body,
				`href="/static/theme.css"`,
				`class="app-page`,
				`app-shell`,
			)
		})
	}
}

func TestThemeDefinesAccessibleSharedPrimitives(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css,
		"--color-page: #171512",
		"--color-surface: #24201b",
		"--color-text: #f4eadb",
		"--color-copper: #d68032",
		"--color-olive: #87945a",
		".panel--cut",
		":focus-visible",
		"@media (prefers-reduced-motion: reduce)",
	)
}

func TestBallotPlacesAggregateResultBesideOptionDetails(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`card.className = "ballot-card panel panel--cut"`,
		`header.className = "ballot-card__header"`,
		`average.className = "ballot-card__result"`,
		`header.appendChild(average)`,
		`controls.className = "vote-scale"`,
		`button.className = "vote-choice"`,
		`button.classList.toggle("vote-choice--selected", active)`,
	)
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css,
		".ballot-card__header",
		"grid-template-columns: minmax(0, 1fr) auto",
		".ballot-card__result",
		".vote-scale",
		"flex-wrap: nowrap",
		"overflow-x: auto",
	)
}
