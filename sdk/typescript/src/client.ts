export type PCMEvent = {
  type: string;
  payload: Record<string, unknown>;
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

  async store(path: string, content: string, metadata: Record<string, unknown> = {}) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/memories`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ path, content, metadata }),
    });
    if (!res.ok) throw new Error(`store failed: ${res.status}`);
    return res.json();
  }

  async retrieve(pathPrefix: string, query = "", limit = 10) {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, "")}/v1/retrieve`, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ path_prefix: pathPrefix, query, limit }),
    });
    if (!res.ok) throw new Error(`retrieve failed: ${res.status}`);
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

  /** @deprecated alias — distillation runs asynchronously in the worker */
  async refine(pathPrefix: string) {
    return this.listDistilled(pathPrefix, 1);
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
