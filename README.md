# Blavoter

Blavoter is a Czech event-scoped voting application. Administrators create
events and voting options, distribute an unlisted invitation URL, control
whether voting and public results are enabled, and inspect aggregate or
individual results. Voters authenticate anonymously and maintain one ballot
per event.

## Requirements

- Go 1.26 or newer
- A Google Cloud project with Firestore in Native mode
- A Firebase project connected to the same Google Cloud project
- Firebase Authentication with Anonymous and Google sign-in enabled
- Google Cloud CLI and Firebase CLI for deployment
- Docker for local container validation

## Firebase and Google authentication setup

1. In the Firebase console, open **Authentication → Sign-in method** and enable
   the **Anonymous** and **Google** providers.
2. Register a Firebase web app for the project and copy its web configuration.
3. Open **Authentication → Settings → Authorized domains** and add every host
   that serves the application. Keep `localhost` for local development and add
   the hostname of the deployed Cloud Run service.
4. Create a Firestore database in Native mode.
5. Configure these application environment variables:

```text
GOOGLE_CLOUD_PROJECT=your-project-id
FIREBASE_API_KEY=your-web-api-key
FIREBASE_AUTH_DOMAIN=your-project-id.firebaseapp.com
FIREBASE_APP_ID=your-web-app-id
ADMIN_EMAILS=admin@example.com,second-admin@example.com
```

`FIREBASE_AUTH_DOMAIN` defaults to
`<GOOGLE_CLOUD_PROJECT>.firebaseapp.com`. `FIREBASE_APP_ID` is optional for the
authentication flow used by this app. Firebase web configuration, including
the API key, identifies the Firebase project but is not an administrator
credential. Do not commit service-account keys, Application Default
Credentials, or populated `.env` files.

The Google account used for administration must be present in `ADMIN_EMAILS`.
Administration is available at `/admin`. A Firebase token is accepted there
only when it was issued through Google, its email is verified, and the
normalized email is allowlisted. Administration also checks whether the token
or account has been revoked. An empty allowlist denies everyone.

## Local development

Authenticate the server to Google Cloud, export the configuration, and start
the Go application:

```sh
gcloud auth application-default login
gcloud config set project your-project-id

export GOOGLE_CLOUD_PROJECT=your-project-id
export FIREBASE_API_KEY=your-web-api-key
export FIREBASE_AUTH_DOMAIN=your-project-id.firebaseapp.com
export FIREBASE_APP_ID=your-web-app-id
export ADMIN_EMAILS=admin@example.com

go run .
```

Open:

- voter landing page: `http://localhost:8080/`
- administration: `http://localhost:8080/admin`

The root page intentionally does not list events. Create or select an event in
administration and use its generated invitation URL.

To validate the production container locally:

```sh
docker build -t blavoter .
docker run --rm -p 8080:8080 \
  -e GOOGLE_CLOUD_PROJECT \
  -e FIREBASE_API_KEY \
  -e FIREBASE_AUTH_DOMAIN \
  -e FIREBASE_APP_ID \
  -e ADMIN_EMAILS \
  -e GOOGLE_APPLICATION_CREDENTIALS=/tmp/gcloud-adc.json \
  -v "$HOME/.config/gcloud/application_default_credentials.json:/tmp/gcloud-adc.json:ro" \
  blavoter
```

The credentials mount is for local testing only. Cloud Run uses its assigned
service account and does not need a credentials file.

## Authentication and ballot ownership

The browser signs in anonymously and sends its Firebase ID token to the API.
The server verifies that token and derives the ballot document ID from its UID;
the client never chooses the owner ID. Ballots are stored at
`events/{eventId}/votes/{firebaseUID}`, preventing one voter from overwriting
another voter's ballot.

For each selected event, administration links to a protected, paginated
voter-detail page at `/admin/event/{eventId}/votes`. It shows stored nicknames,
short voter identifiers, and the option-by-option contents of each ballot. The
page is available only before anonymous-user cleanup; once
`ballotsCleaned=true`, the API returns `410 Gone` because individual results
are no longer complete.

## Multiple events

Administrators create events first and then manage the contestant pool of the
selected event. The administration page generates a unique invitation URL from
the event ID and name:

```text
/event/{eventId}/{event-name}
```

Voters can only enter through that URL; the application does not expose an
event picker or a public event list. Each anonymous Firebase user gets one
independent ballot and nickname per event.

