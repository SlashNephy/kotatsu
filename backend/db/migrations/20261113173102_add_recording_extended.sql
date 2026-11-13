-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "extended" jsonb NOT NULL DEFAULT '{}';
-- 列を足す前の録画は、予約が残っていれば、予約の番組の詳細を写す (#1646)。
UPDATE "recordings" rec SET "extended" = res."extended" FROM "reservations" res WHERE res."id" = rec."reservation_id";

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "extended";
