#!/usr/bin/env bash
# Teste ponta a ponta contra o ambiente do docker compose (via gateway).
# Uso: docker compose up -d --build && ./scripts/e2e.sh
# Requer: bash, curl, unzip, docker (o vídeo de teste é gerado com o ffmpeg do container do worker).
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MAILPIT_URL="${MAILPIT_URL:-http://localhost:8025}"
VIDEOS="${VIDEOS:-3}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

json_field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -1; }
fail() { echo "❌ $*" >&2; exit 1; }

echo "▶ aguardando gateway em $BASE_URL"
for _ in $(seq 1 60); do
  curl -fs "$BASE_URL/" >/dev/null && break
  sleep 2
done

echo "▶ gerando vídeo de teste (5s) com o ffmpeg do worker"
docker compose exec -T processor-worker ffmpeg -hide_banner -loglevel error \
  -f lavfi -i testsrc=duration=5:size=320x240:rate=10 -f matroska - > "$TMP/sample.mkv"
[ -s "$TMP/sample.mkv" ] || fail "vídeo de teste não gerado"
head -c 2048 /dev/urandom > "$TMP/corrompido.mp4"

EMAIL="e2e-$(date +%s)-$RANDOM@fiapx.local"
echo "▶ cadastrando $EMAIL"
code=$(curl -s -o "$TMP/reg.json" -w '%{http_code}' -X POST "$BASE_URL/auth/register" \
  -H 'Content-Type: application/json' -d "{\"name\":\"E2E\",\"email\":\"$EMAIL\",\"password\":\"senha-segura\"}")
[ "$code" = 201 ] || fail "register retornou $code: $(cat "$TMP/reg.json")"

code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/videos")
[ "$code" = 401 ] || fail "API sem token deveria retornar 401, retornou $code"

TOKEN=$(curl -fs -X POST "$BASE_URL/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"senha-segura\"}" | json_field access_token)
[ -n "$TOKEN" ] || fail "login sem token"
AUTH=(-H "Authorization: Bearer $TOKEN")

echo "▶ enviando $VIDEOS vídeos válidos em paralelo + 1 corrompido"
pids=()
for i in $(seq 1 "$VIDEOS"); do
  cp "$TMP/sample.mkv" "$TMP/video-$i.mkv"
  curl -fs "${AUTH[@]}" -F "video=@$TMP/video-$i.mkv" "$BASE_URL/api/videos" -o "$TMP/up-$i.json" &
  pids+=($!)
done
curl -fs "${AUTH[@]}" -F "video=@$TMP/corrompido.mp4" "$BASE_URL/api/videos" -o "$TMP/up-bad.json" &
pids+=($!)
for p in "${pids[@]}"; do wait "$p" || fail "upload falhou"; done
BAD_ID=$(json_field id < "$TMP/up-bad.json")

echo "▶ aguardando processamento"
for _ in $(seq 1 90); do
  LIST=$(curl -fs "${AUTH[@]}" "$BASE_URL/api/videos")
  pending=$(grep -o '"status":"\(PENDING\|PROCESSING\)"' <<<"$LIST" | wc -l || true)
  [ "$pending" -eq 0 ] && break
  sleep 2
done
completed=$(grep -o '"status":"COMPLETED"' <<<"$LIST" | wc -l)
failed=$(grep -o '"status":"FAILED"' <<<"$LIST" | wc -l)
echo "  concluídos=$completed falhos=$failed"
[ "$completed" -eq "$VIDEOS" ] || fail "esperava $VIDEOS concluídos: $LIST"
[ "$failed" -eq 1 ] || fail "esperava 1 vídeo com falha: $LIST"

echo "▶ baixando ZIP"
GOOD_ID=$(grep -o '"id":"[^"]*","original_name":"video-1.mkv"' <<<"$LIST" | json_field id)
curl -fs "${AUTH[@]}" "$BASE_URL/api/videos/$GOOD_ID/download" -o "$TMP/frames.zip"
frames=$(unzip -l "$TMP/frames.zip" | grep -c 'frame_.*\.png' || true)
echo "  frames no zip: $frames"
[ "$frames" -ge 4 ] || fail "zip com poucos frames ($frames)"

code=$(curl -s -o /dev/null -w '%{http_code}' "${AUTH[@]}" "$BASE_URL/api/videos/$BAD_ID/download")
[ "$code" = 409 ] || fail "download de vídeo com falha deveria ser 409, foi $code"

echo "▶ verificando e-mail de falha no Mailpit"
for _ in $(seq 1 15); do
  if curl -fs "$MAILPIT_URL/api/v1/search?query=to:$EMAIL" | grep -q '"total":[1-9]'; then
    echo "✅ E2E OK: auth, upload paralelo, processamento, status, download e notificação"
    exit 0
  fi
  sleep 2
done
fail "e-mail de falha não recebido"
