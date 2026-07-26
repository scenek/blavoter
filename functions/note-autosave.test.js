"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const modulePromise = import("../static/note-autosave.mjs");

function fakeTimers() {
  let callback;
  return {
    setTimer(next, delay) {
      assert.equal(delay, 700);
      callback = next;
      return 1;
    },
    clearTimer() {
      callback = undefined;
    },
    async run() {
      assert.equal(typeof callback, "function");
      const next = callback;
      callback = undefined;
      await next();
    },
  };
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return {promise, resolve, reject};
}

test("debounces edits and saves only the newest draft", async () => {
  const {createNoteAutosave} = await modulePromise;
  const timers = fakeTimers();
  const saved = [];
  const autosave = createNoteAutosave({
    initialNote: "",
    saveNote: async note => {
      saved.push(note);
      return note;
    },
    delayMs: 700,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  autosave.setDraft("first");
  autosave.setDraft("second");
  await timers.run();

  assert.deepEqual(saved, ["second"]);
  assert.equal(autosave.getConfirmed(), "second");
});

test("flush saves immediately without waiting for debounce", async () => {
  const {createNoteAutosave} = await modulePromise;
  const timers = fakeTimers();
  const saved = [];
  const autosave = createNoteAutosave({
    initialNote: "",
    saveNote: async note => {
      saved.push(note);
      return note;
    },
    delayMs: 700,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  autosave.setDraft("on blur");
  await autosave.flush();

  assert.deepEqual(saved, ["on blur"]);
});

test("failed save retains the draft and retries only when requested", async () => {
  const {createNoteAutosave} = await modulePromise;
  const timers = fakeTimers();
  const states = [];
  let attempts = 0;
  const autosave = createNoteAutosave({
    initialNote: "saved",
    saveNote: async note => {
      attempts++;
      if (attempts === 1) throw new Error("offline");
      return note;
    },
    onState: state => states.push(state),
    delayMs: 700,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  autosave.setDraft("unsaved");
  await timers.run();

  assert.equal(attempts, 1);
  assert.equal(autosave.getDraft(), "unsaved");
  assert.equal(autosave.getConfirmed(), "saved");
  assert.equal(states.at(-1), "error");

  await autosave.flush();
  assert.equal(attempts, 2);
  assert.equal(autosave.getConfirmed(), "unsaved");
});

test("newer draft queues behind an in-flight save without being overwritten", async () => {
  const {createNoteAutosave} = await modulePromise;
  const timers = fakeTimers();
  const requests = [];
  const saves = [];
  const autosave = createNoteAutosave({
    initialNote: "",
    saveNote: note => {
      saves.push(note);
      const request = deferred();
      requests.push(request);
      return request.promise;
    },
    delayMs: 700,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  autosave.setDraft("first");
  const firstSave = timers.run();
  await Promise.resolve();
  autosave.setDraft("second");

  requests[0].resolve("first");
  await Promise.resolve();
  await Promise.resolve();
  assert.deepEqual(saves, ["first", "second"]);
  assert.equal(autosave.getDraft(), "second");

  requests[1].resolve("second");
  await firstSave;
  assert.equal(autosave.getConfirmed(), "second");
});

test("current successful response applies the canonical trimmed note", async () => {
  const {createNoteAutosave} = await modulePromise;
  const timers = fakeTimers();
  const autosave = createNoteAutosave({
    initialNote: "",
    saveNote: async note => note.trim(),
    delayMs: 700,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  autosave.setDraft("  citrus  ");
  await timers.run();

  assert.equal(autosave.getDraft(), "citrus");
  assert.equal(autosave.getConfirmed(), "citrus");
});
