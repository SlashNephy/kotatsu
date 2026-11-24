-- +goose Up
-- modify "services" table
ALTER TABLE "services" ADD COLUMN "gone_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "services" table
ALTER TABLE "services" DROP COLUMN "gone_at";
