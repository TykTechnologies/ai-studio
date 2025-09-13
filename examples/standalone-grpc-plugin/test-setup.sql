-- Test setup for standalone gRPC plugin
-- This demonstrates how to configure an external gRPC plugin with the microgateway

-- 1. Add the external gRPC plugin
INSERT INTO plugins (name, slug, description, command, hook_type, is_active, config) VALUES
('external-message-modifier',
 'external-message-modifier',
 'External gRPC message modifier plugin running as microservice',
 'grpc://localhost:9001',
 'pre_auth',
 true,
 '{"instruction": "Add sparkles! ✨"}');

-- 2. Get the plugin ID (assuming it's the last inserted plugin)
SELECT id, name, command, hook_type FROM plugins WHERE slug = 'external-message-modifier';

-- 3. Associate the plugin with an LLM (replace 1 with your actual LLM ID)
-- First check what LLMs are available:
SELECT id, name, slug, vendor FROM llms WHERE is_active = true LIMIT 5;

-- Then associate the plugin (replace LLM_ID_HERE with actual LLM ID)
-- INSERT INTO llm_plugins (llm_id, plugin_id)
-- VALUES (LLM_ID_HERE, (SELECT id FROM plugins WHERE slug = 'external-message-modifier'));

-- 4. Verify the association
-- SELECT
--   l.id as llm_id,
--   l.name as llm_name,
--   p.id as plugin_id,
--   p.name as plugin_name,
--   p.command,
--   p.hook_type
-- FROM llms l
-- JOIN llm_plugins lp ON l.id = lp.llm_id
-- JOIN plugins p ON lp.plugin_id = p.id
-- WHERE l.id = LLM_ID_HERE;