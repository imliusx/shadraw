#!/usr/bin/env bash
# One-shot deploy script for shadraw.
# Run from the `deploy/` directory after editing `.env`.
#
# Usage:
#   ./deploy.sh           # build & start (or upgrade) all services
#   ./deploy.sh down      # stop & remove containers (volumes kept)
#   ./deploy.sh logs      # tail logs from all services
#   ./deploy.sh ps        # show service status

set -euo pipefail

cd "$(dirname "$0")"

COMPOSE_FILE="docker-compose.prod.yml"
ENV_FILE=".env"

if [ ! -f "$ENV_FILE" ]; then
  echo "❌ $ENV_FILE not found. Copy from .env.prod.example and fill in values:"
  echo "   cp .env.prod.example .env && \$EDITOR .env"
  exit 1
fi

COMPOSE="docker compose -f $COMPOSE_FILE --env-file $ENV_FILE"

case "${1:-up}" in
  up)
    echo "▶︎ Building & starting shadraw stack..."
    $COMPOSE up -d --build --remove-orphans
    echo ""
    echo "✓ Done. Services bound to host:"
    echo "    web  -> 127.0.0.1:3000  (point nginx 'location /'         here)"
    echo "    api  -> 127.0.0.1:8080  (point nginx 'location /api/' and"
    echo "                              'location = /healthz' here)"
    ;;
  down)
    $COMPOSE down --remove-orphans
    ;;
  logs)
    $COMPOSE logs -f --tail=200
    ;;
  ps)
    $COMPOSE ps
    ;;
  *)
    echo "Unknown command: $1"
    echo "Usage: $0 [up|down|logs|ps]"
    exit 1
    ;;
esac
