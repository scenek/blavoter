# Dark Taproom UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle every Blavoter application page with the approved dark taproom visual system and place public aggregate results at the right side of each ballot option.

**Architecture:** Add a shared `static/theme.css` containing design tokens and semantic component classes, then adopt those classes in all six application HTML files while preserving their existing IDs, routes, and behavior. Add Go-based static UI contract tests so shared stylesheet loading, page roles, ballot result placement, and matrix labeling remain regression-tested without introducing a JavaScript build system.

**Tech Stack:** Go 1.24 tests, Gin static file serving, HTML5, CSS custom properties, existing Tailwind CDN utilities, vanilla JavaScript, Firebase Web SDK

## Global Constraints

- Apply the theme to the landing page, event ballot, voter profile, public and administrator results, administration, and administrator vote matrix.
- Do not change authentication, voting, administration, routing, or data behavior.
- Use near-black charcoal, dark brown, graphite, warm off-white, copper, amber, muted olive, brick red, taupe, and warm borders.
- Use angular cards with restrained corner cuts or asymmetric corner radii.
- Keep public aggregate results at the upper-right of each ballot card on phones and desktops.
- Omit the ballot result column entirely when public results are disabled.
- Keep `Nehodnoceno` and values 0–10 in one horizontal, optionally scrollable row.
- Keep touch targets approximately 40 pixels high and expose selection through color plus border/shape.
- Preserve existing live regions, labels, button semantics, disabled states, option names, and reduced-motion behavior.
- Leave the promotional poster page under `docs/` visually unchanged.
- Keep existing DOM IDs and JavaScript data flow stable.

---

## File Structure

- Create `static/theme.css`: shared tokens, typography, application shells, navigation, panels, buttons, fields, notices, ballot cards, result rows, administration controls, and matrix styling.
- Create `ui_test.go`: static contracts for theme loading and critical semantic classes.
- Modify `static/index.html`: themed event ballot and right-aligned aggregate result block.
- Modify `static/landing.html`: themed no-event state.
- Modify `static/profile.html`: themed profile form.
- Modify `static/results.html`: themed public/admin ranking rows.
- Modify `static/admin.html`: themed administration shell, forms, action toolbar, and contestant cards.
- Modify `static/votes.html`: themed administrator vote matrix and pagination.

### Task 1: Shared Theme Foundation

**Files:**
- Create: `static/theme.css`
- Create: `ui_test.go`
- Modify: `static/index.html`
- Modify: `static/landing.html`
- Modify: `static/profile.html`
- Modify: `static/results.html`
- Modify: `static/admin.html`
- Modify: `static/votes.html`

**Interfaces:**
- Consumes: Gin's existing `r.Static("/static", "./static")` route.
- Produces: `/static/theme.css`; shared classes `app-page`, `app-shell`, `app-shell--narrow`, `app-shell--wide`, `app-header`, `eyebrow`, `panel`, `panel--cut`, `app-link`, `button`, `button--primary`, `button--positive`, `button--danger`, `field`, `notice`, and `status-text`.

- [ ] **Step 1: Write the failing shared-theme contract test**

Create `ui_test.go`:

```go
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
```

- [ ] **Step 2: Run the contract test and verify it fails**

Run:

```bash
go test ./... -run 'TestApplicationPagesLoadSharedTheme|TestThemeDefinesAccessibleSharedPrimitives'
```

Expected: FAIL because `static/theme.css` does not exist and the pages do not load it.

- [ ] **Step 3: Create the shared stylesheet**

Create `static/theme.css` with these complete foundation rules:

