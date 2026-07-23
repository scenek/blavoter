"use strict";

const {initializeApp} = require("firebase-admin/app");
const {getAuth} = require("firebase-admin/auth");
const {FieldValue, getFirestore} = require("firebase-admin/firestore");
const {logger} = require("firebase-functions");
const {onSchedule} = require("firebase-functions/v2/scheduler");
const {
  cleanupDryRun,
  chunks,
  eventAllowsBallotCleanup,
  isExpiredAnonymousUser,
  retentionDays,
  selectEligibleUsers,
} = require("./cleanup");

initializeApp();

const REGION = "europe-west1";
const SERVICE_ACCOUNT = "blavoter-cleanup@blavoter-5cfc7.iam.gserviceaccount.com";
const DAY_MILLIS = 24 * 60 * 60 * 1000;
const FIRESTORE_GET_ALL_BATCH_SIZE = 400;
const MAX_ATOMIC_CLEANUP_EVENTS = 450;
const BALLOT_INDEX_MARKER_PATH = "maintenance/anonymousBallotIndex";

async function findExpiredAnonymousUsers(auth, cutoffMillis) {
  const users = [];
  let pageToken;
  do {
    const page = await auth.listUsers(1000, pageToken);
    for (const user of page.users) {
      if (isExpiredAnonymousUser(user, cutoffMillis)) users.push(user.uid);
    }
    pageToken = page.pageToken;
  } while (pageToken);
  return users;
}

async function getAllInChunks(db, refs) {
  const snapshots = [];
  for (const batch of chunks(refs, FIRESTORE_GET_ALL_BATCH_SIZE)) {
    snapshots.push(...await db.getAll(...batch));
  }
  return snapshots;
}

function validEventID(value) {
  return typeof value === "string" &&
    value !== "" && value !== "." && value !== ".." && !value.includes("/");
}

async function findCandidateBallots(db, candidateUIDs) {
  const ballotsByUID = new Map();
  const eventRefs = new Map();
  const lookupEventsByUID = new Map();
  const incompleteUIDs = [];
  const indexRefs = candidateUIDs.map(uid =>
    db.collection("anonymousVoterBallots").doc(uid),
  );
  const indexSnapshots = await getAllInChunks(db, indexRefs);
  const indexMarker = await db.doc(BALLOT_INDEX_MARKER_PATH).get();
  const migrationComplete = indexMarker.exists &&
    indexMarker.data()?.complete === true;

  for (let index = 0; index < candidateUIDs.length; index++) {
    const uid = candidateUIDs[index];
    const snapshot = indexSnapshots[index];
    const data = snapshot?.exists ? snapshot.data() : {};
    if (migrationComplete || data.complete === true) {
      const eventIDs = Array.isArray(data.eventIds) ? data.eventIds : [];
      lookupEventsByUID.set(uid, [...new Set(eventIDs.filter(validEventID))]);
    } else {
      incompleteUIDs.push(uid);
    }
  }

  // Missing/incomplete indexes belong to ballots created before indexing was
  // introduced. They take the compatibility path once; blocked users receive
  // a complete index while their provisional cleanup tombstone is held.
  if (incompleteUIDs.length > 0) {
    const eventsSnapshot = await db.collection("events").get();
    const allEventIDs = eventsSnapshot.docs.map(doc => doc.id).filter(validEventID);
    for (const uid of incompleteUIDs) lookupEventsByUID.set(uid, allEventIDs);
  }

  const candidateBallotRefs = [];
  for (const [uid, eventIDs] of lookupEventsByUID) {
    for (const eventID of eventIDs) {
      candidateBallotRefs.push(
        db.collection("events").doc(eventID).collection("votes").doc(uid),
      );
    }
  }
  let ballotLookups = 0;
  for (const refs of chunks(candidateBallotRefs, FIRESTORE_GET_ALL_BATCH_SIZE)) {
    ballotLookups += refs.length;
    const snapshots = await db.getAll(...refs);
    for (const ballot of snapshots) {
      if (!ballot.exists) continue;
      const ballotRefs = ballotsByUID.get(ballot.id) || [];
      ballotRefs.push(ballot.ref);
      ballotsByUID.set(ballot.id, ballotRefs);
      const eventRef = ballot.ref.parent.parent;
      eventRefs.set(eventRef.path, eventRef);
    }
  }

  return {ballotsByUID, eventRefs, ballotLookups, incompleteUIDs};
}

