import { VERSION, type ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { DaemonEvidenceClient } from "./lib/daemon-client.ts";
import { createLearnCommand } from "./lib/learn-command.ts";

export default function registerPiLearnLoop(pi: ExtensionAPI): void {
  pi.registerCommand("learn", {
    description: "Preview changed-Go evidence and generate three learning questions",
    handler: createLearnCommand(new DaemonEvidenceClient(), VERSION),
  });
}
