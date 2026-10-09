CREATE TABLE IF NOT EXISTS pelican_test_runs (
    id BIGSERIAL PRIMARY KEY,
    admin_user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    model TEXT NOT NULL,
    prompt_version TEXT NOT NULL DEFAULT 'simon-2024-v1',
    request_id TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'running',
    response_text TEXT NOT NULL DEFAULT '',
    svg TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS pelican_test_runs_created_at_idx ON pelican_test_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS pelican_test_runs_group_model_idx ON pelican_test_runs (group_id, model, created_at DESC);
