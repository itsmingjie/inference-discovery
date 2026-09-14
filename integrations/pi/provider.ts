import { createHash } from "node:crypto";
import {
  createProvider,
  type Model, type Provider, type StreamOptions, type ThinkingLevelMap,
} from "@earendil-works/pi-ai";
import { openAICompletionsApi, openAIResponsesApi } from "@earendil-works/pi-ai/compat";
import { Agent, fetch as directFetch } from "undici";

// These records come from the Go CLI, which validates the full descriptor contract.
export interface DiscoveryRecord {
  id: string;
  error?: string;
  descriptor?: {
    version: number;
    name: string;
    auth: { methods: string[] };
    api: {
      base_url: string;
      profiles: string[];
      capabilities: string[];
      models: {
        id: string;
        api?: { profiles: string[]; capabilities: string[] };
        name?: string;
        reasoning?: boolean;
        reasoning_efforts?: string[];
        context_window?: number;
        max_output_tokens?: number;
      }[];
    };
  };
}

// A direct dispatcher ignores environment proxies and never stores cookies.
export function transport() {
  const dispatcher = new Agent({ connect: { timeout: 10_000, rejectUnauthorized: true }, maxHeaderSize: 16_384 });
  return {
    close: () => dispatcher.destroy(),
    forEndpoint(endpoint: string): typeof fetch {
      return async (input, init) => {
        const request = new Request(input, init);
        if (request.url !== endpoint || request.method !== "POST") {
          throw new Error("Inference request must target the advertised endpoint");
        }
        const response = await directFetch(endpoint, {
          dispatcher,
          method: "POST",
          // Allowlist protocol headers: no SDK keys, cookies, or custom credentials.
          headers: { "content-type": "application/json", accept: "text/event-stream" },
          body: await request.text(),
          redirect: "error",
          credentials: "omit",
          signal: AbortSignal.any([request.signal, AbortSignal.timeout(120_000)]),
        });
        let bytes = 0;
        const body = (response.body as ReadableStream<Uint8Array> | null)?.pipeThrough(new TransformStream<Uint8Array, Uint8Array>({
          transform(chunk, controller) {
            bytes += chunk.byteLength;
            if (bytes > 16 * 1024 * 1024) throw new Error("Inference response exceeds 16 MiB");
            controller.enqueue(chunk);
          },
        }));
        return new Response(body, { status: response.status, headers: [...response.headers] });
      };
    },
  };
}

export function toProvider(record: DiscoveryRecord, http: ReturnType<typeof transport>): Provider {
  const d = record.descriptor;
  if (record.error || !d) throw new Error("descriptor unavailable");
  if (d.version !== 1) throw new Error("unsupported descriptor version");
  if (d.auth.methods.length !== 1 || d.auth.methods[0] !== "none") {
    throw new Error("only anonymous authentication is supported");
  }
  const api = d.api;
  // Scope registrations to the observation AND destination; never override built-ins.
  const id = "inference-" + createHash("sha256").update(JSON.stringify([record.id, api.base_url])).digest("hex").slice(0, 16);
  const label = `${d.name} · ${api.base_url} · ${api.base_url.startsWith("https:") ? "encrypted" : "unencrypted"}`;
  const baseUrl = new URL(api.base_url).href.replace(/\/+$/, "");
  const chat = openAICompletionsApi(), responses = openAIResponsesApi();
  const chatFetch = http.forEndpoint(baseUrl + "/chat/completions");
  const responsesFetch = http.forEndpoint(baseUrl + "/responses");
  const options = (api: string, input?: StreamOptions): StreamOptions => ({
    ...input,
    // Pi's OpenAI codec requires a nonempty sentinel; the transport never sends it.
    apiKey: "anonymous",
    headers: {},
    fetch: api === "openai-responses" ? responsesFetch : chatFetch,
    maxRetries: 0,
    timeoutMs: 120_000,
  });
  const models: Model<"openai-responses" | "openai-completions">[] = api.models.flatMap((m) => {
    const support = m.api ?? api;
    if (!support.capabilities.includes("streaming")) return [];
    const useResponses = support.profiles.includes("openai-responses");
    if (!useResponses && !(support.profiles.includes("openai-chat-completions") && support.capabilities.includes("function-tools"))) return [];
    const thinkingLevelMap: ThinkingLevelMap = {};
    for (const level of ["minimal", "low", "medium", "high", "xhigh", "max"] as const) {
      thinkingLevelMap[level] = m.reasoning_efforts?.includes(level) ? level : null;
    }
    const contextWindow = m.context_window ?? Math.max(8192, m.max_output_tokens ?? 0);
    return {
      id: m.id,
      name: `${m.name ?? m.id} · ${label}`,
      api: useResponses ? "openai-responses" : "openai-completions",
      provider: id,
      baseUrl,
      reasoning: m.reasoning === true,
      thinkingLevelMap,
      input: ["text"],
      contextWindow,
      maxTokens: m.max_output_tokens ?? Math.min(1024, contextWindow),
      // Use the portable Chat Completions fields covered by function-tools.
      compat: useResponses ? undefined : {
        supportsDeveloperRole: false,
        supportsStore: false,
        supportsStrictMode: false,
        supportsUsageInStreaming: false,
        maxTokensField: "max_tokens",
      },
      // Required Pi bookkeeping; the discovery protocol carries no pricing.
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    };
  });
  if (models.length === 0) throw new Error("streaming Responses or Chat Completions with function-tools is required");
  return createProvider({
    id,
    name: label,
    baseUrl,
    models,
    auth: { apiKey: { name: "Open access", resolve: async () => ({ auth: {}, source: "Open access" }) } },
    api: {
      stream: (model, context, input) => (model.api === "openai-responses" ? responses : chat).stream(model, context, options(model.api, input)),
      streamSimple: (model, context, input) => (model.api === "openai-responses" ? responses : chat).streamSimple(model, context, { ...input, ...options(model.api, input) }),
    },
  });
}
