export class PCMIClient {
  constructor(private baseUrl: string, private apiKey: string) {}

  async store(path: string, content: string, metadata: any = {}) {
    const res = await fetch(`${this.baseUrl}/v1/memories`, {
      method: "POST",
      headers: { "X-API-Key": this.apiKey, "Content-Type": "application/json" },
      body: JSON.stringify({ path, content, metadata })
    });
    return res.json();
  }

  async retrieve(pathPrefix: string, query: string, limit = 10) {
    const res = await fetch(`${this.baseUrl}/v1/retrieve`, {
      method: "POST",
      headers: { "X-API-Key": this.apiKey, "Content-Type": "application/json" },
      body: JSON.stringify({ path_prefix: pathPrefix, query, limit })
    });
    return res.json();
  }
}