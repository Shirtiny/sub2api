-- PR 3593 added Grok media routes for image generation, image edits, and video generation.
-- The auto-created default Grok group predates the media-generation gate. Enable
-- only that system default; custom Grok groups must preserve the administrator's
-- explicit disabled setting.
UPDATE groups
SET allow_image_generation = true
WHERE platform = 'grok'
  AND name = 'grok-default'
  AND allow_image_generation = false;
