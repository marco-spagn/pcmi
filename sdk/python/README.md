# PCMI Python SDK

Async **HTTP** client for [PCMI](https://github.com/marco-spagn/pcmi). For high-throughput store/retrieve, use gRPC or REST directly.

## Requirements

- Python 3.10+

## Install

```bash
pip install pcmi
```

From this directory (editable, for development):

```bash
pip install -e .
```

## Quick start

```bash
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
python smoke.py
```

```python
import asyncio
from pcmi import PCMIClient

async def main() -> None:
    async with PCMIClient("http://localhost:8000", "your-api-key") as client:
        await client.store("user.note", "hello", tags=["demo"])
        result = await client.retrieve("user.note", tags=["demo"])
        print(result)

asyncio.run(main())
```

## Documentation

- [SDK overview](../README.md)
- [HTTP API](../HTTP-API.md)
