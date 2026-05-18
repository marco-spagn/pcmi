package database

import (
	"os"
	"testing"
)

func TestPoolsCloseNilReceiver(t *testing.T) {
	var p *Pools
	p.Close() // must not panic
}

func TestPoolsReadOrPrimaryAndReplica(t *testing.T) {
	if testing.Short() {
		t.Skip("short: requires postgres")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}

	p := NewPools(url, "")
	defer p.Close()
	if p.Primary == nil {
		t.Fatal("primary pool is nil")
	}
	if p.Read != nil {
		t.Fatal("read pool should be nil when replica URL is empty")
	}
	if got := p.ReadOrPrimary(); got != p.Primary {
		t.Fatal("ReadOrPrimary should return Primary when Read is nil")
	}

	p2 := NewPools(url, url)
	defer p2.Close()
	if p2.Read == nil {
		t.Fatal("read pool should be set when replica URL is non-empty")
	}
	if got := p2.ReadOrPrimary(); got != p2.Read {
		t.Fatal("ReadOrPrimary should return Read when configured")
	}
}
