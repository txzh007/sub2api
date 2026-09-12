-- Allow Composite routes to target a complete client protocol family instead
-- of requiring one row per concrete endpoint. Existing endpoint-specific rows
-- remain valid and take precedence in the resolver.
ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_endpoint_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_endpoint_check
    CHECK (endpoint IN ('any', 'openai', 'anthropic', 'messages', 'count_tokens',
                        'responses', 'chat_completions', 'embeddings', 'images', 'gemini'));
