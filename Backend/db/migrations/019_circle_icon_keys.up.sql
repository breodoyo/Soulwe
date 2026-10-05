-- circles.icon used to hold emoji, which the frontend rendered as raw text.
-- Emoji render at wildly different sizes and weights per platform and cannot be
-- tinted, so they are replaced with stable slug-like keys. The frontend maps
-- each key to a single icon component; an unknown or NULL key falls back to a
-- generic message icon.

UPDATE circles SET icon = 'grief'         WHERE slug = 'grief';
UPDATE circles SET icon = 'work'          WHERE slug = 'work-pressure';
UPDATE circles SET icon = 'family'        WHERE slug = 'family';
UPDATE circles SET icon = 'relationships' WHERE slug = 'relationships';
UPDATE circles SET icon = 'growth'        WHERE slug = 'young-adult';

-- Catch any row seeded or added outside the known slugs that still carries an
-- emoji: clear it so the frontend renders the generic fallback instead of
-- leaking a glyph. Rows that already hold a key are left untouched.
UPDATE circles
   SET icon = NULL
 WHERE icon IS NOT NULL
   AND icon NOT IN ('grief', 'work', 'family', 'relationships', 'growth');