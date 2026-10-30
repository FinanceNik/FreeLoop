import { test } from "node:test";
import assert from "node:assert/strict";
import { windowSession } from "./windows.ts";

test("a session window is named by its query string", () => {
  assert.equal(windowSession("?session=ses_06grx852522zxavwvmvknp4jr0"), "ses_06grx852522zxavwvmvknp4jr0");
  assert.equal(windowSession(""), null);
  assert.equal(windowSession("?session="), null);
  assert.equal(windowSession("?other=1"), null);
  assert.equal(windowSession("?session=../x"), null);
});
