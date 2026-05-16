/**
 * Manual SDK smoke (HTTP). From sdk/typescript:
 *   export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
 *   npx tsx smoke.mts
 */
import { PCMIClient } from "./src/client.ts";

const base = process.env.PCMI_BASE_URL ?? "http://localhost:8000";
const key = process.env.PCMI_API_KEY ?? "testkey123";
const path = "root.sdk.ts.smoke";

const client = new PCMIClient(base, key);
await client.store(path, "hello from ts sdk", {}, {
  tags: ["sdk-smoke"],
  embeddingModel: "unspecified",
});
const out = (await client.retrieve(path, "", 5, {
  tags: ["sdk-smoke"],
  tagsMatch: "all",
})) as { total: number };
console.log("retrieve total:", out.total);
const compact = await client.compact(path, { keepSuperseded: 20 });
console.log("compact:", compact);
console.log("done");