```css
@import url("https://fonts.googleapis.com/css2?family=Oswald:wght@500;600;700&family=Source+Sans+3:wght@400;500;600;700&display=swap");

:root {
    color-scheme: dark;
    --color-page: #171512;
    --color-surface: #24201b;
    --color-surface-raised: #302a23;
    --color-text: #f4eadb;
    --color-muted: #b8aa98;
    --color-copper: #d68032;
    --color-amber: #f0ad45;
    --color-olive: #87945a;
    --color-danger: #b9513f;
    --color-border: #51463a;
    --color-inset: #110f0d;
    --font-display: "Oswald", "Arial Narrow", sans-serif;
    --font-body: "Source Sans 3", system-ui, sans-serif;
    --radius-cut: 3px 18px 3px 18px;
    --focus-ring: 0 0 0 3px rgb(240 173 69 / 35%);
}

* { box-sizing: border-box; }
html { min-height: 100%; background: var(--color-page); }
body { margin: 0; }
button, input, select, textarea { font: inherit; }
button, a { -webkit-tap-highlight-color: transparent; }
[hidden] { display: none !important; }

.app-page {
    min-height: 100vh;
    background:
        radial-gradient(circle at 15% 0%, rgb(214 128 50 / 10%), transparent 32rem),
        linear-gradient(145deg, #1d1915 0%, var(--color-page) 52%, #11100e 100%);
    color: var(--color-text);
    font-family: var(--font-body);
    padding: 1rem;
}
.app-shell { width: min(100%, 44rem); margin-inline: auto; }
.app-shell--narrow { width: min(100%, 34rem); }
.app-shell--wide { width: min(100%, 88rem); }
.app-header { margin-block: 1.5rem; }
.app-header h1, .display-heading {
    color: var(--color-text);
    font-family: var(--font-display);
    font-weight: 700;
    letter-spacing: .015em;
    line-height: 1.05;
}
.eyebrow {
    color: var(--color-amber);
    font-family: var(--font-display);
    font-size: .78rem;
    font-weight: 700;
    letter-spacing: .14em;
    text-transform: uppercase;
}
.muted { color: var(--color-muted); }
.app-link {
    color: var(--color-amber);
    font-weight: 600;
    text-decoration-color: transparent;
    text-underline-offset: .2em;
}
.app-link:hover { text-decoration-color: currentColor; }

.panel {
    background: linear-gradient(145deg, var(--color-surface-raised), var(--color-surface));
    border: 1px solid var(--color-border);
    box-shadow: 0 14px 34px rgb(0 0 0 / 22%), inset 0 1px rgb(255 255 255 / 4%);
    color: var(--color-text);
}
.panel--cut { border-radius: var(--radius-cut); }

.button {
    align-items: center;
    border: 1px solid var(--color-border);
    border-radius: 2px 12px 2px 12px;
    cursor: pointer;
    display: inline-flex;
    font-weight: 700;
    justify-content: center;
    min-height: 2.5rem;
    padding: .55rem 1rem;
    transition: background-color .15s ease, border-color .15s ease, transform .15s ease;
}
.button:hover:not(:disabled) { border-color: var(--color-amber); transform: translateY(-1px); }
.button:disabled { cursor: not-allowed; opacity: .45; }
.button--primary { background: var(--color-copper); border-color: var(--color-copper); color: #17110c; }
.button--positive { background: var(--color-olive); border-color: var(--color-olive); color: #10120a; }
.button--danger { background: var(--color-danger); border-color: var(--color-danger); color: #fff7ef; }
.button--secondary { background: var(--color-surface); color: var(--color-text); }

.field {
    background: var(--color-inset);
    border: 1px solid var(--color-border);
    border-radius: 2px 12px 2px 12px;
    color: var(--color-text);
    min-height: 2.6rem;
}
.field::placeholder { color: #817465; }
.notice {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-left: 4px solid var(--color-amber);
    border-radius: 2px 12px 2px 12px;
    color: var(--color-muted);
}
.notice--error { border-left-color: var(--color-danger); color: #f3aaa0; }
.notice--success { border-left-color: var(--color-olive); color: #cbd7a4; }
.status-text { color: var(--color-muted); }

:focus-visible {
    outline: 2px solid var(--color-amber);
    outline-offset: 2px;
    box-shadow: var(--focus-ring);
}
@media (prefers-reduced-motion: reduce) {
    *, *::before, *::after {
        scroll-behavior: auto !important;
        transition-duration: .01ms !important;
    }
}
```

- [ ] **Step 4: Load the stylesheet and assign page shells**

Immediately after the Tailwind script in each application page add:

