import assert from "node:assert/strict";
import test from "node:test";

import registerPiLearnLoop from "../../extensions/pi-learnloop.ts";

test("the extension registers only the user-triggered learning commands", () => {
  const registrations: Array<{
    name: string;
    options: { description?: string; handler: (args: string, context: never) => Promise<void> };
  }> = [];
  const pi = {
    registerCommand(name: string, options: (typeof registrations)[number]["options"]) {
      registrations.push({ name, options });
    },
  };

  registerPiLearnLoop(pi as never);

  assert.equal(registrations.length, 2);
  assert.equal(registrations[0]?.name, "learn");
  assert.match(registrations[0]?.options.description ?? "", /three learning questions/i);
  assert.equal(typeof registrations[0]?.options.handler, "function");
  assert.equal(registrations[1]?.name, "learn-history");
  assert.match(registrations[1]?.options.description ?? "", /local learning history/i);
  assert.equal(typeof registrations[1]?.options.handler, "function");
});
