# CrewAI ↔ PCMI

[CrewAI](https://docs.crewai.com/) `@tool` functions for PCMI **store** and **retrieve**. The demo calls `.run()` on each tool without instantiating a full Crew (no OpenAI key needed for this path).

## Setup

```bash
cd examples/crewai
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export PCMI_API_KEY=testkey123
export PCMI_BASE_URL=http://localhost:8000
```

```bash
python main.py
```

## Crew integration

Attach `PCMI_TOOLS` from `pcmi_tools.py` to agents in your Crew definition. Shared HTTP helpers live in [`../pcmi_http.py`](../pcmi_http.py).

## Smoke

```bash
python smoke_test.py
PCMI_SMOKE_LIVE=1 python smoke_test.py
```
