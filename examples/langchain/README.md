# LangChain ↔ PCMI

Minimal [LangChain](https://python.langchain.com/) tools (`langchain-core`) that call the PCMI HTTP API for **store**, **retrieve**, and **session working memory**. No LLM is required to run this sample.

## Setup

```bash
cd examples/langchain
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export PCMI_API_KEY=testkey123
export PCMI_BASE_URL=http://localhost:8000
```

Start PCMI (`make infra-up` from the repo root) then:

```bash
python main.py
```

## Wiring into an agent

Import `PCMI_TOOLS` from `pcmi_tools.py` and pass them to any LangChain agent or graph that accepts tools. The tools use synchronous HTTP via shared [`../pcmi_http.py`](../pcmi_http.py).

## Smoke

```bash
python smoke_test.py              # structural (no API)
PCMI_SMOKE_LIVE=1 python smoke_test.py  # requires live API + PCMI_API_KEY
```

From the repo root: `make examples-smoke-structural` or `make examples-smoke` (brings up Docker infra).
