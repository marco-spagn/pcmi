# Protezione branch `main` e badge CI

Il workflow CI può aggiornare **`badges/coverage.json`** su `main` dopo un merge. Gli umani devono passare da **pull request** con review.

## Configurazione semplice (5 minuti) — consigliata oggi

Per **`marco-spagn/pcmi`** (repo **personale**): non serve account bot né secret extra **se** il ruleset resta **non attivo**. Il badge usa già `GITHUB_TOKEN` nel workflow.

### Configurazione applicata (API, 2026-05-25)

1. Ruleset **`main_block`**: **Disabilitato**; eccezioni utente **rimosse** (nessun bypass per `marco-spagn`).
2. Protezione **classica** su `main`: **Require a pull request** (1 approval, code owners).
3. **Allow GitHub Actions to bypass required pull requests**: tentato via API (`bypass_pull_request_allowances` → `github-actions`) → **`422`** *Only organization repositories can have users and team restrictions* — su repo personale non si può impostare da API né dalla UI classica come su un’org.

**Cosa fare tu:** apri sempre una **Pull request** verso `main` (il tuo account non bypassa più il ruleset). Per il badge dopo merge: se il push con `GITHUB_TOKEN` fallisce (GH013), usa il percorso bot + `BADGE_UPDATE_TOKEN` descritto sotto.

**Risultato:** gli umani sono vincolati dalla protezione classica; il ruleset resta disattivo per evitare doppie regole. Il badge senza PAT può ancora fallire finché non c’è bypass Actions o token dedicato.

### Quando vorrai protezione «vera» (Active)

Su repo personale **non** puoi mettere GitHub Actions in eccezione. Percorso supportato (una volta sola, ~10 min):

1. Crea un utente dedicato (es. `pcmi-badge-bot`), invitalo al repo con **Write**.
2. Crea un token (Contents: read/write) → secret repo **`BADGE_UPDATE_TOKEN`** (il workflow lo usa già).
3. In **`main_block`**: eccezione **Utente** = nome del bot → **Consenti sempre**; **rimuovi** `marco-spagn` dall’eccezione; poi **Stato** → **Attivo** → **Salva**.

Dettaglio API e alternative: sezioni sotto.

---

## Stato verificato (`marco-spagn/pcmi`)

| Elemento | Valore |
|----------|--------|
| Ruleset **`main_block`** ([apri](https://github.com/marco-spagn/pcmi/rules/16786925)) | `enforcement`: **disabled** |
| Eccezioni ruleset | **Nessuna** (`bypass_actors`: `[]`; `marco-spagn` rimosso) |
| Regole (quando Active) | PR con 1 review + code owners; no delete / no force push |
| Protezione **classica** su `main` | **Attiva**: PR obbligatoria, 1 approval, **code owners** (`.github/CODEOWNERS`) |
| Bypass PR per GitHub Actions (classica) | **Non applicabile** su repo personale (API `422`) |

## Perché «GitHub Actions» non si può aggiungere

Messaggio API tipico:

```text
422 — Actor GitHub Actions integration must be part of the ruleset source or owner organization
```

Su **repo personale** né la UI del ruleset né la protezione classica permettono eccezioni per app/utenti/team come su un’**organizzazione**. La checkbox «Allow GitHub Actions…» della protezione classica **non** equivale al bypass del ruleset e, con review obbligatorie, il push diretto del badge con `GITHUB_TOKEN` **fallisce** comunque.

**Tentativo Path A (classica via API):** protezione con review applicabile; `bypass_pull_request_allowances` con `github-actions` → **422** («Only organization repositories…»). Non usare solo protezione classica Active senza `BADGE_UPDATE_TOKEN`.

## Percorsi non consigliati qui

| Percorso | Motivo |
|----------|--------|
| **B** — PR automatica badge (`create-pull-request`) | Una PR per ogni aggiornamento badge; non richiesto |
| **C** — solo testo «lascia Disabled» | È già la configurazione semplice sopra |
| Ruleset **Active** + solo `GITHUB_TOKEN` | Push badge bloccato (GH013) |

## Badge CI (riferimento tecnico)

Dopo test su push a `main`, [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) committa solo **`badges/coverage.json`** se cambiato. Messaggio con **`[skip ci]`**; `paths-ignore: badges/**` evita loop CI.

Token step: `secrets.BADGE_UPDATE_TOKEN || github.token` con `contents: write` sul job `go`.

## Esempio API ruleset (solo bypass **User** bot)

Sostituisci `BOT_USER_ID` con l’id numerico del bot:

```bash
gh api repos/marco-spagn/pcmi/rulesets/16786925 -X PUT --input - <<'EOF'
{
  "name": "main_block",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] }
  },
  "bypass_actors": [
    {
      "actor_id": BOT_USER_ID,
      "actor_type": "User",
      "bypass_mode": "always"
    }
  ],
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "creation" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": true,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false,
        "allowed_merge_methods": ["merge", "squash", "rebase"]
      }
    }
  ]
}
EOF
```

**Non** usare `actor_id: 15368` (integrazione GitHub Actions): rifiutata su questo repo.

## Protezione classica (solo org o senza bypass Actions)

Se in futuro il repo fosse in un’**organizzazione**, in **Impostazioni → Branches → Add rule** su `main`:

1. **Require a pull request before merging** → 1 approval, **Require review from Code Owners**.
2. **Allow specified actors to bypass required pull requests** → aggiungi l’app **github-actions** *(solo se la UI lo mostra)*.
3. Salva e **disattiva** o elimina il ruleset `main_block` per evitare doppie regole.

Su repo personale oggi: usa la **configurazione semplice** in cima.

## Vedi anche

- [local-ci.md](local-ci.md)
- [CONTRIBUTING.md](../CONTRIBUTING.md)
