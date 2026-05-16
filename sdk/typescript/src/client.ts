export type PCMEvent = {
  type: string;
  payload: Record<string, unknown>;
};

/** Options for POST /v1/memories (HTTP-only transport; gRPC uses proto). */
export type StoreOptions = {
  tags?: string[];
  embedding?: number[];
  embeddingModel?: string;
  embeddingSpace?: string;
  sourceAgentId?: string;
  encryptContent?: boolean;
  /** RFC3339 / RFC3339Nano */
  expiresAt?: string;
};

/** Options for POST /v1/retrieve */
export type RetrieveOptions = {
  asOf?: string;
  sourceAgentId?: string;
  embeddingSpace?: string;
  tags?: string[];
  tagsMatch?: "any" | "all";
};

function parseSSEChunk(buffer: string, onEvent: (ev: PCMEvent) => void): string {
  const parts = buffer.split("\n\n");
  const rest = parts.pop() ?? "";
  for (const block of parts) {
    const lines = block.split("\n");
    let data = "";
    for (const line of lines) {
      if (line.startsWith("data:")) {
        data += line.slice(5).trimStart();
      }
    }
    if (!data) continue;
    try {
      onEvent(JSON.parse(data) as PCMEvent);
    } catch {
      // ignore malformed frames
    }
  }
  return rest;
}

export class PCMIClient {
  constructor(private baseUrl: string, private apiKey: string) {}

  private headers(): Record<string, string> {
    return { "X-API-Key": this.apiKey, "Content-Type": "application/json" };
  }