Each event has its own administrator-defined voting options (stored internally
as contestants). Every option accepts an integer score from `0` through `10`.
A voter may leave any option blank; blank options are omitted from the ballot
and do not affect that option's average. An entirely empty ballot is also valid.

The voter page sorts options by their Czech display name. Each row offers
`Nehodnoceno` and the values `0` through `10`. Selecting a value saves the
updated ballot automatically; rapid selections are serialized so an older
request cannot overwrite a newer choice. The server also limits sustained vote
updates per Firebase UID and event. An event can contain at most 100 voting
options.

Nicknames are managed on the event-specific profile page:

```text
/event/{eventId}/{event-name}/profile
```

The voting page links to this profile from its header and contains no editable
nickname field. Profile updates and score updates use separate API operations.

Each event also has a `showResults` setting controlled from administration.
When enabled, voters see the vote count and mean score for each option. When
disabled, the public API does not aggregate or expose those values; the
authenticated administration API continues to show full results.

Every event has a dedicated ranked results page. Administrators can open its
protected version from the event controls. When `showResults` is enabled, the
voting page also links to the public URL:

```text
/event/{eventId}/{event-name}/results
```

The public results endpoint returns `403 Forbidden` while public results are
disabled. Options with votes are ranked by average score descending, then vote
count descending, and finally Czech name; unrated options are shown last.

Administrators can also stop and resume voting independently for each event.
Stopping voting keeps the event and existing ballots visible, but the server
rejects all ballot changes until voting is resumed.

Initialized events maintain transactional aggregate counters for every voting
option. A ballot change adjusts only the affected totals and vote counts, so
displaying live results does not scan every ballot. Events created before this
feature continue using the legacy calculation until an administrator stops
voting and selects `Přepočítat výsledky`; voting can then be resumed. If a
rebuild fails, the event remains marked as rebuilding so cleanup cannot trust
partial aggregates. Run the rebuild again; a successful retry clears the
marker. Result endpoints temporarily return `503 Service Unavailable` instead
of scanning every ballot while that marker is present.

Firestore data is organized as:

```text
events/{eventId}
  contestants/{contestantId}
  votes/{firebaseUID}
  results/{contestantId}
anonymousVoterBallots/{firebaseUID}
cleanedAnonymousUsers/{firebaseUID}
```

Archiving an event stops public voting but preserves its contestants and
ballots. Restoring it makes the event public again.

Legacy top-level `contestants` and `votes` collections from the earlier
single-event version are ignored and are not migrated automatically. Move any
data that must be retained into an event's subcollections before reopening the
application.

## Anonymous user cleanup

The `functions` directory contains two second-generation scheduled functions.
`migrateAnonymousBallotIndexes` runs at 02:30 and builds the ballot index once;
later executions exit after reading its completion marker.
`cleanupAnonymousUsers` runs daily at 03:00 in the `Europe/Prague` time zone
and deletes anonymous Firebase Auth accounts with no activity for 30 days.
It also deletes their `events/{eventId}/votes/{uid}` documents, including the
stored nickname, while leaving `events/{eventId}/results/{contestantId}`
aggregates unchanged. New ballot writes maintain
`anonymousVoterBallots/{uid}` so cleanup reads only the events associated with
an expired voter. A compatibility lookup discovers pre-index ballots once;
blocked legacy users receive a complete index for later runs.

For safety, an account with ballots is cleaned only when every associated event
has initialized aggregate results and is archived. Before deleting ballots, the
function marks each affected event with `ballotsCleaned=true`. That marker is
irreversible: the application will not restore the event or rebuild its
aggregates from the incomplete ballot collection. Accounts associated with
active, merely stopped, or legacy non-aggregated events are reported as blocked
and retained for a later run.

Before removing a ballot or Auth account, the function also writes
`cleanedAnonymousUsers/{uid}`. Profile and vote transactions check this
tombstone, so an already-issued Firebase token cannot recreate data after its
anonymous account has been selected for cleanup. Cleanup initially uses the
tombstone as a provisional claim and rescans that user's ballots before
deletion; claims for users with active or otherwise unsafe ballots are removed.
Stale provisional claims from an interrupted run are released on the next run,
and each claim becomes permanent before ballot deletion starts.
Accepted vote changes also store token-bucket state on the ballot; this
enforces the write-rate limit consistently across Cloud Run instances.

