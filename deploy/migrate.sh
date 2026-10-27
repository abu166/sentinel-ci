#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -c 'CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now());'
# The Postgres image has already run the immutable phase-one initialization scripts.
psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -c "INSERT INTO schema_migrations(version) SELECT v FROM (VALUES ('001_init.up.sql'),('002_seed.up.sql')) AS versions(v) WHERE to_regclass('public.pipelines') IS NOT NULL ON CONFLICT DO NOTHING;"
for migration in /migrations/*.up.sql; do
 version="$(basename "$migration")"
 applied="$(psql -At "$DATABASE_URL" -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM schema_migrations WHERE version='$version'")"
 if [ "$applied" = 0 ]; then
  psql -v ON_ERROR_STOP=1 --single-transaction "$DATABASE_URL" -f "$migration" -c "INSERT INTO schema_migrations(version) VALUES('$version') ON CONFLICT DO NOTHING;"
 fi
done
