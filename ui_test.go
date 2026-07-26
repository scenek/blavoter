package main

import (
	"os"
	"regexp"
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

func requireCSSRuleContains(t *testing.T, css, selector string, declarations ...string) {
	t.Helper()
	rulePattern := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(selector) + `\s*\{([^}]*)\}`)
	match := rulePattern.FindStringSubmatch(css)
	if match == nil {
		t.Fatalf("theme is missing rule %q", selector)
	}
	requireUIContains(t, match[1], declarations...)
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

func TestThemeUsesFriendlyModernTypography(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css,
		"family=Bricolage+Grotesque",
		"family=Instrument+Sans",
		`--font-display: "Bricolage Grotesque", sans-serif;`,
		`--font-body: "Instrument Sans", system-ui, sans-serif;`,
	)
}

func TestBallotPlacesAggregateResultBesideOptionDetails(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`card.className = "ballot-card panel panel--cut"`,
		`summary.className = "ballot-card__summary"`,
		`average.className = "ballot-card__result"`,
		`summary.appendChild(average)`,
		`scale.className = "vote-scale"`,
		`button.className = "vote-choice"`,
		`button.classList.toggle("vote-choice--selected", active)`,
	)
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css,
		".ballot-card__result",
		".vote-scale",
	)
}

func TestBallotUsesResponsiveScoreGridWithoutHorizontalScrolling(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`controls.className = "vote-controls"`,
		`scale.className = "vote-scale"`,
		`const choices = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10]`,
		`unratedButton.className = "vote-choice vote-choice--unrated"`,
		`createChoiceButton(null, unratedButton)`,
	)

	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".vote-scale",
		"display: grid;",
		"grid-template-columns: repeat(11, minmax(0, 1fr));",
	)
	requireCSSRuleContains(t, css, ".vote-choice",
		"min-height: 2.75rem;",
		"min-width: 0;",
	)
	requireUIContains(t, css,
		"@media (max-width: 600px)",
		"grid-template-columns: repeat(6, minmax(0, 1fr));",
		".vote-choice--unrated",
		"width: 100%;",
	)
	if strings.Contains(css, "overflow-x: auto") {
		t.Error("voting selector still relies on horizontal scrolling")
	}
}

func TestExpandableBallotSummaryIsTouchFriendlyAndResponsive(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".ballot-card__summary",
		"cursor: pointer;",
		"min-height: 2.75rem;",
	)
	requireCSSRuleContains(t, css, ".ballot-card[open] .ballot-card__chevron",
		"transform: rotate(90deg);",
	)
	requireCSSRuleContains(t, css, ".ballot-card__content",
		"border-top: 1px solid var(--color-border);",
	)
	requireUIContains(t, css,
		"@media (max-width: 600px)",
		".ballot-card__summary",
		".ballot-card__personal",
		"@media (prefers-reduced-motion: reduce)",
	)
	if strings.Contains(css, "overflow-x: auto") {
		t.Error("expandable ballot still relies on horizontal scrolling")
	}
}

func TestClippedBallotCardKeepsSummaryFocusRingInsideBounds(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".ballot-card",
		"overflow: clip;",
	)
	requireCSSRuleContains(t, css, ".ballot-card__summary:focus-visible",
		"box-shadow: inset 0 0 0 2px var(--color-amber);",
	)
}

func TestBallotRendersPrivateNoteEditor(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`import { createNoteAutosave } from "/static/note-autosave.mjs";`,
		`let myNotes = {};`,
		`data.notes`,
		`toggle.textContent = hasNote ? "Upravit poznámku" : "Přidat poznámku";`,
		`textarea.setAttribute("aria-label",`,
		`privacy.textContent = "Soukromá poznámka – vidíš ji jen ty.";`,
		`Array.from(textarea.value).length`,
		`Array.from(textarea.value).slice(0, 300).join("")`,
		`delayMs: 700`,
		`/notes/${encodeURIComponent(c.id)}`,
		`method: "PUT"`,
		`status.textContent = "Ukládám…";`,
		`status.textContent = "Uloženo";`,
		`status.textContent = "Poznámku se nepodařilo uložit. Zkuste to znovu.";`,
		`toggle.textContent = "Zobrazit poznámku";`,
		`textarea.readOnly = true;`,
	)

	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".ballot-note__toggle",
		"min-height: 2.75rem;",
	)
	requireUIContains(t, css, ".ballot-note__hide { min-height: 2.75rem; }")
	requireCSSRuleContains(t, css, ".ballot-note__preview",
		"overflow: hidden;",
		"text-overflow: ellipsis;",
		"white-space: nowrap;",
	)
	requireCSSRuleContains(t, css, ".ballot-note__textarea",
		"min-height: 5rem;",
		"width: 100%;",
	)
	requireCSSRuleContains(t, css, ".ballot-note__status--success",
		"color: var(--color-amber);",
	)
	requireCSSRuleContains(t, css, ".ballot-note__status--error",
		"color: #f3aaa0;",
	)
}

func TestBallotPlacesAggregateResultInSummaryWhenResultsAreShown(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	resultBranch := regexp.MustCompile(`if \(showResults\) \{[\s\S]*?` + regexp.QuoteMeta(`summary.appendChild(average)`))
	if !resultBranch.MatchString(body) {
		t.Error("aggregate result is not appended to the summary inside the showResults branch")
	}
}

