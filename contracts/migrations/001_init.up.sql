CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE repositories (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT NOT NULL UNIQUE,
 repo_url TEXT NOT NULL, default_branch TEXT NOT NULL DEFAULT 'main', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE pipelines (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), repo_id UUID NOT NULL REFERENCES repositories(id),
 commit_sha TEXT NOT NULL, branch TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING'
 CHECK (status IN ('PENDING','RUNNING','FAILED','HEALING','HEALED','PASSED')),
 triggered_by TEXT NOT NULL DEFAULT 'api', started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ
);
CREATE TABLE pipeline_steps (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
 name TEXT NOT NULL, stage TEXT NOT NULL, command TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING'
 CHECK (status IN ('PENDING','RUNNING','FAILED','HEALING','HEALED','PASSED','SKIPPED')),
 exit_code INTEGER, duration_ms BIGINT CHECK(duration_ms >= 0), depends_on TEXT[] NOT NULL DEFAULT '{}'
);
CREATE TABLE log_chunks (
 id BIGSERIAL PRIMARY KEY, step_id UUID NOT NULL REFERENCES pipeline_steps(id) ON DELETE CASCADE,
 sequence_num BIGINT NOT NULL, stream TEXT NOT NULL CHECK(stream IN ('stdout','stderr')),
 content TEXT NOT NULL, timestamp TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(step_id,sequence_num)
);
CREATE TABLE healing_patches (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
 step_id UUID NOT NULL REFERENCES pipeline_steps(id), file_path TEXT NOT NULL, bug_category TEXT NOT NULL,
 root_cause_analysis TEXT NOT NULL, ast_node_summary TEXT NOT NULL, original_code TEXT NOT NULL,
 patched_code TEXT NOT NULL, unified_diff TEXT NOT NULL,
 verification_status TEXT NOT NULL DEFAULT 'PENDING' CHECK(verification_status IN ('PENDING','VERIFIED','REJECTED')),
 confidence_score DOUBLE PRECISION NOT NULL CHECK(confidence_score BETWEEN 0 AND 1),
 test_output TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX pipelines_repo_status ON pipelines(repo_id,status);
CREATE INDEX pipeline_steps_pipeline ON pipeline_steps(pipeline_id);
CREATE INDEX log_chunks_order ON log_chunks(step_id,sequence_num);
CREATE INDEX healing_patches_pipeline ON healing_patches(pipeline_id);
CREATE TABLE job_snapshots (
 pipeline_id UUID PRIMARY KEY REFERENCES pipelines(id) ON DELETE CASCADE,
 payload JSONB NOT NULL
);
