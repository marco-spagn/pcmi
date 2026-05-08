Agent → chiama solo l’API (non sa nulla di versioning o DB).
MemoryService → orchestra la richiesta.
MemoryRepository → contiene tutta la logica intelligente:
Controlla se esiste già una versione corrente per quel path.
Chiude la versione vecchia (valid_to = NOW()).
Inserisce la nuova versione (append-only).

PostgreSQL → esegue tutto in transazione ACID.
Event Bus → notifica che è stato salvato un nuovo ricordo (triggera la distillation in background).

Questo flusso garantisce:

Immutabilità (niente viene mai sovrascritto).
Invecchiamento automatico della versione precedente.
Tracciabilità totale.
Decoupling completo: l’agente è completamente stupido rispetto alla memoria.