# Integration testing — note operative

Guida rapida per `go test -tags=integration` su handler, service, repository, grpc, embedding.

---

## Prerequisiti

- **Postgres** con migrazioni applicate (`DATABASE_URL`, es. `postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable`).
- Per test HTTP handler: **Redis** non obbligatorio in host — i test usano **miniredis** in-process.

---

## Comando consigliato (handler + embedding)

```bash
export DATABASE_URL='postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable'

# PCMI_SKIP_SSE_HTTPTEST=1 è il default se usi newIntegrationHTTPApp; esplicito se lanci go test a mano:
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/...

# Con embedding nella stessa run:
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/... ./internal/embedding/...
```

`make ci-like-github` imposta già `PCMI_SKIP_SSE_HTTPTEST=1` nella Phase F (`go test ./internal/...`).

---

## Problema noto: `TestIntegrationHTTP_EventStreamMemoryStored` (SSE + httptest)

### Sintomo

- `go test -tags=integration ./internal/handler/...` **sembra bloccato** per molti minuti senza output.
- Dopo **~10 minuti** fallisce con timeout del **pacchetto** Go (default 10m), stack trace in `miniredis` / `netpoll` / `httptest`.
- Log tipici prima del timeout: `timeout waiting for SSE GET` o `httptest.Server blocked in Close after 5 seconds`.

### Causa

Il test `TestIntegrationHTTP_EventStreamMemoryStored` apre una **GET SSE** su `httptest.Server` che avvolge l’app Fiber tramite `adaptor.FiberApp`. Con **race detector** (e spesso anche senza), la connessione stream può:

1. non completare gli header entro 30s (`startGate` timeout nel test);
2. lasciare `io.Copy` sulla response body **bloccato** dopo `cancel`;
3. far **bloccare** `httptest.Server.Close` fino al timeout del package.

Su **GitHub Actions** il test è già `Skip` quando `GITHUB_ACTIONS=true`. In locale il problema è lo stesso.

### Copertura SSE “vera”

Non si perde copertura funzionale:

- **`scripts/ci_integration_smoke.sh`** — `curl` su `GET /v1/events` + `memory.stored` con server HTTP reale.
- Job CI **`integration-smoke`** (dopo `go build` di API/worker su porta 8000).

---

## Variabili d’ambiente

| Variabile | Default in `newIntegrationHTTPApp` | Effetto |
|-----------|--------------------------------------|---------|
| `PCMI_SKIP_SSE_HTTPTEST` | `1` (se non forzi SSE) | Salta `TestIntegrationHTTP_EventStreamMemoryStored` |
| `PCMI_FORCE_SSE_HTTPTEST` | — | Se `1`, **non** imposta lo skip automatico; esegue il test SSE httptest (può impiegare fino al timeout) |
| `DATABASE_URL` | — | Obbligatoria per test handler/service/repository integration |
| `GITHUB_ACTIONS` | — | Su runner GHA il test SSE httptest è sempre skipped |

### Forzare il test SSE httptest (debug)

```bash
PCMI_FORCE_SSE_HTTPTEST=1 PCMI_SKIP_SSE_HTTPTEST=0 \
  go test -tags=integration -count=1 -timeout 15m -v ./internal/handler/... \
  -run TestIntegrationHTTP_EventStreamMemoryStored
```

Usa un **timeout** esplicito (`-timeout 15m`) e aspettati possibili fallimenti o attese lunghe.

---

## Altri test lenti

- **`go test -race -tags=integration ./internal/...`** — può restare **muto** tra un pacchetto e l’altro per molti minuti; normale su laptop.
- Mitigazioni: `PCMI_GO_TEST_P=1`, `CI_LIKE_NO_RACE=1` (solo script locale `ci_like_github.sh`), `CI_LIKE_GO_VERBOSE=1`, `CI_LIKE_HEARTBEAT_SECS=120`.

Vedi anche [local-ci.md](local-ci.md) e `scripts/ci_like_github.sh --help`.

---

## Riferimenti codice

- Test SSE: `internal/handler/integration_http_e2e_test.go` — `TestIntegrationHTTP_EventStreamMemoryStored`
- Helper HTTP integration: `newIntegrationHTTPApp` nello stesso file
- Smoke CI: `scripts/ci_integration_smoke.sh` (sezione “SSE memory.stored”)
