-- +goose Up
-- modify "anime_record_settings" table
ALTER TABLE "anime_record_settings" ADD COLUMN "notify_wishlist" boolean NOT NULL DEFAULT false;
-- modify "watch_reservations" table
ALTER TABLE "watch_reservations" ADD COLUMN "from_wishlist" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "watch_reservations" table
ALTER TABLE "watch_reservations" DROP COLUMN "from_wishlist";
-- reverse: modify "anime_record_settings" table
ALTER TABLE "anime_record_settings" DROP COLUMN "notify_wishlist";
