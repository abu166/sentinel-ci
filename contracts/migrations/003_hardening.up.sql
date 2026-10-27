ALTER TABLE repositories ADD COLUMN IF NOT EXISTS webhook_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS auto_commit BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS open_pr BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS manual_approval BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE pipelines ADD COLUMN IF NOT EXISTS commit_message TEXT NOT NULL DEFAULT '';
ALTER TABLE healing_patches ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'PENDING' CHECK(approval_status IN ('PENDING','APPROVED','REJECTED'));
ALTER TABLE healing_patches ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE healing_patches ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE pipeline_steps ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE pipeline_steps ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;
ALTER TABLE pipeline_steps ADD COLUMN IF NOT EXISTS peak_rss_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE pipeline_steps ADD COLUMN IF NOT EXISTS cpu_user_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE pipeline_steps ADD COLUMN IF NOT EXISTS cpu_system_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE pipeline_steps ADD COLUMN IF NOT EXISTS sandbox_policy JSONB NOT NULL DEFAULT '{}';
CREATE TABLE IF NOT EXISTS outbox_events (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), aggregate_type TEXT NOT NULL,
 aggregate_id UUID NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL,
 idempotency_key TEXT NOT NULL UNIQUE,
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','PUBLISHED','DEAD')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0),
 next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 published_at TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS outbox_pending ON outbox_events(next_retry_at) WHERE status='PENDING';
CREATE TABLE IF NOT EXISTS api_keys (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE, role TEXT NOT NULL CHECK(role IN ('admin','maintainer','viewer')),
 revoked_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS audit_logs (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), actor TEXT NOT NULL, action TEXT NOT NULL,
 resource_type TEXT NOT NULL, resource_id TEXT NOT NULL, details JSONB NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_created ON audit_logs(created_at DESC);
CREATE TABLE IF NOT EXISTS step_executions (
 step_id UUID PRIMARY KEY REFERENCES pipeline_steps(id) ON DELETE CASCADE,
 pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
 owner TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('RUNNING','COMPLETED','FAILED','INTERRUPTED')),
 claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(), finished_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS pull_requests (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), patch_id UUID NOT NULL UNIQUE REFERENCES healing_patches(id),
 pipeline_id UUID NOT NULL REFERENCES pipelines(id), repository_id UUID NOT NULL REFERENCES repositories(id),
 provider TEXT NOT NULL CHECK(provider IN ('local','github','gitlab')), branch TEXT NOT NULL,
 base_branch TEXT NOT NULL, commit_sha TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL,
 diff TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('OPEN','MERGED','CLOSED','LOCAL','BRANCH')),
 url TEXT NOT NULL DEFAULT '', remote_number INTEGER,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS webhook_deliveries (
 delivery_id TEXT PRIMARY KEY, repository_id UUID NOT NULL REFERENCES repositories(id),
 pipeline_id UUID REFERENCES pipelines(id), received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
