# LlamaIndex ↔ PCMI

[LlamaIndex](https://docs.llamaindex.ai/) `FunctionTool` wrappers around PCMI **store** and **retrieve**. No embedding index or OpenAI key is required for this sample.

## Setup

```bash
cd examples/llamaindex
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export PCMI_API_KEY=testkey123
export PCMI_BASE_URL=http://localhost:8000
```

```bash
python main.py
```

## Agent integration

Pass `PCMI_TOOLS` from `pcmi_tools.py` into a LlamaIndex agent, workflow, or query engine that accepts tools. HTTP calls go through [`../pcmi_http.py`](../pcmi_http.py).

## Smoke

```bash
python smoke_test.py
PCMI_SMOKE_LIVE=1 python smoke_test.py
```
