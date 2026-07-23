"use strict";

const DEFAULT_RETENTION_DAYS = 30;

function cleanupDryRun(value = process.env.ANONYMOUS_CLEANUP_DRY_RUN) {
  if (value === undefined || value === "") return true;
  if (value === "true") return true;
  if (value === "false") return false;
  throw new Error("ANONYMOUS_CLEANUP_DRY_RUN must be true or false");
}

function retentionDays(value = process.env.ANONYMOUS_USER_RETENTION_DAYS) {
  if (value === undefined || value === "") return DEFAULT_RETENTION_DAYS;
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1 || parsed > 3650) {
    throw new Error("ANONYMOUS_USER_RETENTION_DAYS must be an integer from 1 to 3650");
  }
  return parsed;
}

function isAnonymousUser(user) {
  return user.providerData.length === 0 &&
    !user.email &&
    !user.phoneNumber &&
    !user.passwordHash &&
    (!user.customClaims || Object.keys(user.customClaims).length === 0);
}

function lastActivityMillis(user) {
  const timestamps = [
    user.metadata.lastRefreshTime,
    user.metadata.lastSignInTime,
    user.metadata.creationTime,
  ]
    .filter(Boolean)
    .map(value => Date.parse(value))
    .filter(Number.isFinite);
  return timestamps.length === 0 ? Number.POSITIVE_INFINITY : Math.max(...timestamps);
}

function isExpiredAnonymousUser(user, cutoffMillis) {
  return isAnonymousUser(user) && lastActivityMillis(user) < cutoffMillis;
}

function eventAllowsBallotCleanup(eventData) {
  return eventData?.resultsInitialized === true &&
    eventData?.resultsRebuilding !== true &&
    eventData.archived === true;
}

function chunks(values, size) {
  const result = [];
  for (let start = 0; start < values.length; start += size) {
    result.push(values.slice(start, start + size));
  }
  return result;
}

function selectEligibleUsers(candidateUIDs, ballotsByUID, cleanupEventPaths) {
  const eligibleUIDs = [];
  const ballotRefs = [];
  let blockedUsers = 0;

  for (const uid of candidateUIDs) {
    const userBallots = ballotsByUID.get(uid) || [];
    const allBallotsAreSafe = userBallots.every(
      ballotRef => cleanupEventPaths.has(ballotRef.parent.parent.path),
    );
    if (!allBallotsAreSafe) {
      blockedUsers++;
      continue;
    }
    eligibleUIDs.push(uid);
    ballotRefs.push(...userBallots);
  }
  return {eligibleUIDs, ballotRefs, blockedUsers};
}

module.exports = {
  DEFAULT_RETENTION_DAYS,
  cleanupDryRun,
  chunks,
  eventAllowsBallotCleanup,
  isAnonymousUser,
  isExpiredAnonymousUser,
  lastActivityMillis,
  retentionDays,
  selectEligibleUsers,
};