```html
<link rel="stylesheet" href="/static/theme.css">
```

Use these exact outer classes while retaining any layout utilities needed by the
page:

```html
<!-- static/index.html -->
<body class="app-page"><div class="app-shell">

<!-- static/landing.html -->
<body class="app-page flex items-center justify-center"><main class="app-shell app-shell--narrow">

<!-- static/profile.html -->
<body class="app-page"><div class="app-shell app-shell--narrow">

<!-- static/results.html -->
<body class="app-page"><div class="app-shell">

<!-- static/admin.html -->
<body class="app-page"><div class="app-shell app-shell--wide">

<!-- static/votes.html -->
<body class="app-page"><div class="app-shell app-shell--wide">
```

Close each newly added wrapper immediately before its script or before
`</body>`, without moving or renaming existing elements.

- [ ] **Step 5: Run shared-theme tests**

Run:

```bash
go test ./... -run 'TestApplicationPagesLoadSharedTheme|TestThemeDefinesAccessibleSharedPrimitives'
```

Expected: PASS.

- [ ] **Step 6: Commit the foundation**

```bash
git add ui_test.go static/theme.css static/index.html static/landing.html static/profile.html static/results.html static/admin.html static/votes.html
git commit -m "style: add shared dark taproom theme"
```

### Task 2: Ballot Cards and Right-Aligned Results

**Files:**
- Modify: `ui_test.go`
- Modify: `static/theme.css`
- Modify: `static/index.html`

**Interfaces:**
- Consumes: shared colors and panel primitives from Task 1; contestant fields `name`, `description`, `avgScore`, and `voteCount`; existing `showResults`, `pendingScores`, and `votingStopped` state.
- Produces: semantic classes `ballot-card`, `ballot-card__header`, `ballot-card__details`, `ballot-card__result`, `ballot-card__average`, `ballot-card__count`, `vote-scale`, and `vote-choice`.

- [ ] **Step 1: Add a failing ballot structure test**

Append to `ui_test.go`:

```go
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
```

- [ ] **Step 2: Run the ballot test and verify it fails**

Run:

```bash
go test ./... -run TestBallotPlacesAggregateResultBesideOptionDetails
```

Expected: FAIL because the semantic ballot classes and two-column header do not exist.

- [ ] **Step 3: Add ballot component CSS**

Append to `static/theme.css`:

```css
.ballot-list { display: grid; gap: .85rem; }
.ballot-card { padding: 1rem; }
.ballot-card__header {
    align-items: start;
    display: grid;
    gap: 1rem;
    grid-template-columns: minmax(0, 1fr) auto;
}
.ballot-card__details { min-width: 0; }
.ballot-card__name {
    font-family: var(--font-display);
    font-size: 1.25rem;
    letter-spacing: .015em;
    line-height: 1.15;
}
.ballot-card__description { color: var(--color-muted); font-size: .85rem; margin-top: .2rem; }
.ballot-card__result {
    border-left: 1px solid var(--color-border);
    min-width: 5.25rem;
    padding-left: .85rem;
    text-align: right;
}
.ballot-card__average {
    color: var(--color-amber);
    display: block;
    font-family: var(--font-display);
    font-size: 1.45rem;
    font-weight: 700;
    line-height: 1;
}
.ballot-card__count { color: var(--color-muted); display: block; font-size: .72rem; margin-top: .25rem; }
.vote-scale {
    display: flex;
    flex-wrap: nowrap;
    gap: .35rem;
    margin-top: .9rem;
    overflow-x: auto;
    padding: .15rem .1rem .45rem;
    scrollbar-color: var(--color-border) transparent;
}
.vote-choice {
    background: var(--color-inset);
    border: 1px solid var(--color-border);
    border-radius: 2px 9px 2px 9px;
    color: var(--color-text);
    flex: 0 0 auto;
    font-weight: 700;
    min-height: 2.5rem;
    min-width: 2.5rem;
    padding: .45rem .7rem;
}
.vote-choice:hover:not(:disabled) { border-color: var(--color-amber); color: var(--color-amber); }
.vote-choice--selected {
    background: var(--color-olive);
    border: 2px solid #c4d184;
    color: #11140b;
}
.vote-choice:disabled { cursor: not-allowed; opacity: .45; }
@media (max-width: 390px) {
    .ballot-card { padding: .85rem; }
    .ballot-card__header { gap: .65rem; }
    .ballot-card__result { min-width: 4.5rem; padding-left: .6rem; }
}
```

