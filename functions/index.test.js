"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {_test} = require("./index");

test("writes cleanup tombstones for every eligible anonymous user", async () => {
  const writes = [];
  let errorHandler;
  let closed = false;
  const db = {
    collection(name) {
      assert.equal(name, "cleanedAnonymousUsers");
      return {
        doc(uid) {
          return {path: `${name}/${uid}`};
        },
      };
    },
    async getAll(...refs) {
      return refs.map(ref => ({exists: false, ref}));
    },
    bulkWriter() {
      return {
        onWriteError(handler) {
          errorHandler = handler;
        },
        set(ref, data) {
          writes.push({ref, data});
        },
        async close() {
          closed = true;
        },
      };
    },
  };

  await _test.writeCleanupTombstones(db, ["uid-1", "uid-2"]);

  assert.equal(typeof errorHandler, "function");
  assert.equal(closed, true);
  assert.deepEqual(writes.map(write => write.ref.path), [
    "cleanedAnonymousUsers/uid-1",
    "cleanedAnonymousUsers/uid-2",
  ]);
  for (const write of writes) {
    assert.equal(write.data.claimedAt instanceof Date, true);
    assert.equal(write.data.provisional, true);
  }
});

test("releases only provisional cleanup claims", async () => {
  const deleted = [];
  const db = {
    collection(name) {
      assert.equal(name, "cleanedAnonymousUsers");
      return {
        doc(uid) {
          return {id: uid, path: `${name}/${uid}`};
        },
      };
    },
    async getAll(...refs) {
      return refs.map(ref => ({
        exists: true,
        ref,
        data: () => ({provisional: ref.id === "claimed"}),
      }));
    },
    bulkWriter() {
      return {
        onWriteError() {},
        delete(ref) {
          deleted.push(ref.path);
        },
        async close() {},
      };
    },
  };

  await _test.releaseCleanupClaims(db, ["claimed", "final"]);

  assert.deepEqual(deleted, ["cleanedAnonymousUsers/claimed"]);
});

test("does not downgrade a permanent cleanup tombstone", async () => {
  const writes = [];
  const db = {
    collection(name) {
      return {doc: uid => ({id: uid, path: `${name}/${uid}`})};
    },
    async getAll(...refs) {
      return refs.map(ref => ({
        exists: true,
        ref,
        data: () => ({cleanedAt: new Date(), provisional: false}),
      }));
    },
    bulkWriter() {
      return {
        onWriteError() {},
        set(ref) {
          writes.push(ref.path);
        },
        async close() {},
      };
    },
  };

  await _test.writeCleanupTombstones(db, ["already-cleaned"]);

  assert.deepEqual(writes, []);
});

test("locks all cleanup events atomically", async () => {
  const eventA = {path: "events/a"};
  const eventB = {path: "events/b"};
  const writes = [];
  const db = {
    async runTransaction(callback) {
      await callback({
        async getAll(...refs) {
          assert.deepEqual(refs, [eventA, eventB]);
          return refs.map(ref => ({
            exists: true,
            data: () => ({resultsInitialized: true, archived: true}),
            ref,
          }));
        },
        set(ref, data, options) {
          writes.push({ref, data, options});
        },
      });
    },
  };

  await _test.lockCleanupEvents(
    db,
    new Map([[eventA.path, eventA], [eventB.path, eventB]]),
    new Set([eventA.path, eventB.path]),
  );

  assert.deepEqual(writes.map(write => write.ref.path), [eventA.path, eventB.path]);
});

test("does not partially lock cleanup events when one is ineligible", async () => {
  const eventA = {path: "events/a"};
  const eventB = {path: "events/b"};
  const writes = [];
  const db = {
    async runTransaction(callback) {
      await callback({
        async getAll() {
          return [
            {exists: true, data: () => ({resultsInitialized: true, archived: true})},
            {exists: true, data: () => ({resultsInitialized: true, archived: false})},
          ];
        },
        set(ref) {
          writes.push(ref);
        },
      });
    },
  };

  await assert.rejects(() => _test.lockCleanupEvents(
    db,
    new Map([[eventA.path, eventA], [eventB.path, eventB]]),
    new Set([eventA.path, eventB.path]),
  ));
  assert.deepEqual(writes, []);
});

