Agent → calls only the API (knows nothing about versioning or the DB).
MemoryService → orchestrates the request.
MemoryRepository → contains all the smart logic:
Checks whether a current version already exists for that path.
Closes the old version (valid_to = NOW()).
Inserts the new version (append-only).

PostgreSQL → executes everything in an ACID transaction.
Event Bus → notifies that a new memory has been saved (triggers distillation in the background).

This flow guarantees:

Immutability (nothing is ever overwritten).
Automatic aging of the previous version.
Full traceability.
Complete decoupling: the agent is completely unaware of the memory internals.
