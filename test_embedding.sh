#!/bin/bash

echo "🚀 PCMI - Test Embedding Completo"
echo "=================================="
echo "Data: $(date)"
echo

# 1. Pulizia e avvio fresco
echo "🧹 1. Pulizia container..."
docker compose down -v --remove-orphans

echo "🏗️  2. Build e avvio..."
docker compose up -d --build

echo "⏳ 3. Attesa avvio (15 secondi)..."
sleep 15

# 4. Health check
echo "🔍 4. Health Check..."
curl -s -f http://localhost:8000/v1/health > /dev/null && echo "✅ API healthy" || { echo "❌ API non raggiungibile"; exit 1; }

# 5. Store
echo "📤 5. Store di un ricordo di test..."
cat > /tmp/payload.json << JSON
{
  "tenant_id": "00000000-0000-0000-0000-000000000000",
  "path": "root.test.embedding.deep.$(date +%s)",
  "content": "La strategia di scalping funziona meglio con alto volume e RSI sotto 30 nel timeframe di 5 minuti",
  "metadata": {"test": "embedding-deep", "author": "marco"},
  "embedding_model": "text-embedding-3-large",
  "tags": ["scalping", "trading"]
}
JSON

curl -s -X POST http://localhost:8000/v1/memories \
  -H "Content-Type: application/json" \
  -d @/tmp/payload.json | jq -r '.'

echo

echo "⏳ 6. Aspetto 35 secondi che il worker generi l'embedding..."
sleep 35

# 7. Retrieve
echo "📥 7. Retrieve..."
curl -s -X POST http://localhost:8000/v1/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "00000000-0000-0000-0000-000000000000",
    "path_prefix": "root.test",
    "limit": 8
  }' | jq '.'

echo

# 8. Verifica embedding nel DB
echo "🗄️  8. Verifica embedding nel database..."
docker compose exec postgres psql -U pcmi -d pcmi -c "
SELECT id, 
       path, 
       embedding IS NOT NULL as has_embedding,
       vector_dims(embedding) as embedding_length
FROM memory_entries 
WHERE path::text LIKE 'root.test.embedding.deep%'
ORDER BY id DESC 
LIMIT 5;
"

echo
echo "✅ Test Embedding completato."
echo "Per vedere i log live del worker: docker compose logs -f worker"