func TestBallotUsesIndependentExpandableOptionRows(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`const expandedContestantIds = new Set();`,
		`const card = document.createElement("details");`,
		`const summary = document.createElement("summary");`,
		`summary.className = "ballot-card__summary";`,
		`const expandedContent = document.createElement("div");`,
		`expandedContent.className = "ballot-card__content";`,
		`card.open = expandedContestantIds.has(c.id);`,
		`card.addEventListener("toggle",`,
		`expandedContestantIds.add(c.id);`,
		`expandedContestantIds.delete(c.id);`,
		`expandedContent.append(description, controls);`,
	)
}

func TestResultHeaderActionsAreProminentAndDoNotWrap(t *testing.T) {
	ballot := readUIFile(t, "static/index.html")
	results := readUIFile(t, "static/results.html")
	requireUIContains(t, ballot,
		`class="header-action header-action--results"`,
		`aria-label="Zobrazit výsledky"`,
	)
	requireUIContains(t, results,
		`class="results-header-actions"`,
		`class="button button--secondary results-refresh"`,
		`Obnovit výsledky`,
	)

	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".header-action--results",
		"border: 1px solid var(--color-amber);",
		"color: var(--color-amber);",
	)
	requireCSSRuleContains(t, css, ".results-refresh",
		"min-height: 2.875rem;",
		"white-space: nowrap;",
	)
	if strings.Index(css, ".results-refresh {") < strings.Index(css, ".button {") {
		t.Error("results refresh sizing is overridden by the later generic button rule")
	}
}

func TestBallotProfileActionIsTouchFriendlyAndDoesNotWrap(t *testing.T) {
	ballot := readUIFile(t, "static/index.html")
	requireUIContains(t, ballot,
		`id="profileLink" href="#" class="app-link ballot-profile-action"`,
	)

	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".ballot-profile-action",
		"max-width: 100%;",
		"min-height: 2.75rem;",
		"overflow: hidden;",
		"text-overflow: ellipsis;",
		"white-space: nowrap;",
	)
}

func TestBallotSummaryShowsAndRefreshesPersonalScore(t *testing.T) {
	body := readUIFile(t, "static/index.html")
	requireUIContains(t, body,
		`personalLabel.textContent = "Tvoje hodnocení";`,
		`personalValue.textContent = selected === null ? "Nehodnoceno" : String(selected);`,
		`refreshPersonalScore();`,
		`for (const contestantId of expandedContestantIds)`,
		`expandedContestantIds.delete(contestantId);`,
	)
}

func TestThemeProvidesAccessibleTouchAndControlStates(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireUIContains(t, css, "--color-control-border: #8f7f6d;")
	requireCSSRuleContains(t, css, ".app-link",
		"align-items: center;",
		"display: inline-flex;",
	)
	for _, selector := range []string{".button", ".field", ".vote-choice"} {
		requireCSSRuleContains(t, css, selector,
			"border: 1px solid var(--color-control-border);",
		)
	}
	requireCSSRuleContains(t, css, ".matrix-score--empty",
		"color: #a19484;",
	)
}

func TestAdminScrollRespectsReducedMotion(t *testing.T) {
	body := readUIFile(t, "static/admin.html")
	requireUIContains(t, body,
		`window.matchMedia("(prefers-reduced-motion: reduce)").matches`,
		`contestantForm.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth" });`,
	)
}

func TestRuntimeStatusesUseSemanticThemeColors(t *testing.T) {
	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".status-text--warning",
		"color: var(--color-amber);",
	)
	requireCSSRuleContains(t, css, ".status-text--error",
		"color: #f3aaa0;",
	)

	ballot := readUIFile(t, "static/index.html")
	requireUIContains(t, ballot,
		`class="muted text-sm">Ohodnoť položky`,
		`id="eventDescription" class="text-sm muted"`,
		`status.className = "status-text mt-4 min-h-5 text-center text-sm"`,
		`status.className = "status-text status-text--warning mt-4 min-h-5 text-center text-sm"`,
		`status.className = "status-text status-text--success mt-4 min-h-5 text-center text-sm"`,
		`status.className = "status-text status-text--error mt-4 min-h-5 text-center text-sm"`,
	)

	profile := readUIFile(t, "static/profile.html")
	requireUIContains(t, profile,
		`message.className = "status-text text-sm"`,
		`message.className = "status-text status-text--success text-sm"`,
		`message.className = "status-text status-text--error text-sm"`,
	)

	oldRuntimeColor := regexp.MustCompile(`(?:status|message)\.className\s*=\s*"[^"]*\btext-(?:gray|amber|green|red)-\d+`)
	for page, body := range map[string]string{
		"ballot":  ballot,
		"profile": profile,
	} {
		if assignment := oldRuntimeColor.FindString(body); assignment != "" {
			t.Errorf("%s still uses a Tailwind runtime status color: %s", page, assignment)
		}
	}
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

func TestVoteCountDisplaysUseSharedCzechFormatter(t *testing.T) {
	numericVoteCount := regexp.MustCompile(`\$\{[^}]+\}\s*hlasů`)
	pages := []string{
		"static/index.html",
		"static/results.html",
		"static/admin.html",
	}
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			body := readUIFile(t, page)
			requireUIContains(t, body,
				`import { formatVoteCount } from "/static/vote-count.mjs";`,
				"formatVoteCount(",
			)
			if numericVoteCount.MatchString(body) {
				t.Error("page still renders a numeric template literal ending in hlasů")
			}
		})
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

func TestAdministrationErrorLinkUsesStandaloneLayout(t *testing.T) {
	body := readUIFile(t, "static/votes.html")
	requireUIContains(t, body,
		`link.className = "app-link app-link--standalone mt-3 block"`,
	)

	css := readUIFile(t, "static/theme.css")
	requireCSSRuleContains(t, css, ".app-link--standalone",
		"display: flex;",
		"width: fit-content;",
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
