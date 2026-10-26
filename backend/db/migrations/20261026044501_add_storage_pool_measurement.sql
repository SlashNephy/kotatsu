-- +goose Up
-- modify "storage_pools" table
ALTER TABLE "storage_pools" ADD COLUMN "total_bytes" bigint NULL, ADD COLUMN "free_bytes" bigint NULL, ADD COLUMN "measured_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "storage_pools" table
ALTER TABLE "storage_pools" DROP COLUMN "measured_at", DROP COLUMN "free_bytes", DROP COLUMN "total_bytes";
