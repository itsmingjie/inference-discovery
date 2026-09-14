import assert from "node:assert/strict";
import { once } from "node:events";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { test, type TestContext } from "node:test";
import { createModels, getSupportedThinkingLevels, Type, type Context } from "@earendil-works/pi-ai";
import { toProvider, transport, type DiscoveryRecord } from "./provider.ts";

function record(base = "http://127.0.0.1:8000/v1"): DiscoveryRecord {
  return {
    id: "Office._inference._tcp.local.",
    descriptor: {
      version: 1, name: "Office", auth: { methods: ["none"] },
      api: {
        base_url: base, profiles: ["openai-chat-completions"], capabilities: ["streaming", "function-tools"],
        models: [{ id: "chat" }, { id: "reasoner", reasoning: true, reasoning_efforts: ["low", "high"], context_window: 32768, max_output_tokens: 4096 }],
      },
    },
  };
}

async function server(t: TestContext, handler: (req: IncomingMessage, res: ServerResponse) => void) {
  const http = createServer(handler).listen(0, "127.0.0.1");
  await once(http, "listening");
  t.after(() => { http.closeAllConnections(); http.close(); });
  const address = http.address();
  assert(address && typeof address !== "string");
  return `http://127.0.0.1:${address.port}`;
}

const context: Context = { messages: [{ role: "user", content: "hello", timestamp: 0 }] };

test("catalog maps into Pi with anonymous auth and endpoint-scoped identity", async (t) => {
  const http = transport();
  t.after(http.close);
  const provider = toProvider(record(), http);
  const models = createModels();
  models.setProvider(provider);
  const available = await models.getAvailable();
  assert.equal(available.length, 2);
  assert.deepEqual((await models.getAuth(provider.id))?.auth, {});
  assert.match(available[0].name, /Office.*http:\/\/127\.0\.0\.1:8000\/v1.*unencrypted/);
  assert.equal(available[1].contextWindow, 32768);
  assert.equal(available[1].maxTokens, 4096);
  assert.deepEqual(getSupportedThinkingLevels(available[1]), ["off", "low", "high"]);
  assert.notEqual(toProvider(record("https://elsewhere.example/v1"), http).id, provider.id);
  models.deleteProvider(provider.id);
  assert.equal((await models.getAvailable()).length, 0);
});

test("unsupported auth, profiles and missing coding capabilities are rejected", (t) => {
  const http = transport();
  t.after(http.close);
  for (const methods of [["bearer"], ["none", "bearer"], []]) {
    const input = record();
    input.descriptor!.auth.methods = methods;
    assert.throws(() => toProvider(input, http), /authentication/);
  }
  for (const capabilities of [[], ["streaming"], ["function-tools"]]) {
    const input = record();
    input.descriptor!.api.capabilities = capabilities;
    assert.throws(() => toProvider(input, http), /required/);
  }
  const input = record();
  input.descriptor!.api.profiles = ["embeddings"];
  assert.throws(() => toProvider(input, http), /required/);
  assert.throws(() => toProvider({ id: "bad", error: "malformed descriptor" }, http), /unavailable/);
});

test("mixed catalogs select an API per model and omit text-only models", (t) => {
  const http = transport();
  t.after(http.close);
  const input = record();
  input.descriptor!.api.profiles = [];
  input.descriptor!.api.capabilities = ["streaming"];
  input.descriptor!.api.models = [
    { id: "text", api: { profiles: ["openai-chat-completions"], capabilities: ["streaming"] } },
    { id: "tools", api: { profiles: ["openai-chat-completions"], capabilities: ["streaming", "function-tools"] } },
    { id: "both", api: { profiles: ["openai-chat-completions", "openai-responses"], capabilities: ["streaming", "function-tools"] } },
  ];
  const models = toProvider(input, http).getModels();
  assert.deepEqual(models.map((m) => [m.id, m.api]), [["tools", "openai-completions"], ["both", "openai-responses"]]);
});

