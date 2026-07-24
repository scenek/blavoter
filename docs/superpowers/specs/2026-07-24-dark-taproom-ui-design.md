# Dark Taproom UI Design

## Goal

Refresh every Blavoter page with a consistent festival-oriented visual system. The
information hierarchy should take inspiration from the referenced voting site,
especially its compact ballot rows, while giving Blavoter a distinct identity
through darker colors, angular geometry, and different typography.

The redesign must not change authentication, voting, administration, routing, or
data behavior.

## Scope

The visual system applies to:

- the no-event landing page;
- the event ballot;
- the voter profile;
- public and administrator result views;
- the administration interface; and
- the administrator vote matrix.

The poster page in `docs/` remains a separate promotional page and is not part of
the application redesign.

## Visual Direction

The theme is “dark taproom”:

- a near-black charcoal page background;
- dark brown and graphite surfaces;
- warm off-white primary text;
- copper and amber as the main interactive accents;
- muted olive for selected and positive states;
- brick red for destructive and error states; and
- subdued taupe text for descriptions and metadata.

Cards and controls use restrained corner cuts or asymmetric corner radii instead
of the current uniformly rounded white cards. Thin warm borders and subtle inset
highlights separate surfaces without relying on large shadows.

Headings use a sturdy display face suitable for event signage. Body text and
controls use a neutral sans-serif optimized for legibility. Fonts must have
system fallbacks so the UI remains usable if the remote font cannot load.

## Shared Design Tokens

The implementation will define shared CSS custom properties in one application
stylesheet. Approximate tokens:

| Role | Value |
| --- | --- |
| Page background | `#171512` |
| Raised surface | `#24201b` |
| Secondary surface | `#302a23` |
| Primary text | `#f4eadb` |
| Muted text | `#b8aa98` |
| Copper accent | `#d68032` |
| Bright amber | `#f0ad45` |
| Olive state | `#87945a` |
| Error/destructive | `#b9513f` |
| Warm border | `#51463a` |

Exact contrast values may be adjusted during implementation to meet accessible
contrast requirements.

## Shared Components

All pages use a centered application shell with consistent navigation, headings,
spacing, cards, buttons, fields, notices, and loading states.

Primary buttons use a solid copper surface. Secondary buttons use a dark surface
with a warm border. Destructive controls use brick red. Links use bright amber
and retain a visible focus treatment.

Inputs, text areas, and selects use dark inset surfaces with light text, visible
labels, and clear focus rings. Disabled controls must remain visibly distinct.

Status messages use bordered dark notices with color-coded accent edges rather
than pale background banners.

## Event Ballot

The event heading, description, navigation, and voting status form a compact
top section. Each voting option is rendered as one angular ballot card.

The card header uses two columns:

- the flexible left column contains the option name and description;
- the fixed right column contains the aggregate result when public results are
  enabled.

The result block shows the average as the strongest value and the vote count as
secondary text. It is right-aligned and visually separated from the description
with a subtle vertical border. If no votes exist, it displays “Nehodnoceno”
without implying that an unselected value is a vote. If public results are
disabled, the right column is absent and the option details use the full width.

The `Nehodnoceno` choice and values 0–10 remain in one horizontal row below the
header. The selected value uses the olive state with a strong outline; other
values use compact dark buttons. The row may scroll horizontally on narrow
screens, with a visible but unobtrusive scrollbar or overflow cue. Automatic
saving and all existing feedback behavior remain unchanged.

## Results Page

Results remain sorted by the application’s existing ranking logic. Each row has
a prominent rank marker, option name and description, then a right-aligned
average and vote count matching the ballot result block. The leading entries may
receive progressively stronger copper accents without using conventional medal
clip art.

## Voter Profile

The profile uses a narrow, focused panel with the event context, nickname field,
and a single strong save action. Back navigation and success/error feedback use
the shared components. Redirect behavior is unchanged.

## Administration

Administration uses the same palette with a wider shell. Event selection and
high-frequency actions occupy a compact toolbar. Editing forms and contestant
management are grouped into clearly titled angular panels.

Actions have consistent semantic styling:

- copper for primary actions;
- olive for create/save/resume;
- neutral outlined controls for edit/refresh/navigation; and
- brick red for archive/delete/stop where destructive emphasis is appropriate.

Dense areas may use responsive grids on desktop and stack on smaller screens.
No administration feature is removed or hidden by the redesign.

## Vote Matrix

The vote matrix uses a dark, high-contrast table. Sticky headers and the user
identity column remain easy to distinguish while scrolling. Alternating subtle
row tones, warm grid lines, and centered score cells improve scanning. Option
names remain fully identifiable and are never replaced visually by document IDs.

## Landing Page

The no-event landing page becomes a simple dark branded panel explaining that a
valid event link is required, with a restrained administration link. It does not
restore the previously removed generic event title.

## Responsive and Accessibility Requirements

- The main voter experience is optimized first for common phone widths.
- Result values stay at the upper-right of ballot cards on phones.
- Voting values remain on one line and may scroll rather than wrap.
- Touch targets are at least approximately 40 pixels high.
- Keyboard focus is clearly visible on every interactive control.
- Selected voting values expose their state through both color and border/shape.
- Text and interactive controls meet WCAG AA contrast where practical.
- Existing live regions, labels, button semantics, and disabled states are
  preserved.
- Motion is minimal and respects reduced-motion preferences.

## Implementation Structure

Add one shared stylesheet under `static/` and load it from every application HTML
page. Existing Tailwind utilities may remain for layout during the transition,
but reusable visual rules and tokens belong in the shared stylesheet to prevent
page-to-page drift.

Dynamic elements created by JavaScript receive shared semantic classes rather
than long, duplicated Tailwind class strings wherever practical. IDs and DOM
relationships used by existing JavaScript remain stable.

## Validation

Implementation is complete when:

- every application route uses the dark taproom theme;
- public results appear on the right side of each ballot card when enabled;
- ballot cards use their full width when public results are disabled;
- the vote scale stays on one line at phone and desktop widths;
- all existing voter and administrator workflows still function;
- every page is usable by keyboard;
- no option name is lost in the vote matrix; and
- Go tests and relevant static/application checks pass.

Manual browser checks should cover the landing, ballot with and without public
results, profile, results, administration, and vote matrix at narrow and wide
viewport sizes.
