"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  cleanupDryRun,
  chunks,
  eventAllowsBallotCleanup,
  isAnonymousUser,
  isExpiredAnonymousUser,
  lastActivityMillis,
  retentionDays,
  selectEligibleUsers,
} = require("./cleanup");

function user(overrides = {}) {
  return {
    providerData: [],
    metadata: {
      creationTime: "2026-01-01T00:00:00.000Z",
      lastSignInTime: "2026-01-01T00:00:00.000Z",
      lastRefreshTime: null,
    },
    ...overrides,
  };
}

test("recognizes only unlinked anonymous accounts", () => {
  assert.equal(isAnonymousUser(user()), true);
  assert.equal(isAnonymousUser(user({email: "voter@example.com"})), false);
  assert.equal(isAnonymousUser(user({providerData: [{providerId: "google.com"}]})), false);
  assert.equal(isAnonymousUser(user({customClaims: {admin: true}})), false);
});

test("uses the latest activity timestamp", () => {
  const value = lastActivityMillis(user({
    metadata: {
      creationTime: "2026-01-01T00:00:00.000Z",
      lastSignInTime: "2026-01-02T00:00:00.000Z",
      lastRefreshTime: "2026-01-03T00:00:00.000Z",
    },
  }));
  assert.equal(value, Date.parse("2026-01-03T00:00:00.000Z"));
});

test("expires anonymous users only before the cutoff", () => {
  const cutoff = Date.parse("2026-02-01T00:00:00.000Z");
  assert.equal(isExpiredAnonymousUser(user(), cutoff), true);
  assert.equal(isExpiredAnonymousUser(user({
    metadata: {
      creationTime: "2026-02-02T00:00:00.000Z",
      lastSignInTime: "2026-02-02T00:00:00.000Z",
      lastRefreshTime: null,
    },
  }), cutoff), false);
});

test("cleans ballots only when aggregates exist and the event is archived", () => {
  assert.equal(eventAllowsBallotCleanup({resultsInitialized: true, archived: true}), true);
  assert.equal(eventAllowsBallotCleanup({
    resultsInitialized: true,
    resultsRebuilding: true,
    archived: true,
  }), false);
  assert.equal(eventAllowsBallotCleanup({resultsInitialized: true, votingStopped: true}), false);
  assert.equal(eventAllowsBallotCleanup({resultsInitialized: true}), false);
  assert.equal(eventAllowsBallotCleanup({archived: true}), false);
});

test("retention defaults to 30 days and validates overrides", () => {
  assert.equal(retentionDays(undefined), 30);
  assert.equal(retentionDays("90"), 90);
  assert.throws(() => retentionDays("0"));
  assert.throws(() => retentionDays("not-a-number"));
});

test("cleanup defaults to dry-run and requires an explicit boolean", () => {
  assert.equal(cleanupDryRun(undefined), true);
  assert.equal(cleanupDryRun("true"), true);
  assert.equal(cleanupDryRun("false"), false);
  assert.throws(() => cleanupDryRun("yes"));
});

test("chunks values for Firebase bulk deletion", () => {
  assert.deepEqual(chunks([1, 2, 3, 4, 5], 2), [[1, 2], [3, 4], [5]]);
});

test("blocks an account when any ballot belongs to an unsafe event", () => {
  const safeBallot = {parent: {parent: {path: "events/safe"}}};
  const activeBallot = {parent: {parent: {path: "events/active"}}};
  const ballots = new Map([
    ["safe-user", [safeBallot]],
    ["blocked-user", [safeBallot, activeBallot]],
  ]);

  const result = selectEligibleUsers(
    ["safe-user", "blocked-user", "no-ballot-user"],
    ballots,
    new Set(["events/safe"]),
  );

  assert.deepEqual(result.eligibleUIDs, ["safe-user", "no-ballot-user"]);
  assert.deepEqual(result.ballotRefs, [safeBallot]);
  assert.equal(result.blockedUsers, 1);
});
