ALTER TABLE retros ADD COLUMN invite_token TEXT;
CREATE UNIQUE INDEX idx_retros_invite_token ON retros(invite_token) WHERE invite_token IS NOT NULL;