test("looks up only expired users' ballots in each event", async () => {
  const existingBallots = new Set(["events/a/votes/uid-1"]);
  function eventRef(id) {
    const ref = {
      id,
      path: `events/${id}`,
      collection(name) {
        assert.equal(name, "votes");
        return {
          doc(uid) {
            return {
              id: uid,
              path: `${ref.path}/votes/${uid}`,
              parent: {parent: ref},
            };
          },
        };
      },
    };
    return ref;
  }
  const events = [eventRef("a"), eventRef("b")];
  const db = {
    doc(path) {
      assert.equal(path, "maintenance/anonymousBallotIndex");
      return {
        async get() {
          return {exists: false};
        },
      };
    },
    collection(name) {
      if (name === "anonymousVoterBallots") {
        return {
          doc(uid) {
            return {kind: "index", id: uid, path: `${name}/${uid}`};
          },
        };
      }
      assert.equal(name, "events");
      return {
        doc(id) {
          return events.find(ref => ref.id === id);
        },
        async get() {
          return {docs: events.map(ref => ({id: ref.id, ref}))};
        },
      };
    },
    async getAll(...refs) {
      return refs.map(ref => {
        if (ref.kind === "index") {
          return ref.id === "uid-1" ?
            {
              exists: true,
              data: () => ({complete: true, eventIds: ["a"]}),
            } :
            {exists: false};
        }
        return {
          id: ref.id,
          ref,
          exists: existingBallots.has(ref.path),
        };
      });
    },
  };

  const result = await _test.findCandidateBallots(db, ["uid-1", "uid-2"]);

  assert.equal(result.ballotLookups, 3);
  assert.deepEqual(result.incompleteUIDs, ["uid-2"]);
  assert.deepEqual(result.ballotsByUID.get("uid-1").map(ref => ref.path), [
    "events/a/votes/uid-1",
  ]);
  assert.equal(result.ballotsByUID.has("uid-2"), false);
  assert.deepEqual([...result.eventRefs.keys()], ["events/a"]);
});

test("partitions cleanup without letting one oversized user block others", () => {
  function ballot(uid, eventID) {
    return {
      id: uid,
      parent: {parent: {id: eventID, path: `events/${eventID}`}},
    };
  }
  const ballots = new Map([
    ["uid-1", [ballot("uid-1", "a"), ballot("uid-1", "b")]],
    ["uid-2", [ballot("uid-2", "c")]],
    ["oversized", [
      ballot("oversized", "d"),
      ballot("oversized", "e"),
      ballot("oversized", "f"),
    ]],
  ]);

  const result = _test.partitionUsersByEventLimit(
    ["uid-1", "uid-2", "oversized"],
    ballots,
    2,
  );

  assert.deepEqual(result.groups, [["uid-1"], ["uid-2"]]);
  assert.deepEqual(result.oversizedUIDs, ["oversized"]);
});

