#!/bin/bash

echo "🚀 PCMI v1.1 – FINAL TEST"
echo "=========================="
echo "Data: $(date)"
echo

# 1. Pulizia e avvio fresco
echo "🧹 1. Pulizia completa..."
docker compose down -v --remove-orphans

echo "🏗️  2. Build e avvio..."
docker compose up -d --build

echo "⏳ 3. Attesa avvio servizi (20 secondi)..."
sleep 20

# 4. Health check
echo "🔍 4. Health Check..."
curl -s -f http://localhost:8000/v1/health > /dev/null && echo "✅ API healthy" || { echo "❌ API non raggiungibile"; exit 1; }

# 5. Store 3 ricordi correlati (scalping)
echo "📤 5. Store di 3 ricordi correlati..."
for i in {1..3}; do
  curl -s -X POST http://localhost:8000/v1/memories \
    -H "Content-Type: application/json" \
    -H "X-API-Key: testkey123" \
    -d '{
      "path": "root.test.distill.'$i'",
      "content": "La strategia di scalping funziona meglio con alto volume e RSI sotto 30 nel timeframe di 5 minuti",
      "metadata": {"test": "finale", "topic": "scalping"},
      "embedding_model": "text-embedding-3-large"
    }' | jq -r '.id // "error"'
done

echo

echo "⏳ 6. Aspetto 90 secondi (embedding + distillation)..."
sleep 90

# 7. Retrieve raw
echo "📥 7. Retrieve RAW memories..."
curl -s -X POST http://localhost:8000/v1/retrieve \
  -H "Content-Type: application/json" \
  -H "X-API-Key: testkey123" \
  -d '{"path_prefix":"root.test","limit":10}' | jq '.'

echo

# 8. Retrieve distilled
echo "🧠 8. Retrieve DISTILLED knowledge..."
curl -s -X GET "http://localhost:8000/v1/distilled?path_prefix=root.test" \
  -H "X-API-Key: testkey123" | jq '.'

echo

# 9. Riepilogo DB
echo "📊 9. Riepilogo database..."
docker compose exec postgres psql -U pcmi -d pcmi -c "
SELECT 'raw_memories' as type, COUNT(*) as count FROM memory_entries WHERE path::text LIKE 'root.test.distill%';
SELECT 'distilled_entries' as type, COUNT(*) as count FROM distilled_knowledge;
"

echo
echo "✅ TEST FINALE v1.1 COMPLETATO"
echo "=============================="
echo "Se vedi dati sia in RAW che in DISTILLED → v1.1 è pienamente funzionante!"
