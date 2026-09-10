-- Give the bridge and its provider accounts stable identities. Runtime code no
-- longer depends on the mutable Chinese display name of the group.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS system_role VARCHAR(32) NOT NULL DEFAULT '';

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS purpose VARCHAR(32) NOT NULL DEFAULT 'general';

-- Pick one deterministic legacy image group. Prefer an active image-enabled
-- group, then the oldest ID, so duplicate display names cannot change routing.
WITH legacy_image_group AS (
    SELECT id
      FROM groups
     WHERE deleted_at IS NULL
       AND name = '生图'
       AND allow_image_generation = TRUE
     ORDER BY (status = 'active') DESC, id ASC
     LIMIT 1
)
UPDATE groups
   SET system_role = 'image_generation'
 WHERE id IN (SELECT id FROM legacy_image_group)
   AND system_role = '';

CREATE UNIQUE INDEX IF NOT EXISTS groups_unique_live_system_role
    ON groups (system_role)
    WHERE deleted_at IS NULL AND system_role <> '';

CREATE INDEX IF NOT EXISTS accounts_purpose_idx ON accounts (purpose);

-- Existing accounts bound to the legacy image group become first-class image
-- providers. Future writes are validated by the service layer.
UPDATE accounts a
   SET purpose = 'image_provider'
 WHERE a.deleted_at IS NULL
   AND a.type = 'apikey'
   AND a.platform IN ('openai', 'gemini', 'grok')
   AND EXISTS (
       SELECT 1
         FROM account_groups ag
         JOIN groups g ON g.id = ag.group_id
        WHERE ag.account_id = a.id
          AND g.deleted_at IS NULL
          AND g.system_role = 'image_generation'
   );

-- Video is a separate media path and must not leak into the still-image bridge.
UPDATE groups g
   SET model_allowlist = jsonb_set(
       g.model_allowlist,
       '{models}',
       COALESCE((
           SELECT jsonb_agg(model ORDER BY ord)
             FROM jsonb_array_elements_text(COALESCE(g.model_allowlist->'models', '[]'::jsonb))
                  WITH ORDINALITY AS listed(model, ord)
            WHERE lower(model) NOT LIKE '%grok-imagine-video%'
              AND lower(model) NOT LIKE '%grok-video%'
       ), '[]'::jsonb),
       TRUE
   )
 WHERE g.deleted_at IS NULL
   AND g.system_role = 'image_generation'
   AND jsonb_typeof(g.model_allowlist->'models') = 'array';

-- Repair the known xAI generation change only when the new public model is
-- actually exposed by a live provider and the old public model is not.
UPDATE groups g
   SET model_allowlist = jsonb_set(
       g.model_allowlist,
       '{models}',
       (
           SELECT jsonb_agg(
               CASE WHEN model = 'grok-imagine-image'
                    THEN 'grok-imagine-image-2.0'
                    ELSE model END
               ORDER BY ord
           )
             FROM jsonb_array_elements_text(g.model_allowlist->'models')
                  WITH ORDINALITY AS listed(model, ord)
       ),
       TRUE
   )
 WHERE g.deleted_at IS NULL
   AND g.system_role = 'image_generation'
   AND g.model_allowlist->'models' ? 'grok-imagine-image'
   AND EXISTS (
       SELECT 1
         FROM account_groups ag
         JOIN accounts a ON a.id = ag.account_id
        WHERE ag.group_id = g.id
          AND a.deleted_at IS NULL
          AND a.status = 'active'
          AND a.schedulable = TRUE
          AND a.platform = 'grok'
          AND COALESCE(a.credentials->'model_mapping', '{}'::jsonb) ? 'grok-imagine-image-2.0'
   )
   AND NOT EXISTS (
       SELECT 1
         FROM account_groups ag
         JOIN accounts a ON a.id = ag.account_id
        WHERE ag.group_id = g.id
          AND a.deleted_at IS NULL
          AND a.status = 'active'
          AND a.schedulable = TRUE
          AND a.platform = 'grok'
          AND COALESCE(a.credentials->'model_mapping', '{}'::jsonb) ? 'grok-imagine-image'
   );

COMMENT ON COLUMN groups.system_role IS
    'Stable internal role for system-managed groups; image_generation identifies the image bridge group';
COMMENT ON COLUMN accounts.purpose IS
    'Operational account purpose: general or image_provider';
