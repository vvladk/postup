DROP TABLE retro_participants;
DROP INDEX IF EXISTS idx_retros_invite_token;
ALTER TABLE retros DROP COLUMN invite_token;