- [ ] **Step 4: Render the semantic two-column ballot header**

In `static/index.html`, replace the contestant card construction through
`details.append(...)` with:

```js
const card = document.createElement("article");
card.className = "ballot-card panel panel--cut";

const header = document.createElement("div");
header.className = "ballot-card__header";
const details = document.createElement("div");
details.className = "ballot-card__details";
const name = document.createElement("h3");
name.className = "ballot-card__name";
name.textContent = c.name;

const description = document.createElement("p");
description.className = "ballot-card__description";
description.textContent = c.description;
details.append(name, description);
header.appendChild(details);

if (showResults) {
    const average = document.createElement("div");
    average.className = "ballot-card__result";
    const averageValue = document.createElement("span");
    averageValue.className = "ballot-card__average";
    averageValue.textContent = c.voteCount > 0 ? c.avgScore.toFixed(1) : "—";
    const voteCount = document.createElement("span");
    voteCount.className = "ballot-card__count";
    voteCount.textContent = c.voteCount > 0 ? `${c.voteCount} hlasů` : "Nehodnoceno";
    average.append(averageValue, voteCount);
    header.appendChild(average);
}
```

- [ ] **Step 5: Convert voting controls to stable semantic classes**

Use:

```js
const controls = document.createElement("div");
controls.className = "vote-scale";
```

Set the base class once when each button is created:

```js
button.className = "vote-choice";
```

Replace the class-string assignment in `refreshChoices` with:

```js
entry.button.className = "vote-choice";
entry.button.classList.toggle("vote-choice--selected", active);
```

Finish each card with:

```js
card.append(header, controls);
```

Give the static list `class="ballot-list"` and theme the page header,
description panel, stopped notice, navigation links, and save status using
`app-header`, `panel panel--cut`, `notice`, `app-link`, and `status-text`.

- [ ] **Step 6: Run the ballot and shared-theme tests**

Run:

```bash
go test ./... -run 'TestBallotPlacesAggregateResultBesideOptionDetails|TestApplicationPagesLoadSharedTheme'
```

Expected: PASS.

- [ ] **Step 7: Commit the ballot redesign**

```bash
git add ui_test.go static/theme.css static/index.html
git commit -m "style: align ballot results beside options"
```

### Task 3: Landing, Profile, and Ranking Pages

**Files:**
- Modify: `ui_test.go`
- Modify: `static/theme.css`
- Modify: `static/landing.html`
- Modify: `static/profile.html`
- Modify: `static/results.html`

**Interfaces:**
- Consumes: shared panel, button, field, notice, heading, and link primitives.
- Produces: `empty-state`, `profile-card`, `results-list`, `result-row`, `result-row__rank`, `result-row__details`, and `result-row__values`.

- [ ] **Step 1: Add failing public-page contracts**

Append to `ui_test.go`:

```go
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
```

- [ ] **Step 2: Run the public-page test and verify it fails**

Run:

```bash
go test ./... -run TestSimplePublicPagesUseSemanticThemeComponents
```

Expected: FAIL on missing semantic classes.

- [ ] **Step 3: Add focused public-page CSS**

Append to `static/theme.css`:

```css
.empty-state { padding: 2rem; text-align: center; }
.profile-card { padding: 1.25rem; }
.results-list { display: grid; gap: .7rem; }
.result-row {
    align-items: center;
    display: grid;
    gap: 1rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: 1rem;
}
.result-row__rank {
    align-items: center;
    background: var(--color-inset);
    border: 1px solid var(--color-copper);
    border-radius: 2px 10px 2px 10px;
    color: var(--color-amber);
    display: inline-flex;
    font-family: var(--font-display);
    font-size: 1.15rem;
    font-weight: 700;
    height: 2.5rem;
    justify-content: center;
    width: 2.5rem;
}
.result-row:nth-child(1) .result-row__rank { background: var(--color-copper); color: #17110c; }
.result-row__details { min-width: 0; }
.result-row__values {
    border-left: 1px solid var(--color-border);
    min-width: 6rem;
    padding-left: .9rem;
    text-align: right;
}
.result-row__average { color: var(--color-amber); font-family: var(--font-display); font-size: 1.45rem; font-weight: 700; }
.result-row__count { color: var(--color-muted); font-size: .75rem; }
@media (max-width: 390px) {
    .result-row { gap: .6rem; padding: .8rem; }
    .result-row__values { min-width: 4.8rem; padding-left: .55rem; }
}
```

- [ ] **Step 4: Theme the landing and profile markup**

Use this complete landing main content:

```html
<main class="app-shell app-shell--narrow">
    <section class="empty-state panel panel--cut">
        <p class="muted mb-6">
            Pro hlasování použijte unikátní odkaz události, který jste obdrželi od pořadatele.
        </p>
        <a href="/admin" class="app-link">Administrace</a>
    </section>
</main>
```

In `profile.html`, assign `app-header` to the header, `app-link` to the back
link, `display-heading` to the heading, `muted` to the event name,
`profile-card panel panel--cut` to the main panel, `field w-full border p-2` to
the nickname input, `button button--positive` to the save button, and
`status-text` to the status message. Keep every ID and script unchanged.

- [ ] **Step 5: Theme ranking rendering and status messages**

In `results.html`, use `app-header`, `app-link`, `eyebrow`, `display-heading`,
`muted`, `notice`, and `results-list` in static markup. Replace dynamic row
classes with:

```js
row.className = "result-row panel panel--cut";
rankElement.className = "result-row__rank";
details.className = "result-row__details";
name.className = "font-bold";
description.className = "muted text-sm";
values.className = "result-row__values";
average.className = "result-row__average";
count.className = "result-row__count";
```

Use `notice` for loading/empty messages and `notice notice--error` for errors;
retain `role="status"` and existing text.

- [ ] **Step 6: Run public-page and full Go tests**

Run:

```bash
go test ./... -run 'TestSimplePublicPagesUseSemanticThemeComponents|TestApplicationPagesLoadSharedTheme'
go test ./...
```

Expected: both commands PASS.

- [ ] **Step 7: Commit public pages**

```bash
git add ui_test.go static/theme.css static/landing.html static/profile.html static/results.html
git commit -m "style: theme public profile and results pages"
```

### Task 4: Administration Interface

**Files:**
- Modify: `ui_test.go`
- Modify: `static/theme.css`
- Modify: `static/admin.html`

**Interfaces:**
- Consumes: existing admin authentication, event CRUD, voting state, rebuild, and contestant CRUD handlers; shared theme primitives.
- Produces: `admin-toolbar`, `admin-grid`, `admin-section`, `admin-card`, `admin-card__details`, and `admin-card__actions`.

- [ ] **Step 1: Add a failing administration contract**

Append to `ui_test.go`:

```go
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
```

- [ ] **Step 2: Run the administration test and verify it fails**

Run:

```bash
go test ./... -run TestAdministrationUsesDarkSemanticControls
```

Expected: FAIL because admin semantic classes are absent.

- [ ] **Step 3: Add administration layout CSS**

Append to `static/theme.css`:

```css
.admin-toolbar { align-items: center; display: flex; flex-wrap: wrap; gap: .55rem; }
.admin-grid { display: grid; gap: 1rem; }
.admin-section { padding: 1.25rem; }
.admin-card {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    justify-content: space-between;
    padding: 1rem;
}
.admin-card__details { flex: 1 1 18rem; min-width: 0; }
.admin-card__actions { display: flex; flex-wrap: wrap; gap: .5rem; }
.admin-divider { border-color: var(--color-border); }
@media (min-width: 960px) {
    .admin-grid--forms { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
```

- [ ] **Step 4: Theme static administration sections and controls**

Assign:

```html
<header class="app-header ...">
<section id="loginPanel" class="admin-section panel panel--cut text-center">
<div class="admin-toolbar panel panel--cut ...">
<section class="admin-section panel panel--cut ...">
<section id="contestantSection" class="admin-section panel panel--cut ...">
```

Use `display-heading`, `muted`, `app-link`, `field`, and `button` variants for
headings, descriptive text, links, inputs, selects, textareas, and controls.
Map create/save/resume to `button--positive`, primary actions to
`button--primary`, ordinary navigation/edit/refresh to `button--secondary`, and
archive/delete/stop to `button--danger`. Retain all IDs, `hidden` attributes,
labels, event listeners, and form structure.

- [ ] **Step 5: Theme dynamically rendered contestant cards**

In `renderContestants`, use:

```js
card.className = "admin-card panel panel--cut";
details.className = "admin-card__details";
name.className = "font-bold text-lg";
description.className = "muted text-sm";
result.className = "eyebrow mt-1";
actions.className = "admin-card__actions";
editButton.className = "button button--secondary";
deleteButton.className = "button button--danger";
```

Update `createMessage` and runtime `className` assignments to use
`status-text`, `notice--error`, and `notice--success` without changing control
flow or displayed Czech copy.

- [ ] **Step 6: Run administration and backend tests**

Run:

