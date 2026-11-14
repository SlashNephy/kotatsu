-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "external_player_urls_enabled" boolean NOT NULL DEFAULT false, ADD COLUMN "iptv_feeds_enabled" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "iptv_feeds_enabled", DROP COLUMN "external_player_urls_enabled";
