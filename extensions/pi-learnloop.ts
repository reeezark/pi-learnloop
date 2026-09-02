import { SessionManager, VERSION, type ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { DaemonEvidenceClient } from "./lib/daemon-client.ts";
import { createLearnCommand, createLearnHistoryCommand } from "./lib/learn-command.ts";

export default function registerPiLearnLoop(pi: ExtensionAPI): void {
  const client = new DaemonEvidenceClient();
  pi.registerCommand("learn", {
    description: "Review a Git changeset or explicitly bound Pi Session with three learning questions",
    handler: createLearnCommand(
      client,
      VERSION,
      (cwd, sessionDir) => SessionManager.list(cwd, sessionDir),
    ),
  });
  pi.registerCommand("learn-history", {
    description: "Show recent local learning history for the current repository",
    handler: createLearnHistoryCommand(client),
  });
}
