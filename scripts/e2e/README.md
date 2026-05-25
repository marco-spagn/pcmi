# E2E and manual test scripts

| Script | Purpose |
|--------|---------|
| [`test_pcmi.sh`](test_pcmi.sh) | Docker compose smoke: store + retrieve (used by CI when OpenAI E2E runs) |

**Distillation E2E (recommended):**

```bash
make distillation-e2e PRESET=finance NUM=500 SEED=42
# or
./scripts/distill_e2e.sh --preset soc --num 1000 --seed 42
```

See [docs/distillation-tests.md](../../docs/distillation-tests.md) and [../pcmi_synth/README.md](../pcmi_synth/README.md).
