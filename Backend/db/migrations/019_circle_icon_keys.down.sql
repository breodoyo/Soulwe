-- Restores the emoji this migration replaced, so a rollback returns the seed to
-- its original state.
UPDATE circles SET icon = '🌿' WHERE slug = 'grief';
UPDATE circles SET icon = '💼' WHERE slug = 'work-pressure';
UPDATE circles SET icon = '🏠' WHERE slug = 'family';
UPDATE circles SET icon = '💛' WHERE slug = 'relationships';
UPDATE circles SET icon = '🌱' WHERE slug = 'young-adult';

UPDATE circles
   SET icon = NULL
 WHERE icon IS NOT NULL
   AND icon NOT IN ('🌿', '💼', '🏠', '💛', '🌱');