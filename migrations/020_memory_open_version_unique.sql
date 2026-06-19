-- v1.52.0: enforce exactly one open version per (tenant_id, path).
--
-- Closes a data-integrity race in MemoryRepository.Store. The store path
-- soft-closes the current row (UPDATE ... SET valid_to = NOW() WHERE valid_to
-- IS NULL) and then INSERTs the next version. Under the default READ COMMITTED
-- isolation, two concurrent stores to the same path could BOTH end with a row
-- where valid_to IS NULL: the second transaction's UPDATE matches zero rows
-- (the first already closed the row), so it inserts a fresh version=1 alongside
-- the first writer's new open row. The result is two "current" versions with
-- regressed/duplicate version numbers, making GetByPath (... valid_to IS NULL
-- LIMIT 1) and as_of temporal reads non-deterministic.
--
-- The migrate runner wraps each file in a single transaction, so CREATE INDEX
-- CONCURRENTLY cannot be used here; a plain CREATE UNIQUE INDEX takes a brief
-- lock while it builds, which is acceptable for this one-time correctness fix.

-- 1) Heal any pre-existing duplicate open versions left by the race. Keep the
--    highest version open (tie-break on id) and tombstone the rest as zero-width
--    intervals (valid_to = valid_from) so they never surface in as_of reads.
WITH ranked AS (
    SELECT id,
           valid_from,
           ROW_NUMBER() OVER (
               PARTITION BY tenant_id, path
               ORDER BY version DESC, id DESC
           ) AS rn
    FROM memory_entries
    WHERE valid_to IS NULL
)
UPDATE memory_entries m
SET valid_to = m.valid_from
FROM ranked
WHERE m.id = ranked.id
  AND ranked.rn > 1;

-- 2) Enforce the invariant going forward. The losing transaction in a
--    concurrent store now fails with unique_violation (SQLSTATE 23505) on this
--    constraint; MemoryRepository.Store catches it and retries, recomputing the
--    correct next version against the row the winner left open.
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_entries_open_version
    ON memory_entries (tenant_id, path)
    WHERE valid_to IS NULL;
