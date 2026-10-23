-- AI reflections can quote or paraphrase the journal entry, so they get the
-- same AES-256-GCM protection as content_enc/content_iv. New writes seal the
-- reflection into ai_reflection_enc with a fresh ai_reflection_iv and leave
-- the legacy ai_reflection column NULL.
--
-- Backward compatibility: rows written before this migration still hold
-- plaintext in ai_reflection and are returned as-is. Once the old values are
-- no longer needed, a deployment can clear them:
--   UPDATE journal_entries SET ai_reflection = NULL WHERE ai_reflection IS NOT NULL;
-- That step is irreversible, so backfill first if the legacy text must survive.
ALTER TABLE journal_entries
    ADD COLUMN ai_reflection_enc BYTEA,
    ADD COLUMN ai_reflection_iv BYTEA;
