-- +goose Up
-- Tracks the most recent accepted TOTP time-step counter so a captured code
-- cannot be replayed within its validity window (RFC 6238 section 5.2).
ALTER TABLE user_totp ADD COLUMN last_used_counter INTEGER;

-- +goose Down
ALTER TABLE user_totp DROP COLUMN last_used_counter;