async function migrateBallotIndexes(db) {
  const markerRef = db.doc(BALLOT_INDEX_MARKER_PATH);
  const marker = await markerRef.get();
  if (marker.exists && marker.data()?.complete === true) {
    return {alreadyComplete: true, indexedUsers: 0, indexedBallots: 0};
  }

  const eventsByUID = new Map();
  const snapshot = await db.collectionGroup("votes").get();
  for (const ballot of snapshot.docs) {
    const path = ballot.ref.path.split("/");
    if (path.length !== 4 || path[0] !== "events" || path[2] !== "votes") continue;
    const eventID = path[1];
    if (!validEventID(eventID)) continue;
    const eventIDs = eventsByUID.get(ballot.id) || new Set();
    eventIDs.add(eventID);
    eventsByUID.set(ballot.id, eventIDs);
  }

  const writer = db.bulkWriter();
  writer.onWriteError(error => error.failedAttempts < 3);
  for (const [uid, eventIDs] of eventsByUID) {
    writer.set(db.collection("anonymousVoterBallots").doc(uid), {
      eventIds: FieldValue.arrayUnion(...eventIDs),
    }, {merge: true});
  }
  await writer.close();
  await markerRef.set({
    complete: true,
    completedAt: FieldValue.serverTimestamp(),
    indexedUsers: eventsByUID.size,
    indexedBallots: snapshot.size,
  });
  return {
    alreadyComplete: false,
    indexedUsers: eventsByUID.size,
    indexedBallots: snapshot.size,
  };
}

async function loadCleanupEventPaths(db, eventRefs) {
  if (eventRefs.size === 0) return new Set();
  const snapshots = await getAllInChunks(db, [...eventRefs.values()]);
  return new Set(
    snapshots
      .filter(snapshot => snapshot.exists && eventAllowsBallotCleanup(snapshot.data()))
      .map(snapshot => snapshot.ref.path),
  );
}

async function deleteDocuments(db, refs) {
  if (refs.length === 0) return;
  const writer = db.bulkWriter();
  writer.onWriteError(error => error.failedAttempts < 3);
  for (const ref of refs) writer.delete(ref);
  await writer.close();
}

async function writeCleanupTombstones(db, uids) {
  if (uids.length === 0) return;
  const refs = uids.map(uid => db.collection("cleanedAnonymousUsers").doc(uid));
  const snapshots = await getAllInChunks(db, refs);
  const writer = db.bulkWriter();
  writer.onWriteError(error => error.failedAttempts < 3);
  for (let index = 0; index < refs.length; index++) {
    const existing = snapshots[index];
    // A prior run may have deleted the ballots but failed to delete the Auth
    // account. Its permanent tombstone must never be downgraded to a claim.
    if (existing.exists && existing.data()?.provisional !== true) continue;
    writer.set(refs[index], {
      claimedAt: new Date(),
      provisional: true,
    }, {merge: true});
  }
  await writer.close();
}

async function releaseCleanupClaims(db, uids) {
  if (uids.length === 0) return;
  const refs = uids.map(uid => db.collection("cleanedAnonymousUsers").doc(uid));
  const snapshots = await getAllInChunks(db, refs);
  await deleteDocuments(db, snapshots
    .filter(snapshot => snapshot.exists && snapshot.data()?.provisional === true)
    .map(snapshot => snapshot.ref));
}

async function finalizeCleanupTombstones(db, uids) {
  if (uids.length === 0) return;
  const writer = db.bulkWriter();
  writer.onWriteError(error => error.failedAttempts < 3);
  for (const uid of uids) {
    writer.set(db.collection("cleanedAnonymousUsers").doc(uid), {
      cleanedAt: new Date(),
      provisional: false,
    }, {merge: true});
  }
  await writer.close();
}

async function writeCompleteBallotIndexes(db, uids, ballotsByUID) {
  if (uids.length === 0) return;
  const writer = db.bulkWriter();
  writer.onWriteError(error => error.failedAttempts < 3);
  for (const uid of uids) {
    const eventIds = [...new Set(
      (ballotsByUID.get(uid) || []).map(ref => ref.parent.parent.id),
    )];
    writer.set(db.collection("anonymousVoterBallots").doc(uid), {
      complete: true,
      eventIds,
    });
  }
  await writer.close();
}

function eventPathsForUser(uid, ballotsByUID) {
  return new Set(
    (ballotsByUID.get(uid) || []).map(ref => ref.parent.parent.path),
  );
}

