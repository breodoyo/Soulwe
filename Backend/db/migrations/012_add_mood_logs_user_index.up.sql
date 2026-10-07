-- Serves the authenticated per-user queries (WHERE user_id = $1 ORDER BY
-- logged_at DESC): list my moods, latest mood, and the dashboard count.
CREATE INDEX idx_mood_logs_user_logged
    ON mood_logs (user_id, logged_at DESC);