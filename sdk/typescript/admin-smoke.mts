/**
 * Admin SDK smoke (read-only). Requires admin API key.
 *   npm run admin-smoke
 */
import { PCMIClient } from "./src/client.ts";

const base = process.env.PCMI_BASE_URL ?? "http://localhost:8000";
const key = process.env.PCMI_API_KEY ?? "testkey123";
const client = new PCMIClient(base, key);

const tenants = (await client.listTenants(5)) as { total?: number };
console.log("tenants total:", tenants.total ?? 0);
const apiKeys = (await client.listApiKeys({ limit: 5 })) as { total?: number };
console.log("api_keys total:", apiKeys.total ?? 0);
console.log("admin smoke done");
