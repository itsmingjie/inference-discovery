import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { toProvider, transport, type DiscoveryRecord } from "./provider.ts";

const run = promisify(execFile);
// Source checkouts and release archives carry their own CLI; PATH is a fallback.
const binary = ["../../bin/inference", "../../inference"]
  .map((path) => fileURLToPath(new URL(path, import.meta.url)))
  .find((path) => existsSync(path)) ?? "inference";

export default async function discovery(pi: ExtensionAPI) {
  const http = transport();
  const abort = new AbortController();
  let registered = new Map<string, string>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let warning = "";
  let notify: ((message: string) => void) | undefined;

  pi.on("session_start", async (_event, ctx) => {
    notify = (message) => ctx.ui.notify(message, "warning");
  });
  pi.on("session_shutdown", async () => {
    abort.abort();
    clearTimeout(timer);
    for (const id of registered.keys()) pi.unregisterProvider(id);
    registered.clear();
    await http.close();
  });

  async function refresh() {
    const current = new Map<string, string>();
    const problems: string[] = [];
    try {
      const args = ["discover", "--json"];
      for (const [variable, option] of [["INFERENCE_INTERFACE", "--interface"], ["INFERENCE_DESCRIPTOR_URL", "--descriptor-url"]]) {
        const value = process.env[variable];
        if (value) args.push(option, value);
      }
      const { stdout } = await run(binary, args, {
        timeout: 15_000, maxBuffer: 4 * 1024 * 1024, signal: abort.signal,
      });
      const records: DiscoveryRecord[] = JSON.parse(stdout);
      if (!Array.isArray(records) || records.length > 64) throw new Error("invalid discovery output");
      if (abort.signal.aborted) return;
      for (const record of records) {
        try {
          const provider = toProvider(record, http);
          const fingerprint = JSON.stringify(record.descriptor);
          if (registered.get(provider.id) !== fingerprint) pi.registerProvider(provider);
          current.set(provider.id, fingerprint);
        } catch (error) {
          problems.push(`${record.descriptor?.name ?? "Inference provider"} skipped: ${error instanceof Error ? error.message : "invalid metadata"}`);
        }
      }
    } catch {
      if (abort.signal.aborted) return;
      problems.push("Inference discovery failed. Check the network; use INFERENCE_INTERFACE or INFERENCE_DESCRIPTOR_URL to override discovery. If the CLI is missing, reinstall with make install-pi.");
    }
    for (const id of registered.keys()) {
      if (!current.has(id)) pi.unregisterProvider(id);
    }
    registered = current;
    const message = [...new Set(problems)].join("\n");
    if (message && message !== warning) {
      if (notify) notify(message);
      else console.error(message);
    }
    warning = message;
    timer = setTimeout(refresh, 5000);
    timer.unref();
  }

  // Pi awaits extension factories, making initial models available to /model.
  await refresh();
}
