import assert from "node:assert/strict";
import test from "node:test";

import { SessionManager } from "@earendil-works/pi-coding-agent";

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

test("the registered Session path lists only the current cwd and current Session directory", async (t) => {
  const originalList = SessionManager.list;
  const listCalls: Array<[string, string | undefined]> = [];
  SessionManager.list = async (cwd, sessionDir) => {
    listCalls.push([cwd, sessionDir]);
    return [];
  };
  t.after(() => {
    SessionManager.list = originalList;
  });

  const registrations: Array<{
    name: string;
    options: { handler: (args: string, context: never) => Promise<void> };
  }> = [];
  registerPiLearnLoop({
    registerCommand(name: string, options: (typeof registrations)[number]["options"]) {
      registrations.push({ name, options });
    },
  } as never);
  const learn = registrations.find((registration) => registration.name === "learn");
  assert.ok(learn);
  const notifications: string[] = [];

  await learn.options.handler("", {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    sessionManager: { getSessionDir: () => "/current/session-directory" },
    ui: {
      select: async () => "Pi Session",
      input: async () => undefined,
      confirm: async () => false,
      notify: (message: string) => notifications.push(message),
    },
  } as never);

  assert.deepEqual(listCalls, [["/work/repository", "/current/session-directory"]]);
  assert.deepEqual(notifications, ["No Pi Sessions are available for the current project."]);
});
