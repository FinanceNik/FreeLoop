import { test } from "node:test";
import assert from "node:assert/strict";
import { canMove, queueSummary } from "./queue.ts";

const t = (state: string) => ({ id: state, title: state, state, projectRoot: "/p" });

test("the queue says what it is doing", () => {
  assert.equal(queueSummary([t("running"), t("waiting"), t("waiting")], false, 3), "1 of 3 running, 2 waiting.");
  assert.equal(queueSummary([t("failed")], true, 2), "Paused. 0 of 2 running, 0 waiting, 1 failed.");
});

test("only a waiting task moves, and not past the ends", () => {
  const list = [t("waiting"), t("running"), t("waiting")];
  assert.equal(canMove(list, 0, -1), false);
  assert.equal(canMove(list, 0, 1), true);
  assert.equal(canMove(list, 1, 1), false);
  assert.equal(canMove(list, 2, 1), false);
});
