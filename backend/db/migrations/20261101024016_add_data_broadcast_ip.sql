-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "data_broadcast_ip_enabled" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "data_broadcast_ip_enabled";