function partitionUsersByEventLimit(uids, ballotsByUID, limit = MAX_ATOMIC_CLEANUP_EVENTS) {
  const groups = [];
  const oversizedUIDs = [];
  let currentUIDs = [];
  let currentEvents = new Set();

  for (const uid of uids) {
    const userEvents = eventPathsForUser(uid, ballotsByUID);
    if (userEvents.size > limit) {
      oversizedUIDs.push(uid);
      continue;
    }
    const combined = new Set([...currentEvents, ...userEvents]);
    if (currentUIDs.length > 0 && combined.size > limit) {
      groups.push(currentUIDs);
      currentUIDs = [];
      currentEvents = new Set();
    }
    currentUIDs.push(uid);
    for (const eventPath of userEvents) currentEvents.add(eventPath);
  }
  if (currentUIDs.length > 0) groups.push(currentUIDs);
  return {groups, oversizedUIDs};
}

async function deleteAuthUsers(auth, uids) {
  let deletedUsers = 0;
  for (const batch of chunks(uids, 1000)) {
    const result = await auth.deleteUsers(batch);
    deletedUsers += result.successCount;
    if (result.failureCount > 0) {
      const failures = result.errors.map(error => ({
        uid: batch[error.index],
        code: error.error.code,
      }));
      logger.error("Some anonymous Auth accounts could not be deleted", {failures});
      throw new Error(`Failed to delete ${result.failureCount} anonymous Auth accounts`);
    }
  }
  return deletedUsers;
}

async function lockCleanupEvents(db, eventRefs, eventPaths) {
  if (eventPaths.size > MAX_ATOMIC_CLEANUP_EVENTS) {
    throw new Error(
      `Cleanup touches ${eventPaths.size} events; maximum atomic scope is ${MAX_ATOMIC_CLEANUP_EVENTS}`,
    );
  }
  const refs = [];
  for (const eventPath of eventPaths) {
    const eventRef = eventRefs.get(eventPath);
    if (!eventRef) throw new Error(`Missing event reference for ${eventPath}`);
    refs.push(eventRef);
  }
  if (refs.length === 0) return;
  await db.runTransaction(async transaction => {
    const snapshots = await transaction.getAll(...refs);
    for (let index = 0; index < refs.length; index++) {
      const snapshot = snapshots[index];
      if (!snapshot.exists || !eventAllowsBallotCleanup(snapshot.data())) {
        throw new Error(`Event is no longer eligible for ballot cleanup: ${refs[index].path}`);
      }
    }
    for (const eventRef of refs) {
      transaction.set(eventRef, {ballotsCleaned: true}, {merge: true});
    }
  });
}

async function cleanupClaimedGroup(db, auth, claimedUIDs) {
  const refreshed = await findCandidateBallots(db, claimedUIDs);
  const cleanupEventPaths = await loadCleanupEventPaths(db, refreshed.eventRefs);
  const selection = selectEligibleUsers(
    claimedUIDs,
    refreshed.ballotsByUID,
    cleanupEventPaths,
  );
  const eligibleSet = new Set(selection.eligibleUIDs);
  const blockedUIDs = claimedUIDs.filter(uid => !eligibleSet.has(uid));

  // The provisional tombstones make this index snapshot complete: voter
  // transactions cannot add another event until the tombstone is removed.
  await writeCompleteBallotIndexes(db, blockedUIDs, refreshed.ballotsByUID);
  await releaseCleanupClaims(db, blockedUIDs);

  const {groups, oversizedUIDs} = partitionUsersByEventLimit(
    selection.eligibleUIDs,
    refreshed.ballotsByUID,
  );
  await writeCompleteBallotIndexes(db, oversizedUIDs, refreshed.ballotsByUID);
  await releaseCleanupClaims(db, oversizedUIDs);

  let deletedUsers = 0;
  let deletedBallots = 0;
  let deferredUsers = blockedUIDs.length + oversizedUIDs.length;
  for (const uids of groups) {
    const ballotRefs = uids.flatMap(uid => refreshed.ballotsByUID.get(uid) || []);
    const eventPaths = new Set(
      ballotRefs.map(ballotRef => ballotRef.parent.parent.path),
    );
    try {
      await lockCleanupEvents(db, refreshed.eventRefs, eventPaths);
    } catch (error) {
      // No ballots have been deleted and lockCleanupEvents is atomic, so these
      // claims are safe to release and retry on the next scheduled run.
      await releaseCleanupClaims(db, uids);
      deferredUsers += uids.length;
      logger.error("Cleanup group could not lock its events", {
        error: error.message,
        users: uids.length,
        events: eventPaths.size,
      });
      continue;
    }

    // From this point onward cleanup may be partially complete. Keep a
    // permanent tombstone so cached tokens cannot recreate deleted data, and
    // let a later invocation retry any failed deletions.
    await finalizeCleanupTombstones(db, uids);
    await deleteDocuments(db, ballotRefs);
    await deleteDocuments(
      db,
      uids.map(uid => db.collection("anonymousVoterBallots").doc(uid)),
    );
    deletedUsers += await deleteAuthUsers(auth, uids);
    deletedBallots += ballotRefs.length;
  }

  return {
    deletedUsers,
    deletedBallots,
    deferredUsers,
    ballotLookups: refreshed.ballotLookups,
  };
}

