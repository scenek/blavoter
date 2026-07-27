"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

test("formats standalone vote counts with the requested Czech forms", async () => {
  const { formatVoteCount } = await import("../static/vote-count.mjs");

  assert.equal(formatVoteCount(0), "0 hlasů");
  assert.equal(formatVoteCount(1), "1 hlas");
  assert.equal(formatVoteCount(2), "2 hlasy");
  assert.equal(formatVoteCount(3), "3 hlasy");
  assert.equal(formatVoteCount(4), "4 hlasy");
  assert.equal(formatVoteCount(5), "5 hlasů");
  assert.equal(formatVoteCount(10), "10 hlasů");
});
