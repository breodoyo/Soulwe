-- Phase 4: mood check-ins. The original mood_logs table (004) ships without
-- any index, but Phase 4 adds authenticated per-user queries
-- (WHERE user_id = $1 ORDER BY logged_at DESC). This composite index keeps
-- "list my moods", "latest mood", and the dashboard's count query
-- efficient and mirrors the existing journal_entries pattern
-- (idx_journal_entries_user_created).
CREATE INDEX idx_mood_logs_user_logged
    ON mood_logs (user_id, logged_at DESC);