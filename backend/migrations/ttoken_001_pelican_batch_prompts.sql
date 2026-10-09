ALTER TABLE pelican_test_runs
    ADD COLUMN IF NOT EXISTS prompt TEXT NOT NULL DEFAULT 'Generate an SVG of a pelican riding a bicycle',
    ADD COLUMN IF NOT EXISTS output_mode TEXT NOT NULL DEFAULT 'svg',
    ADD COLUMN IF NOT EXISTS batch_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS pelican_test_runs_batch_idx ON pelican_test_runs (batch_id, id);
CREATE INDEX IF NOT EXISTS pelican_test_runs_queue_idx ON pelican_test_runs (id) WHERE status = 'queued';

CREATE TABLE IF NOT EXISTS pelican_prompt_templates (
    id BIGSERIAL PRIMARY KEY,
    admin_user_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    prompt TEXT NOT NULL,
    output_mode TEXT NOT NULL DEFAULT 'svg',
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pelican_template_output_mode CHECK (output_mode IN ('svg', 'text'))
);
CREATE INDEX IF NOT EXISTS pelican_prompt_templates_owner_idx ON pelican_prompt_templates (admin_user_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS pelican_prompt_templates_default_idx ON pelican_prompt_templates (admin_user_id) WHERE is_default;
