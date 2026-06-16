# PCMI Python SDK

Async **HTTP** client for [PCMI](https://github.com/marco-spagn/pcmi). For high-throughput store/retrieve, use gRPC or REST directly.

## Requirements

- Python 3.10+

## Install

The [`pcmi`](https://pypi.org/project/pcmi/) package is published to PyPI on each GitHub Release:

```bash
pip install pcmi
```

For local SDK development from this directory:

```bash
pip install -e .
pip install "git+https://github.com/marco-spagn/pcmi.git#subdirectory=sdk/python"
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
