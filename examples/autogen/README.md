# AutoGen ↔ PCMI

[AutoGen](https://microsoft.github.io/autogen/) `autogen_core.tools.FunctionTool` definitions that call PCMI **store** and **retrieve**. This sample runs the tools directly; wiring them into `AssistantAgent` only requires an LLM API key for the agent itself.

## Setup

```bash
cd examples/autogen
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export PCMI_API_KEY=testkey123
export PCMI_BASE_URL=http://localhost:8000
```

```bash
python main.py
```

## Agent integration

```python
from pcmi_tools import build_pcmi_tools

tools = build_pcmi_tools()
# Pass tools=tools when constructing an AssistantAgent (plus your model client).
```

## Smoke

```bash
python smoke_test.py
PCMI_SMOKE_LIVE=1 python smoke_test.py
```
