package database

import "github.com/jackc/pgx/v5/pgxpool"

// Pools holds the primary PostgreSQL pool and an optional read-replica pool.
// Writes and session setup (tenant context) always use Primary.
// When Read is nil, callers should use Primary for queries as well.
type Pools struct {
	Primary *pgxpool.Pool
	Read    *pgxpool.Pool
}

// NewPools opens Primary; if readReplicaURL is non-empty, opens a second pool for read-heavy paths.
func NewPools(primaryURL, readReplicaURL string) *Pools {
	p := &Pools{Primary: New(primaryURL)}
	if readReplicaURL != "" {
		p.Read = New(readReplicaURL)
	}
	return p
}

// ReadOrPrimary returns the replica pool when configured, otherwise the primary.
func (p *Pools) ReadOrPrimary() *pgxpool.Pool {
	if p.Read != nil {
		return p.Read
	}
	return p.Primary
}

// Close closes all non-nil pools.
func (p *Pools) Close() {
	if p == nil {
		return
	}
	p.Primary.Close()
	if p.Read != nil {
		p.Read.Close()
	}
}
