#!/usr/bin/env bash
# Idempotent dependency setup for the GoTodo (Ordryn) Cloud Agent environment.
# Runs once to produce the environment build snapshot: installs PostgreSQL and
# Redis, fetches Go modules, and builds the Vue SPA. No long-running process is
# started here (that belongs in start.sh / terminals).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "==> Installing system packages (PostgreSQL, Redis)"
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -qq
sudo apt-get install -y -qq postgresql redis-server

echo "==> Downloading Go modules"
go mod download

echo "==> Building Vue SPA (web/dist)"
npm run build:web

echo "==> Ensuring repo-root .env exists"
if [ ! -f .env ]; then
  cp .cursor/dev.env .env
  echo "    created .env from .cursor/dev.env"
else
  echo "    .env already present; leaving as-is"
fi

echo "==> install.sh complete"
