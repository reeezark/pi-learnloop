import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { DaemonEvidenceClient } from "./lib/daemon-client.ts";
import { createLearnCommand } from "./lib/learn-command.ts";

export default function registerPiLearnLoop(pi: ExtensionAPI): void {
  pi.registerCommand("learn", {
    description: "Choose a Git changeset and inspect its changed-Go evidence preview",
    handler: createLearnCommand(new DaemonEvidenceClient()),
  });
}
