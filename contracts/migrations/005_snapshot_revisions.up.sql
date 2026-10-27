UPDATE pipelines SET commit_sha=left(commit_sha,40) WHERE commit_sha ~ '^[0-9a-f]{64}$';
UPDATE job_snapshots j SET payload=jsonb_set(j.payload,'{commit_sha}',to_jsonb(p.commit_sha)) FROM pipelines p WHERE j.pipeline_id=p.id;