test("provisional claim rescan releases a user who added an active ballot", async () => {
  const operations = [];
  function eventRef(id, archived) {
    const ref = {
      kind: "event",
      id,
      archived,
      path: `events/${id}`,
      collection(name) {
        assert.equal(name, "votes");
        return {
          doc(uid) {
            return {
              kind: "ballot",
              id: uid,
              path: `${ref.path}/votes/${uid}`,
              parent: {parent: ref},
            };
          },
        };
      },
    };
    return ref;
  }
  const safeEvent = eventRef("safe", true);
  const activeEvent = eventRef("active", false);
  const events = new Map([["safe", safeEvent], ["active", activeEvent]]);
  const preliminaryBallot = safeEvent.collection("votes").doc("uid-1");

  const db = {
    doc(path) {
      assert.equal(path, "maintenance/anonymousBallotIndex");
      return {
        async get() {
          return {exists: true, data: () => ({complete: true})};
        },
      };
    },
    collection(name) {
      if (name === "events") {
        return {doc: id => events.get(id)};
      }
      return {
        doc(uid) {
          return {kind: name, id: uid, path: `${name}/${uid}`};
        },
      };
    },
    async getAll(...refs) {
      operations.push(`read:${refs.map(ref => ref.path).join(",")}`);
      return refs.map(ref => {
        if (ref.kind === "cleanedAnonymousUsers") {
          return {
            exists: true,
            ref,
            data: () => ({provisional: true}),
          };
        }
        if (ref.kind === "anonymousVoterBallots") {
          return {
            exists: true,
            data: () => ({complete: true, eventIds: ["safe", "active"]}),
          };
        }
        if (ref.kind === "ballot") {
          return {exists: true, id: ref.id, ref};
        }
        if (ref.kind === "event") {
          return {
            exists: true,
            ref,
            data: () => ({
              archived: ref.archived,
              resultsInitialized: true,
            }),
          };
        }
        throw new Error(`Unexpected ref ${ref.path}`);
      });
    },
    bulkWriter() {
      return {
        onWriteError() {},
        set(ref) {
          operations.push(`set:${ref.path}`);
        },
        delete(ref) {
          operations.push(`delete:${ref.path}`);
        },
        async close() {},
      };
    },
    async runTransaction() {
      throw new Error("an active ballot must prevent event locking");
    },
  };
  const auth = {
    async deleteUsers() {
      throw new Error("an active user must not be deleted");
    },
  };

  const result = await _test.cleanupEligibleUsers(
    db,
    auth,
    ["uid-1"],
    new Map([["uid-1", [preliminaryBallot]]]),
  );

  assert.equal(result.deletedUsers, 0);
  assert.equal(result.deferredUsers, 1);
  assert.ok(operations.indexOf("set:cleanedAnonymousUsers/uid-1") <
    operations.indexOf("read:anonymousVoterBallots/uid-1"));
  assert.equal(operations.at(-1), "delete:cleanedAnonymousUsers/uid-1");
});

test("one-time migration builds voter indexes before marking them complete", async () => {
  const indexWrites = [];
  let markerData;
  const markerRef = {
    async get() {
      return {exists: false};
    },
    async set(data) {
      markerData = data;
    },
  };
  const db = {
    doc(path) {
      assert.equal(path, "maintenance/anonymousBallotIndex");
      return markerRef;
    },
    collectionGroup(name) {
      assert.equal(name, "votes");
      return {
        async get() {
          return {
            size: 2,
            docs: [
              {id: "uid-1", ref: {path: "events/a/votes/uid-1"}},
              {id: "uid-1", ref: {path: "events/b/votes/uid-1"}},
            ],
          };
        },
      };
    },
    collection(name) {
      assert.equal(name, "anonymousVoterBallots");
      return {doc: uid => ({path: `${name}/${uid}`})};
    },
    bulkWriter() {
      return {
        onWriteError() {},
        set(ref, data, options) {
          indexWrites.push({ref, data, options});
        },
        async close() {},
      };
    },
  };

  const result = await _test.migrateBallotIndexes(db);

  assert.equal(result.indexedUsers, 1);
  assert.equal(result.indexedBallots, 2);
  assert.equal(indexWrites.length, 1);
  assert.equal(indexWrites[0].ref.path, "anonymousVoterBallots/uid-1");
  assert.deepEqual(indexWrites[0].options, {merge: true});
  assert.equal(markerData.complete, true);
  assert.equal(markerData.indexedUsers, 1);
  assert.equal(markerData.indexedBallots, 2);
});
