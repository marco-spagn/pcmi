# PCMI TypeScript SDK

**HTTP** client for [PCMI](https://github.com/marco-spagn/pcmi) (Node and browsers with `fetch`).

## Install

The `@marco-spagn/pcmi-sdk` package is published to npm on each GitHub Release. Until the first publish, install from this directory:

```bash
npm ci
npm run build
```

After publish:

```bash
npm install @marco-spagn/pcmi-sdk
```

Smoke tests (local checkout):

```bash
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
