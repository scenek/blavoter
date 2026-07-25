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

func TestSaveStatusRetainsSharedThemeAcrossRuntimeStates(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`status.className = "status-text mt-4 min-h-5 text-center text-sm text-gray-500"`,
		`status.className = "status-text mt-4 min-h-5 text-center text-sm text-amber-700"`,
		`status.className = "status-text status-text--success mt-4 min-h-5 text-center text-sm"`,
		`status.className = "status-text mt-4 min-h-5 text-center text-sm text-red-600"`,
	)
}

func TestVoteSelectionAndSavedStatusUseResultLinkColor(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css,
		`.vote-choice--selected {
    background: var(--color-amber);`,
		`.status-text--success { color: var(--color-amber); }`,
	)

	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`status.className = "status-text status-text--success mt-4 min-h-5 text-center text-sm"`,
	)
	if strings.Contains(body, `status.className = "status-text mt-4 min-h-5 text-center text-sm text-green-700"`) {
		t.Error("successful save status still uses Tailwind green instead of the shared amber status class")
	}
}

func TestSimplePublicPagesUseSemanticThemeComponents(t *testing.T) {
	landing := readUIFile(t, "static/landing.html")
	requireUIContains(t, landing, `class="empty-state panel panel--cut"`, `class="app-link"`)

	profile := readUIFile(t, "static/profile.html")
	requireUIContains(t, profile,
		`class="profile-card panel panel--cut"`,
		`class="field w-full`,
		`class="button button--positive"`,
	)

	results := readUIFile(t, "static/results.html")
	requireUIContains(t, results,
		`row.className = "result-row panel panel--cut"`,
		`rankElement.className = "result-row__rank"`,
		`values.className = "result-row__values"`,
	)
}

func TestAdministrationUsesDarkSemanticControls(t *testing.T) {
	body := readUIFile(t, "static/admin.html")
	requireUIContains(t, body,
		`class="admin-toolbar`,
		`class="admin-section panel panel--cut`,
		`card.className = "admin-card panel panel--cut"`,
		`actions.className = "admin-card__actions"`,
		`editButton.className = "button button--secondary"`,
		`deleteButton.className = "button button--danger"`,
	)
}

func TestVoteMatrixKeepsNamedStickyColumns(t *testing.T) {
	body := readUIFile(t, "static/votes.html")
	requireUIContains(t, body,
		`class="matrix-panel panel panel--cut"`,
		`class="vote-matrix"`,
		`userHeading.className = "matrix-heading matrix-user"`,
		`heading.textContent = option.name || "Položka bez názvu"`,
		`userCell.className = "matrix-user matrix-user--body"`,
		`scoreCell.className = "matrix-score"`,
	)
}

func TestThemeRetainsMobileAndInteractionRequirements(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css,
		"min-height: 2.5rem",
		".vote-choice--selected",
		"border: 2px solid",
		"@media (max-width: 390px)",
		"@media (prefers-reduced-motion: reduce)",
	)
}
