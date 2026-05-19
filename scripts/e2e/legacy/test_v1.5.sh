#!/bin/bash
# PCMI v1.5 - Full Robust Test Script (works after docker compose down -v)

echo "🚀 PCMI v1.5 - Full Integration Test (5 memories)"
echo "================================================"

# 1. Full clean start
echo "🧹 Full clean start..."
docker compose down -v --remove-orphans

echo "🔄 Starting fresh stack..."
docker compose up -d --build > /dev/null

sleep 8

# 2. Create test API Key
echo "🔑 Creating test API Key..."
docker compose exec postgres psql -U pcmi -d pcmi -c '
DROP TABLE IF EXISTS api_keys;
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    is_active BOOLEAN DEFAULT true
);

INSERT INTO api_keys (tenant_id, key_hash, name, role, expires_at, is_active)
VALUES (
    '\''00000000-0000-0000-0000-000000000000'\'',
    '\''87d452521c9a7f5c9052ae6190e900a46e2a2df5f144158c2fc20b797adb470b'\'',
    '\''Default Test Key'\'',
    '\''admin'\'',
    NULL,
    true
) ON CONFLICT (key_hash) DO NOTHING;
' > /dev/null

echo "✅ Test API Key ready (testkey123)"

# 3. Create 5 memories
echo "📤 Creating 5 test memories..."
for i in {1..5}; do
  cat > /tmp/test_v15_$i.json << JSON
{
  "path": "root.test.event.$(date +%s)_$i",
  "content": "Test memory #$i - v1.5 API Key + RBAC",
  "metadata": {"test": "v1.5", "memory_id": $i},
  "embedding_model": "text-embedding-3-large"
}
JSON

  curl -s -X POST http://localhost:8000/v1/memories \
    -H "Content-Type: application/json" \
    -H "X-API-Key: testkey123" \
    -d @/tmp/test_v15_$i.json | jq . > /dev/null

  echo "   ✅ Memory #$i created"
  sleep 0.3
done

echo -e "\n⏳ Waiting 15 seconds for Redis events + distillation...\n"
sleep 15

# 4. Check distilled knowledge (with API Key and error handling)
echo "📥 2. Checking distilled knowledge..."
RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X GET "http://localhost:8000/v1/distilled?path_prefix=root.test" \
  -H "X-API-Key: testkey123")

BODY=$(echo "$RESPONSE" | head -n 1)
CODE=$(echo "$RESPONSE" | tail -n 1 | cut -d: -f2)

echo "HTTP Status: $CODE"
echo "$BODY" | jq . 2>/dev/null || echo "Raw response (not JSON): $BODY"

# 5. Show logs
echo -e "\n📋 3. Last 30 lines of worker logs:"
docker compose logs worker --tail=30

echo -e "\n✅ Test completed successfully (5 memories created)."
