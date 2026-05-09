#!/bin/bash

echo "🚀 PCMI Test Automation Script v1.1 - Versione Stabile"
echo "===================================================="
echo "Data: $(date)"
echo

echo "🧹 1. Pulizia completa dei container..."
docker compose down -v --remove-orphans

echo "🏗️  2. Build e avvio fresco..."
docker compose up -d --build

echo "⏳ 3. Attesa avvio servizi (15 secondi)..."
sleep 15

echo "🔍 4. Health Check..."
curl -s -f http://localhost:8000/v1/health > /dev/null && echo "✅ API healthy" || { echo "❌ API non raggiungibile"; exit 1; }

echo "📤 5. Store di un nuovo ricordo di test..."
cat > /tmp/pcmi_test_payload.json << JSON
{
  "tenant_id": "00000000-0000-0000-0000-000000000000",
  "path": "root.test.autotest.$(date +%s)",
  "content": "Test automatico generato da script - verifica embedding + retrieve",
  "metadata": {"author": "marco", "test": "auto"},
  "embedding_model": "text-embedding-3-large",
  "tags": ["auto-test"]
}
JSON

curl -s -X POST http://localhost:8000/v1/memories \
  -H "Content-Type: application/json" \
  -d @/tmp/pcmi_test_payload.json | jq -r '.'

echo

echo "⏳ 6. Aspetto 30 secondi che il worker generi l'embedding..."
sleep 30

echo "📥 7. Retrieve con path_prefix root.test..."
curl -s -X POST http://localhost:8000/v1/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "00000000-0000-0000-0000-000000000000",
    "path_prefix": "root.test",
    "limit": 10
  }' | jq '.'

echo

echo "📋 8. Ultimi log del Embedding Worker:"
docker compose logs -f worker --tail=20

echo
echo "✅ Test completati."
echo "Per vedere i log live del worker: docker compose logs -f worker"