async function cleanupEligibleUsers(db, auth, eligibleUIDs, ballotsByUID) {
  const initial = partitionUsersByEventLimit(eligibleUIDs, ballotsByUID);
  let deletedUsers = 0;
  let deletedBallots = 0;
  let deferredUsers = initial.oversizedUIDs.length;
  let ballotLookups = 0;

  // A single user spanning more than one transaction's event limit is left
  // untouched, but does not prevent unrelated users from being cleaned.
  for (const uids of initial.groups) {
    await writeCleanupTombstones(db, uids);
    const result = await cleanupClaimedGroup(db, auth, uids);
    deletedUsers += result.deletedUsers;
    deletedBallots += result.deletedBallots;
    deferredUsers += result.deferredUsers;
    ballotLookups += result.ballotLookups;
  }
  return {deletedUsers, deletedBallots, deferredUsers, ballotLookups};
}

exports.migrateAnonymousBallotIndexes = onSchedule(
  {
    schedule: "every day 02:30",
    timeZone: "Europe/Prague",
    region: REGION,
    serviceAccount: SERVICE_ACCOUNT,
    timeoutSeconds: 540,
    memory: "512MiB",
    maxInstances: 1,
    retryCount: 3,
  },
  async () => {
    const result = await migrateBallotIndexes(getFirestore());
    logger.info("Anonymous ballot index migration finished", result);
  },
);

exports.cleanupAnonymousUsers = onSchedule(
  {
    schedule: "every day 03:00",
    timeZone: "Europe/Prague",
    region: REGION,
    serviceAccount: SERVICE_ACCOUNT,
    timeoutSeconds: 540,
    memory: "512MiB",
    maxInstances: 1,
    retryCount: 3,
  },
  async () => {
    const days = retentionDays();
    const dryRun = cleanupDryRun();
    const cutoffMillis = Date.now() - days * DAY_MILLIS;
    const auth = getAuth();
    const db = getFirestore();

    const candidateUIDs = await findExpiredAnonymousUsers(auth, cutoffMillis);
    if (candidateUIDs.length === 0) {
      logger.info("Anonymous user cleanup finished; no expired accounts found", {
        dryRun,
        retentionDays: days,
      });
      return;
    }

    // maxInstances=1 prevents this from racing a live cleanup invocation.
    // Release claims left by a crashed invocation before recalculating safety.
    if (!dryRun) await releaseCleanupClaims(db, candidateUIDs);

    const {ballotsByUID, eventRefs, ballotLookups} =
      await findCandidateBallots(db, candidateUIDs);
    const cleanupEventPaths = await loadCleanupEventPaths(db, eventRefs);
    const {eligibleUIDs, ballotRefs, blockedUsers} = selectEligibleUsers(
      candidateUIDs,
      ballotsByUID,
      cleanupEventPaths,
    );

    if (dryRun) {
      logger.info("Anonymous user cleanup dry run finished; nothing was deleted", {
        retentionDays: days,
        expiredUsers: candidateUIDs.length,
        eligibleUsers: eligibleUIDs.length,
        eligibleBallots: ballotRefs.length,
        ballotLookups,
        blockedUsers,
      });
      return;
    }

    // Preliminary eligibility avoids disrupting expired users that still have
    // active ballots. Each eligible group is then claimed with tombstones and
    // rescanned, closing the discovery-to-deletion race with cached ID tokens.
    const cleanup = await cleanupEligibleUsers(
      db,
      auth,
      eligibleUIDs,
      ballotsByUID,
    );

    logger.info("Anonymous user cleanup finished", {
      dryRun,
      retentionDays: days,
      expiredUsers: candidateUIDs.length,
      deletedUsers: cleanup.deletedUsers,
      deletedBallots: cleanup.deletedBallots,
      ballotLookups: ballotLookups + cleanup.ballotLookups,
      blockedUsers: blockedUsers + cleanup.deferredUsers,
    });
  },
);

module.exports._test = {
  findCandidateBallots,
  findExpiredAnonymousUsers,
  loadCleanupEventPaths,
  lockCleanupEvents,
  cleanupClaimedGroup,
  cleanupEligibleUsers,
  finalizeCleanupTombstones,
  migrateBallotIndexes,
  partitionUsersByEventLimit,
  releaseCleanupClaims,
  selectEligibleUsers,
  writeCompleteBallotIndexes,
  writeCleanupTombstones,
};
