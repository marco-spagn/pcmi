package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// seedMemory is one line of a seed JSONL file (same shape as POST /v1/memories).
type seedMemory struct {
	Path       string         `json:"path"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Importance *float64       `json:"importance,omitempty"`
}

func imp(v float64) *float64 { return &v }

// demoCorpus is a small, domain-neutral dataset for a first run. Paths are
// relative to the --prefix (default root.demo). The pricing entry is stored
// twice so `pcmi get --as-of` can show time travel.
var demoCorpus = []seedMemory{
	{Path: "ops.incidents.api_latency", Content: "API p99 latency spiked to 2.4s after the 14:00 deploy; rolled back the connection-pool change and latency returned to 180ms.", Tags: []string{"incident", "api"}, Importance: imp(0.8)},
	{Path: "ops.incidents.db_failover", Content: "Primary Postgres failed over to the replica at 03:12 UTC; writes paused for 40 seconds, no data loss confirmed by the WAL check.", Tags: []string{"incident", "database"}, Importance: imp(0.9)},
	{Path: "ops.runbooks.cache_flush", Content: "To clear stale sessions, flush the Redis keyspace db 2 only; never FLUSHALL in production because rate-limit counters live in db 0.", Tags: []string{"runbook", "redis"}, Importance: imp(0.7)},
	{Path: "product.decisions.sso", Content: "Decision: enterprise customers authenticate through OIDC single sign-on; API keys remain for service-to-service automation.", Tags: []string{"decision", "security"}, Importance: imp(0.8)},
	{Path: "product.decisions.retention", Content: "Decision: finance records are retained for seven years; scratch notes expire after two weeks.", Tags: []string{"decision", "compliance"}, Importance: imp(0.7)},
	{Path: "product.pricing.plan", Content: "The Team plan costs 20 USD per seat per month.", Tags: []string{"pricing"}, Importance: imp(0.6)},
	{Path: "customers.acme.preferences", Content: "Acme prefers weekly summary emails on Monday mornings and wants escalations routed to their on-call Slack channel.", Tags: []string{"customer"}, Importance: imp(0.5)},
	{Path: "customers.acme.contract", Content: "Acme renewed for 24 months in March with a 99.9% uptime SLA and quarterly business reviews.", Tags: []string{"customer", "contract"}, Importance: imp(0.6)},
	{Path: "customers.globex.feedback", Content: "Globex reported that search misses documents that use synonyms, for example 'invoice' versus 'bill'.", Tags: []string{"customer", "feedback"}, Importance: imp(0.5)},
	{Path: "research.embeddings.dimension", Content: "Switching the embedding model requires re-embedding every memory because vectors from different models are not comparable.", Tags: []string{"research"}, Importance: imp(0.4)},
	{Path: "research.agents.memory", Content: "Agents answer better when they retrieve a few highly relevant memories instead of stuffing the whole history into the prompt.", Tags: []string{"research", "agents"}, Importance: imp(0.4)},
	{Path: "product.pricing.plan", Content: "The Team plan costs 25 USD per seat per month starting in Q3.", Tags: []string{"pricing"}, Importance: imp(0.6)},
}

// readSeedFile parses JSONL seed memories (blank lines and # comments ignored).
func readSeedFile(r io.Reader) ([]seedMemory, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	var out []seedMemory
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		var m seedMemory
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if strings.TrimSpace(m.Path) == "" || m.Content == "" {
			return nil, fmt.Errorf("line %d: path and content are required", line)
		}
		out = append(out, m)
	}
	return out, sc.Err()
}
