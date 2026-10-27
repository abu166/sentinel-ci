ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS headers JSONB NOT NULL DEFAULT '{}';
CREATE TABLE IF NOT EXISTS patch_candidates (
 id UUID PRIMARY KEY,
 patch_id UUID NOT NULL REFERENCES healing_patches(id) ON DELETE CASCADE,
 rank INTEGER NOT NULL CHECK(rank>=1 AND rank<=3),
 category TEXT NOT NULL, variant TEXT NOT NULL, file_path TEXT NOT NULL,
 original_code TEXT NOT NULL, patched_code TEXT NOT NULL, unified_diff TEXT NOT NULL,
 ast_cost INTEGER NOT NULL CHECK(ast_cost>=0), test_success BOOLEAN NOT NULL,
 score DOUBLE PRECISION NOT NULL, test_output TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('VERIFIED','REJECTED')),
 selected BOOLEAN NOT NULL DEFAULT false,
 diagnostics JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(patch_id,rank)
);
CREATE UNIQUE INDEX IF NOT EXISTS patch_candidates_selected ON patch_candidates(patch_id) WHERE selected;
ALTER TABLE healing_patches ADD COLUMN IF NOT EXISTS selected_candidate_id UUID REFERENCES patch_candidates(id);
