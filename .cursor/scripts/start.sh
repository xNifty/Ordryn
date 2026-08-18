#!/usr/bin/env bash
# Per-boot service reconciliation for the GoTodo (Ordryn) Cloud Agent environment.
# Starts PostgreSQL and Redis (the app's required backing services) and ensures
# the application role and database exist. Idempotent and safe to re-run; returns
# once services are ready. The application server itself runs as a terminal.
set -euo pipefail

PG_VERSION=16
PG_CLUSTER=main
DB_NAME=gotodo
DB_PASSWORD=postgres

echo "==> Starting PostgreSQL cluster ${PG_VERSION}/${PG_CLUSTER}"
if ! pg_isready -h 127.0.0.1 -p 5432 -q 2>/dev/null; then
  sudo pg_ctlcluster "$PG_VERSION" "$PG_CLUSTER" start
fi

echo "==> Waiting for PostgreSQL to accept connections"
for _ in $(seq 1 30); do
  if pg_isready -h 127.0.0.1 -p 5432 -q 2>/dev/null; then
    break
  fi
  sleep 1
done
pg_isready -h 127.0.0.1 -p 5432 -q

echo "==> Ensuring database role and database"
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "ALTER USER postgres PASSWORD '${DB_PASSWORD}';"
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
  sudo -u postgres createdb "$DB_NAME"
  echo "    created database ${DB_NAME}"
else
  echo "    database ${DB_NAME} already exists"
fi

echo "==> Starting Redis"
if ! redis-cli ping >/dev/null 2>&1; then
  sudo redis-server /etc/redis/redis.conf --daemonize yes
  for _ in $(seq 1 15); do
    if redis-cli ping >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
fi
redis-cli ping >/dev/null

echo "==> start.sh complete: PostgreSQL and Redis are ready"