```bash
go test ./... -run 'TestAdministrationUsesDarkSemanticControls|TestAdmin'
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit administration styling**

```bash
git add ui_test.go static/theme.css static/admin.html
git commit -m "style: redesign administration interface"
```

### Task 5: Administrator Vote Matrix

**Files:**
- Modify: `ui_test.go`
- Modify: `static/theme.css`
- Modify: `static/votes.html`

**Interfaces:**
- Consumes: existing `data.options`, `data.voters`, `option.name`, `option.id`, `voter.nickname`, `voter.voterId`, and `voter.scores`.
- Produces: `matrix-panel`, `vote-matrix`, `matrix-heading`, `matrix-user`, `matrix-row`, `matrix-score`, `matrix-score--empty`, and `pagination`.

- [ ] **Step 1: Add a failing vote-matrix contract**

Append to `ui_test.go`:

```go
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
```

- [ ] **Step 2: Run the matrix test and verify it fails**

Run:

```bash
go test ./... -run TestVoteMatrixKeepsNamedStickyColumns
```

Expected: FAIL because the semantic matrix classes do not exist.

- [ ] **Step 3: Add matrix CSS**

Append to `static/theme.css`:

```css
.matrix-panel { max-height: 70vh; overflow: auto; }
.vote-matrix { border-collapse: separate; border-spacing: 0; min-width: 100%; }
.matrix-heading {
    background: #2b251f;
    border-bottom: 1px solid var(--color-copper);
    color: var(--color-text);
    min-width: 9rem;
    padding: .8rem;
    position: sticky;
    text-align: center;
    top: 0;
    vertical-align: bottom;
    z-index: 20;
}
.matrix-user {
    left: 0;
    min-width: 13rem;
    position: sticky;
    text-align: left;
}
.matrix-heading.matrix-user { border-right: 1px solid var(--color-border); z-index: 30; }
.matrix-row { background: var(--color-surface); }
.matrix-row:nth-child(even) { background: #2a251f; }
.matrix-row:hover { background: #352d24; }
.matrix-user--body {
    background: inherit;
    border-right: 1px solid var(--color-border);
    padding: .75rem 1rem;
    z-index: 10;
}
.matrix-score {
    border-bottom: 1px solid rgb(81 70 58 / 55%);
    color: var(--color-amber);
    font-variant-numeric: tabular-nums;
    font-weight: 700;
    min-width: 9rem;
    padding: .75rem;
    text-align: center;
}
.matrix-score--empty { color: #74695d; font-weight: 400; }
.pagination { align-items: center; display: flex; gap: 1rem; justify-content: center; margin-top: 1rem; }
```

- [ ] **Step 4: Theme static matrix markup**

Use:

```html
<div id="tablePanel" hidden class="matrix-panel panel panel--cut">
    <table class="vote-matrix">
        <thead id="votesHead"></thead>
        <tbody id="votesBody"></tbody>
    </table>
</div>
<nav id="pagination" hidden class="pagination" aria-label="Stránkování uživatelů">
```

Theme the header with shared heading/link classes, refresh and pagination
buttons with `button button--secondary`, and messages with `notice` variants.

- [ ] **Step 5: Theme dynamically generated matrix cells**

Use these exact assignments:

```js
userHeading.className = "matrix-heading matrix-user";
heading.className = "matrix-heading";
row.className = "matrix-row";
userCell.className = "matrix-user matrix-user--body";
identifier.className = "ml-2 font-mono font-normal muted";
scoreCell.className = "matrix-score";
```

When a score is absent, add:

```js
scoreCell.classList.add("matrix-score--empty");
```

Continue assigning `heading.textContent` and `heading.title` from
`option.name || "Položka bez názvu"` so human-readable option identification is
preserved.

- [ ] **Step 6: Run matrix and full Go tests**

Run:

```bash
go test ./... -run 'TestVoteMatrixKeepsNamedStickyColumns|TestApplicationPagesLoadSharedTheme'
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit matrix styling**

```bash
git add ui_test.go static/theme.css static/votes.html
git commit -m "style: redesign administrator vote matrix"
```

### Task 6: Responsive and Regression Validation

**Files:**
- Modify: `ui_test.go`
- Modify: `static/theme.css` only if validation finds a concrete responsive or contrast defect.
- Modify: affected `static/*.html` only if validation finds a concrete semantic or layout defect.

**Interfaces:**
- Consumes: all themed pages and existing Go/Node test suites.
- Produces: verified keyboard, mobile, desktop, routing, and application behavior.

- [ ] **Step 1: Add a final CSS behavior contract**

Append to `ui_test.go`:

```go
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
```

- [ ] **Step 2: Run format, static, race, and function tests**

Run:

```bash
gofmt -w ui_test.go
git diff --check
go vet ./...
go test -race ./...
npm --prefix functions test
```

Expected: no formatting errors; `go vet` exits 0; all Go and Node tests PASS.

- [ ] **Step 3: Start the application for browser validation**

Run:

```bash
PORT=8080 go run .
```

Expected: the server warns that `GOOGLE_CLOUD_PROJECT` is unset, then logs
`Server běží na portu 8080...`. This mode is sufficient for static route and
layout checks and does not require production credentials.

- [ ] **Step 4: Verify routes and stylesheet delivery**

In another shell run:

```bash
curl -I http://localhost:8080/static/theme.css
curl -I http://localhost:8080/
curl -I http://localhost:8080/admin
curl -I http://localhost:8080/event/test/test/profile
curl -I http://localhost:8080/admin/event/test/results
curl -I http://localhost:8080/admin/event/test/votes
```

Expected: HTTP 200 for the stylesheet and all HTML routes. API-backed content may
show an in-page loading/error state for the synthetic event ID, which is
acceptable for this routing check.

- [ ] **Step 5: Perform narrow and wide browser checks**

At approximately 375×812 and 1440×900 verify:

- all six application pages use the charcoal/copper theme;
- every ballot option name and description is left-aligned;
- enabled public results show average and vote count at the upper-right;
- disabled public results leave no empty right column;
- `Nehodnoceno` and 0–10 remain on one scrollable line;
- selected votes have olive fill and a strong border;
- landing does not show the removed generic competition title;
- profile save and automatic return still work;
- results remain sorted and right-align average/count;
- admin actions, forms, stopping/resuming, refresh, rebuild, and links remain available;
- vote matrix headers show option names and remain sticky;
- keyboard Tab focus is visible; and
- reduced-motion emulation removes meaningful transitions.

- [ ] **Step 6: Run the complete verification again after any visual corrections**

Run:

```bash
git diff --check
go vet ./...
go test -race ./...
npm --prefix functions test
```

Expected: all commands PASS.

- [ ] **Step 7: Commit validated corrections**

If Step 5 required changes:

```bash
git add ui_test.go static/theme.css static/*.html
git commit -m "fix: polish responsive dark taproom UI"
```

If no changes were required, do not create an empty commit.
