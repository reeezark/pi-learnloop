import assert from "node:assert/strict";
import test from "node:test";

import registerPiLearnLoop from "../../extensions/pi-learnloop.ts";

test("the extension registers only the user-triggered /learn command", () => {
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

  assert.equal(registrations.length, 1);
  assert.equal(registrations[0]?.name, "learn");
  assert.match(registrations[0]?.options.description ?? "", /evidence preview/i);
  assert.equal(typeof registrations[0]?.options.handler, "function");
});
