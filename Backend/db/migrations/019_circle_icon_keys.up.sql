-- circles.icon used to hold emoji; it now holds slug-like keys the frontend maps
-- to icon components, falling back to a generic message icon when unknown.

UPDATE circles SET icon = 'grief'         WHERE slug = 'grief';
UPDATE circles SET icon = 'work'          WHERE slug = 'work-pressure';
UPDATE circles SET icon = 'family'        WHERE slug = 'family';
UPDATE circles SET icon = 'relationships' WHERE slug = 'relationships';
UPDATE circles SET icon = 'growth'        WHERE slug = 'young-adult';

-- Clear any leftover emoji so the generic fallback renders instead.
UPDATE circles
   SET icon = NULL
 WHERE icon IS NOT NULL
   AND icon NOT IN ('grief', 'work', 'family', 'relationships', 'growth');