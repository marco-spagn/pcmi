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

  /** Reserved for future /v1/events stream */
  subscribe(_onEvent: (ev: unknown) => void): never {
    throw new Error("PCMI subscribe() is not implemented yet");
  }
}
