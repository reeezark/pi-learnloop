import { VERSION, type ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { DaemonEvidenceClient } from "./lib/daemon-client.ts";
import { createLearnCommand, createLearnHistoryCommand } from "./lib/learn-command.ts";

export default function registerPiLearnLoop(pi: ExtensionAPI): void {
  const client = new DaemonEvidenceClient();
  pi.registerCommand("learn", {
    description: "Preview changed-Go evidence and generate three learning questions",
    handler: createLearnCommand(client, VERSION),
  });
  pi.registerCommand("learn-history", {
    description: "Show recent local learning history for the current repository",
    handler: createLearnHistoryCommand(client),
  });
}
