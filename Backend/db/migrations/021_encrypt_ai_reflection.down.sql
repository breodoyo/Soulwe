ALTER TABLE journal_entries
    DROP COLUMN IF EXISTS ai_reflection_iv,
    DROP COLUMN IF EXISTS ai_reflection_enc;