test("Pi streams text and tool calls directly with no credentials", async (t) => {
  for (const [name, value] of Object.entries({
    OPENAI_API_KEY: "CANARY-ENV-KEY",
    HTTP_PROXY: "http://CANARY:CANARY@127.0.0.1:1",
    HTTPS_PROXY: "http://CANARY:CANARY@127.0.0.1:1",
  })) {
    const previous = process.env[name];
    process.env[name] = value;
    t.after(() => {
      if (previous === undefined) delete process.env[name];
      else process.env[name] = previous;
    });
  }
  const requests: { url: string; headers: IncomingMessage["headers"]; body: Record<string, unknown> }[] = [];
  const base = await server(t, async (req, res) => {
    let body = "";
    for await (const chunk of req) body += chunk;
    requests.push({ url: req.url!, headers: req.headers, body: JSON.parse(body) });
    res.writeHead(200, { "content-type": "text/event-stream" });
    for (const delta of [
      { content: "Hello" },
      { tool_calls: [{ index: 0, id: "call_1", type: "function", function: { name: "lookup", arguments: '{"q":' } }] },
      { tool_calls: [{ index: 0, function: { arguments: '"hello"}' } }] },
    ]) res.write(`data: ${JSON.stringify({ id: "test", choices: [{ index: 0, delta, finish_reason: null }] })}\n\n`);
    res.end('data: {"id":"test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}\n\ndata: [DONE]\n\n');
  });
  const http = transport();
  t.after(http.close);
  const provider = toProvider(record(base + "/prefix/v1///"), http);
  const models = createModels();
  models.setProvider(provider);
  const stream = models.streamSimple(provider.getModels()[0], {
    ...context,
    tools: [{ name: "lookup", description: "Look up a term", parameters: Type.Object({ q: Type.String() }) }],
  }, {
    apiKey: "CANARY-KEY", headers: { Authorization: "Bearer CANARY", Cookie: "CANARY", "X-Api-Key": "CANARY" },
  });
  const events = [];
  for await (const event of stream) events.push(event.type);
  const result = await stream.result();
  assert.equal(result.stopReason, "toolUse", result.errorMessage);
  assert(events.includes("text_delta"));
  assert.deepEqual(result.content.find((c) => c.type === "toolCall"), { type: "toolCall", id: "call_1", name: "lookup", arguments: { q: "hello" } });
  assert.equal(requests.length, 1);
  assert.equal(requests[0].url, "/prefix/v1/chat/completions");
  assert.equal(requests[0].body.model, "chat");
  assert.equal(requests[0].body.stream, true);
  assert.match(JSON.stringify(requests[0].body.tools), /lookup/);
  assert(!JSON.stringify(requests[0].headers).includes("CANARY"));
  assert.equal(requests[0].headers.authorization, undefined);
  assert.equal(requests[0].headers.cookie, undefined);
});

test("transport bounds responses and rejects requests outside the advertised route", async (t) => {
  const base = await server(t, (_req, res) => {
    res.writeHead(200, { "content-type": "text/event-stream" });
    res.end(Buffer.alloc(16 * 1024 * 1024 + 1, 65));
  });
  const http = transport();
  t.after(http.close);
  const fetch = http.forEndpoint(base + "/v1/chat/completions");
  await assert.rejects(fetch(base + "/elsewhere", { method: "POST" }), /advertised endpoint/);
  const response = await fetch(base + "/v1/chat/completions", { method: "POST", body: "{}" });
  await assert.rejects(response.text(), /16 MiB/);
  const abort = new AbortController();
  abort.abort();
  await assert.rejects(fetch(base + "/v1/chat/completions", { method: "POST", signal: abort.signal }));
});

test("redirects, auth failures and server failures never retry or change destination", async (t) => {
  let targetRequests = 0;
  const target = await server(t, (_req, res) => { targetRequests++; res.end(); });
  let status = 302;
  let requests = 0;
  const base = await server(t, (_req, res) => {
    requests++;
    res.writeHead(status, { location: target, "content-type": "application/json", "retry-after": "0" });
    res.end('{"error":{"message":"unavailable"}}');
  });
  const http = transport();
  t.after(http.close);
  const provider = toProvider(record(base + "/v1"), http);
  for (status of [302, 307, 401, 403, 429, 503]) {
    const before = requests;
    const result = await provider.streamSimple(provider.getModels()[0], context, { maxRetries: 5 }).result();
    assert.equal(result.stopReason, "error");
    assert.equal(requests, before + 1);
  }
  assert.equal(targetRequests, 0);
});

test("Responses profile uses Pi's Responses stream", async (t) => {
  let path = "";
  const base = await server(t, (req, res) => {
    path = req.url!;
    assert.equal(req.headers.authorization, undefined);
    res.writeHead(200, { "content-type": "text/event-stream" });
    const response = { id: "r1", status: "completed", output: [], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    for (const event of [
      { type: "response.created", response: { ...response, status: "in_progress" } },
      { type: "response.output_item.added", output_index: 0, item: { type: "message", id: "m1", role: "assistant", content: [] } },
      { type: "response.content_part.added", item_id: "m1", output_index: 0, content_index: 0, part: { type: "output_text", text: "", annotations: [] } },
      { type: "response.output_text.delta", item_id: "m1", output_index: 0, content_index: 0, delta: "Hello" },
      { type: "response.completed", response },
    ]) res.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    res.end();
  });
  const http = transport();
  t.after(http.close);
  const input = record(base + "/v1");
  input.descriptor!.api.profiles = ["openai-responses"];
  input.descriptor!.api.capabilities = ["streaming"];
  const provider = toProvider(input, http);
  const result = await provider.streamSimple(provider.getModels()[0], context).result();
  assert.equal(path, "/v1/responses");
  assert.equal(result.stopReason, "stop", result.errorMessage);
  assert(result.content.some((c) => c.type === "text" && c.text === "Hello"));
});
