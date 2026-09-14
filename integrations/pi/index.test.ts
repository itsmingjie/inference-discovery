import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { EventEmitter, once } from "node:events";
import { mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { test } from "node:test";
import { promisify } from "node:util";
import { ModelRuntime, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { InMemoryCredentialStore, InMemoryModelsStore } from "@earendil-works/pi-ai";
import discovery from "./index.ts";

test("real discovery updates Pi's native registry and cleans up on shutdown", { timeout: 30_000 }, async (t) => {
  let available = true;
  const descriptor = {
    version: 1, name: "Office", auth: { methods: ["none"] },
    api: {
      base_url: "http://127.0.0.1:8000/v1", profiles: ["openai-chat-completions"],
      capabilities: ["streaming", "function-tools"], models: [{ id: "first" }, { id: "second" }],
    },
  };
  const server = createServer((_req, res) => {
    res.writeHead(available ? 200 : 503, { "content-type": "application/json" });
    res.end(JSON.stringify(descriptor));
  }).listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => { server.closeAllConnections(); server.close(); });
  const address = server.address();
  assert(address && typeof address !== "string");
  const url = `http://127.0.0.1:${address.port}/descriptor`;
  const oldURL = process.env.INFERENCE_DESCRIPTOR_URL;
  process.env.INFERENCE_DESCRIPTOR_URL = url;
  t.after(() => {
    if (oldURL === undefined) delete process.env.INFERENCE_DESCRIPTOR_URL;
    else process.env.INFERENCE_DESCRIPTOR_URL = oldURL;
  });
  const runtime = await ModelRuntime.create({
    credentials: new InMemoryCredentialStore(), modelsStore: new InMemoryModelsStore(),
    modelsPath: null, refreshOnCreate: false,
  });
  const events = new EventEmitter();
  const handlers = new Map<string, () => Promise<void>>();
  const changed = () => once(events, "changed", { signal: AbortSignal.timeout(10_000) });
  const pi = {
    on: (name: string, handler: () => Promise<void>) => handlers.set(name, handler),
    registerProvider: (provider: Parameters<ModelRuntime["registerNativeProvider"]>[0]) => {
      runtime.registerNativeProvider(provider);
      events.emit("changed");
    },
    unregisterProvider: (id: string) => { runtime.unregisterProvider(id); events.emit("changed"); },
  } as unknown as ExtensionAPI;
  await discovery(pi);
  t.after(async () => { await handlers.get("session_shutdown")!(); });
  const models = () => runtime.getModels().filter((m) => m.provider.startsWith("inference-"));
  assert.deepEqual(models().map((m) => m.id), ["first", "second"]);
  assert.equal((await runtime.getAvailable(models()[0].provider)).length, 2);
  const providerID = models()[0].provider;
  descriptor.api.models = [{ id: "second" }];
  await changed();
  assert.deepEqual(models().map((m) => m.id), ["second"]);
  assert.equal(models()[0].provider, providerID);
  available = false;
  await changed();
  assert.equal(models().length, 0);
  available = true;
  await changed();
  assert.equal(models().length, 1);
  await handlers.get("session_shutdown")!();
  assert.equal(models().length, 0);
});

test("Pi loads the package and lists discovered models without endpoint configuration files", { timeout: 30_000 }, async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "inference-pi-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const server = createServer((_req, res) => {
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({
      version: 1, name: "Office", auth: { methods: ["none"] },
      api: { base_url: "http://127.0.0.1:8000/v1", profiles: ["openai-responses"], capabilities: ["streaming"], models: [{ id: "discovery-smoke-model" }] },
    }));
  }).listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => { server.closeAllConnections(); server.close(); });
  const address = server.address();
  assert(address && typeof address !== "string");
  const options = {
    timeout: 25_000,
    env: {
      PATH: "/usr/bin:/bin", PI_CODING_AGENT_DIR: directory,
      INFERENCE_DESCRIPTOR_URL: `http://127.0.0.1:${address.port}/descriptor`,
    },
  };
  const cli = "node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js";
  await promisify(execFile)(process.execPath, [cli, "install", resolve(".")], options);
  const { stdout, stderr } = await promisify(execFile)(process.execPath, [cli, "--list-models"], options);
  assert.match(stdout, /discovery-smoke-model/, stderr);
});
