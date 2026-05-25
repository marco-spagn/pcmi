# PCMI TypeScript SDK

**HTTP** client for [PCMI](https://github.com/marco-spagn/pcmi) (Node and browsers with `fetch`).

## Install

```bash
npm install @marco-spagn/pcmi-sdk
```

From this directory:

```bash
npm ci
npm run smoke
```

## Quick start

```typescript
import { PCMIClient } from "@marco-spagn/pcmi-sdk";

const client = new PCMIClient("http://localhost:8000", process.env.PCMI_API_KEY!);
await client.store("user.note", "hello", { tags: ["demo"] });
const result = await client.retrieve("user.note", { tags: ["demo"] });
```

## Documentation

- [SDK overview](../README.md)
- [HTTP API](../HTTP-API.md)