Do not enable Firebase Authentication's built-in anonymous-account cleanup at
the same time. It can remove the Auth record before this function has identified
the UID and deleted its Firestore ballots.

The function defaults to dry-run mode, so its first deployment logs eligible
users and ballots without deleting anything. Copy `functions/.env.example` to
`functions/.env`, deploy with `ANONYMOUS_CLEANUP_DRY_RUN=true`, and inspect the
function logs. Change it to `false` and redeploy only when the reported scope is
correct. The same file controls `ANONYMOUS_USER_RETENTION_DAYS`.

The function uses a dedicated service account so the public Cloud Run service
does not receive Firebase Authentication administration permissions. Create it
and grant it Firestore and Firebase Authentication access before the first
deployment:

```sh
gcloud iam service-accounts create blavoter-cleanup \
  --project=blavoter-5cfc7 \
  --display-name="Blavoter anonymous user cleanup"

gcloud projects add-iam-policy-binding blavoter-5cfc7 \
  --member=serviceAccount:blavoter-cleanup@blavoter-5cfc7.iam.gserviceaccount.com \
  --role=roles/datastore.user

gcloud projects add-iam-policy-binding blavoter-5cfc7 \
  --member=serviceAccount:blavoter-cleanup@blavoter-5cfc7.iam.gserviceaccount.com \
  --role=roles/firebaseauth.admin
```

If `blavoter-run` was previously granted `roles/firebaseauth.admin` only for
this cleanup function, remove that binding after deploying with the dedicated
account:

```sh
gcloud projects remove-iam-policy-binding blavoter-5cfc7 \
  --member=serviceAccount:blavoter-run@blavoter-5cfc7.iam.gserviceaccount.com \
  --role=roles/firebaseauth.admin
```

Cloud Scheduler-backed functions require the Blaze plan. Deploy from the
repository root. Deploy the Cloud Run voter revision first so every new ballot
maintains its index, then deploy the functions:

```sh
firebase use your-project-id
firebase deploy --only functions:anonymous-user-cleanup
```

The selector is the `codebase` configured in `firebase.json`. To deploy the
two functions explicitly by name instead, use:

```sh
firebase deploy --only \
  functions:migrateAnonymousBallotIndexes,functions:cleanupAnonymousUsers
```

## Deploying the voter application to Cloud Run

Create a dedicated runtime service account once and grant it Firestore access:

```sh
export PROJECT_ID=your-project-id
export REGION=europe-west1

gcloud config set project "$PROJECT_ID"
gcloud services enable run.googleapis.com cloudbuild.googleapis.com \
  artifactregistry.googleapis.com firestore.googleapis.com

gcloud iam service-accounts create blavoter-run \
  --display-name="Blavoter Cloud Run"

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:blavoter-run@$PROJECT_ID.iam.gserviceaccount.com" \
  --role=roles/datastore.user

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:blavoter-run@$PROJECT_ID.iam.gserviceaccount.com" \
  --role=roles/firebaseauth.viewer
```

The Firebase Authentication Viewer role is required because administration
checks whether a Google session or account has been revoked. The public voter
API still performs local token verification and does not make an Auth user
lookup for every vote.

Create an untracked `.env.cloudrun.yaml`:

```yaml
GOOGLE_CLOUD_PROJECT: "your-project-id"
FIREBASE_API_KEY: "your-web-api-key"
FIREBASE_AUTH_DOMAIN: "your-project-id.firebaseapp.com"
FIREBASE_APP_ID: "your-web-app-id"
ADMIN_EMAILS: "admin@example.com,second-admin@example.com"
```

Deploy the first version or any later revision from the repository root:

```sh
gcloud run deploy blavoter \
  --source . \
  --region "$REGION" \
  --allow-unauthenticated \
  --service-account "blavoter-run@$PROJECT_ID.iam.gserviceaccount.com" \
  --env-vars-file .env.cloudrun.yaml
```

Because the repository contains a `Dockerfile`, source deployment builds that
container and creates a new Cloud Run revision. After the first deployment, add
the generated `*.run.app` hostname to Firebase Authentication's authorized
domains. Re-run the same command whenever application code or static pages
change. Deploy the scheduled cleanup separately with the Firebase CLI.

## Tests

Run the server checks:

```sh
go test -race ./...
go vet ./...
```

Run the scheduled-function checks:

```sh
cd functions
npm install
npm test
npm run check
```