  async store(
    path: string,
    content: string,
    metadata: Record<string, unknown> = {},
    opts?: StoreOptions,
  ) {
    const body: Record<string, unknown> = { path, content, metadata };
    if (opts?.tags?.length) body.tags = opts.tags;
    if (opts?.embedding?.length) body.embedding = opts.embedding;
    if (opts?.sourceAgentId) body.source_agent_id = opts.sourceAgentId;
    if (opts?.embeddingSpace) body.embedding_space = opts.embeddingSpace;
    if (opts?.embeddingModel) body.embedding_model = opts.embeddingModel;
    if (opts?.encryptContent) body.encrypt_content = true;
    if (opts?.expiresAt) body.expires_at = opts.expiresAt;
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`store failed: ${res.status}`);
    return res.json();
  }

  async retrieve(
    pathPrefix: string,
    query = "",
    limit = 10,
    opts?: RetrieveOptions,
  ) {
    const body: Record<string, unknown> = { path_prefix: pathPrefix, query, limit };
    if (opts?.asOf) body.as_of = opts.asOf;
    if (opts?.sourceAgentId) body.source_agent_id = opts.sourceAgentId;
    if (opts?.embeddingSpace) body.embedding_space = opts.embeddingSpace;
    if (opts?.tags?.length) body.tags = opts.tags;
    if (opts?.tagsMatch) body.tags_match = opts.tagsMatch;
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/retrieve`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`retrieve failed: ${res.status}`);
    return res.json();
  }

  async rollback(path: string, opts?: { version?: number; asOf?: string }) {
    const body: Record<string, unknown> = { path };
    if (opts?.version != null) body.version = opts.version;
    if (opts?.asOf) body.as_of = opts.asOf;
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/rollback`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`rollback failed: ${res.status}`);
    return res.json();
  }

  async listAudit(limit = 50, offset = 0, since?: string) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/audit`);
    u.searchParams.set("limit", String(limit));
    u.searchParams.set("offset", String(offset));
    if (since) u.searchParams.set("since", since);
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listAudit failed: ${res.status}`);
    return res.json();
  }

  async ingestEvent(
    eventType: string,
    payload: Record<string, unknown> = {},
    opts?: { agentId?: string; correlationId?: string },
  ) {
    const body: Record<string, unknown> = { event_type: eventType, payload };
    if (opts?.agentId) body.agent_id = opts.agentId;
    if (opts?.correlationId) body.correlation_id = opts.correlationId;
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/events`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`ingestEvent failed: ${res.status}`);
    return res.json();
  }

  async getMemory(path: string, opts?: { version?: number; asOf?: string }) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/${path}`);
    if (opts?.version != null) u.searchParams.set("version", String(opts.version));
    if (opts?.asOf) u.searchParams.set("as_of", opts.asOf);
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`getMemory failed: ${res.status}`);
    return res.json();
  }

  async batchStore(items: Record<string, unknown>[]) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/batch`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ items }),
    });
    if (!res.ok) throw new Error(`batchStore failed: ${res.status}`);
    return res.json();
  }

  async batchRetrieve(queries: Record<string, unknown>[]) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/retrieve/batch`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ queries }),
    });
    if (!res.ok) throw new Error(`batchRetrieve failed: ${res.status}`);
    return res.json();
  }

  async exportMemories(pathPrefix: string, limit = 500, includeEmbeddings = false) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/export`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({
        path_prefix: pathPrefix,
        limit,
        include_embeddings: includeEmbeddings,
      }),
    });
    if (!res.ok) throw new Error(`exportMemories failed: ${res.status}`);
    return res.json();
  }

  async importMemories(entries: Record<string, unknown>[], mode = "skip") {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/import`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ entries, mode }),
    });
    if (!res.ok) throw new Error(`importMemories failed: ${res.status}`);
    return res.json();
  }

  async listTenants(limit = 100) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/admin/tenants`);
    u.searchParams.set("limit", String(limit));
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listTenants failed: ${res.status}`);
    return res.json();
  }

  async createTenant(slug: string, name: string, settings: Record<string, unknown> = {}) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/admin/tenants`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ slug, name, settings }),
    });
    if (!res.ok) throw new Error(`createTenant failed: ${res.status}`);
    return res.json();
  }

  async listApiKeys(opts?: { tenantId?: string; limit?: number }) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/admin/api-keys`);
    u.searchParams.set("limit", String(opts?.limit ?? 50));
    if (opts?.tenantId) u.searchParams.set("tenant_id", opts.tenantId);
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listApiKeys failed: ${res.status}`);
    return res.json();
  }

  async createApiKey(
    name: string,
    opts?: { tenantId?: string; role?: string; expiresAt?: string },
  ) {
    const body: Record<string, unknown> = { name, role: opts?.role ?? "user" };
    if (opts?.tenantId) body.tenant_id = opts.tenantId;
    if (opts?.expiresAt) body.expires_at = opts.expiresAt;
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/admin/api-keys`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`createApiKey failed: ${res.status}`);
    return res.json();
  }

  async rotateApiKey(keyId: string, name = "") {
    const res = await fetch(
      `${this.baseUrl.replace(/\/$/, "")}/v1/admin/api-keys/${encodeURIComponent(keyId)}/rotate`,
      { method: "POST", headers: this.headers(), body: JSON.stringify({ name }) },
    );
    if (!res.ok) throw new Error(`rotateApiKey failed: ${res.status}`);
    return res.json();
  }

  async getHistory(path: string, limit = 50) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/history`);
    u.searchParams.set("path", path);
    u.searchParams.set("limit", String(limit));
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`getHistory failed: ${res.status}`);
    return res.json();
  }

  async listEventSchemas() {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/events/schemas`, {
      headers: { "X-API-Key": this.apiKey },
    });
    if (!res.ok) throw new Error(`listEventSchemas failed: ${res.status}`);
    return res.json();
  }

  async summarize(pathPrefix: string, opts?: { limit?: number; style?: "brief" | "detailed" }) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/summarize`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({
        path_prefix: pathPrefix,
        limit: opts?.limit ?? 20,
        style: opts?.style ?? "brief",
      }),
    });
    if (!res.ok) throw new Error(`summarize failed: ${res.status}`);
    return res.json();
  }

  async listWebhookDeadLetter(limit = 50) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/webhooks/dead-letter`);
    u.searchParams.set("limit", String(limit));
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listWebhookDeadLetter failed: ${res.status}`);
    return res.json();
  }

  async listDistilled(pathPrefix: string, limit = 50) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/distilled`);
    u.searchParams.set("path_prefix", pathPrefix);
    u.searchParams.set("limit", String(limit));
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listDistilled failed: ${res.status}`);
    return res.json();
  }

  async compact(path: string, opts?: { keepSuperseded?: number }) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/compact`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({
        path,
        keep_superseded: opts?.keepSuperseded ?? 20,
      }),
    });
    if (!res.ok) throw new Error(`compact failed: ${res.status}`);
    return res.json();
  }

  async listLinks(opts?: {
    fromPath?: string;
    toPath?: string;
    linkType?: string;
    limit?: number;
  }) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/links`);
    if (opts?.fromPath) u.searchParams.set("from_path", opts.fromPath);
    if (opts?.toPath) u.searchParams.set("to_path", opts.toPath);
    if (opts?.linkType) u.searchParams.set("link_type", opts.linkType);
    if (opts?.limit != null) u.searchParams.set("limit", String(opts.limit));
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listLinks failed: ${res.status}`);
    return res.json();
  }

  async registerWebhook(
    url: string,
    opts?: { eventTypes?: string[]; secret?: string },
  ) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/webhooks`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({
        url,
        event_types: opts?.eventTypes ?? [],
        secret: opts?.secret ?? "",
      }),
    });
    if (!res.ok) throw new Error(`registerWebhook failed: ${res.status}`);
    return res.json();
  }

  async listWebhooks(limit = 50) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/webhooks`);
    u.searchParams.set("limit", String(limit));
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`listWebhooks failed: ${res.status}`);
    return res.json();
  }

  async migrateEmbeddings(
    pathPrefix: string,
    opts?: { targetModel?: string; embeddingSpace?: string },
  ) {
    const body: Record<string, unknown> = {
      path_prefix: pathPrefix,
      target_model: opts?.targetModel ?? "",
    };
    if (opts?.embeddingSpace) body.embedding_space = opts.embeddingSpace;
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/embeddings/migrate`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`migrateEmbeddings failed: ${res.status}`);
    return res.json();
  }

  async distilledLineage(distilledId: number) {
    const res = await fetch(
      `${this.baseUrl.replace(/\/$/, "")}/v1/lineage/distilled/${distilledId}`,
      { headers: { "X-API-Key": this.apiKey } },
    );
    if (!res.ok) throw new Error(`distilledLineage failed: ${res.status}`);
    return res.json();
  }

  /** Queue asynchronous distillation for a path prefix (worker via Redis). */
  async refine(pathPrefix: string) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/refine`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-API-Key": this.apiKey },
      body: JSON.stringify({ path_prefix: pathPrefix }),
    });
    if (!res.ok) throw new Error(`refine failed: ${res.status}`);
    return res.json();
  }

  async getStats() {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/stats`, {
      headers: { "X-API-Key": this.apiKey },
    });
    if (!res.ok) throw new Error(`getStats failed: ${res.status}`);
    return res.json();
  }

  async createLink(fromPath: string, toPath: string, linkType = "related") {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories/links`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-API-Key": this.apiKey },
      body: JSON.stringify({ from_path: fromPath, to_path: toPath, link_type: linkType }),
    });
    if (!res.ok) throw new Error(`createLink failed: ${res.status}`);
    return res.json();
  }

  async memoryLineage(path: string) {
    const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/lineage/memory`);
    u.searchParams.set("path", path);
    const res = await fetch(u, { headers: { "X-API-Key": this.apiKey } });
    if (!res.ok) throw new Error(`memoryLineage failed: ${res.status}`);
    return res.json();
  }

  /**
   * Stream PCMI events from GET /v1/events (SSE). Returns an abort function.
   */
  subscribe(
    onEvent: (ev: PCMEvent) => void,
    opts?: { types?: string[]; signal?: AbortSignal },
  ): () => void {
    const controller = new AbortController();
    const outer = opts?.signal;
    if (outer) {
      if (outer.aborted) {
        controller.abort();
      } else {
        outer.addEventListener("abort", () => controller.abort(), { once: true });
      }
    }

    const run = async () => {
      const u = new URL(`${this.baseUrl.replace(/\/$/, "")}/v1/events`);
      if (opts?.types?.length) {
        u.searchParams.set("types", opts.types.join(","));
      }
      const res = await fetch(u, {
        headers: { "X-API-Key": this.apiKey, Accept: "text/event-stream" },
        signal: controller.signal,
      });
      if (!res.ok || !res.body) {
        throw new Error(`subscribe failed: ${res.status}`);
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        buffer = parseSSEChunk(buffer, onEvent);
      }
    };

    run().catch((err) => {
      if (controller.signal.aborted) return;
      throw err;
    });

    return () => controller.abort();
  }
}
