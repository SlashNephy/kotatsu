-- +goose Up
-- create "storage_pools" table
CREATE TABLE "storage_pools" ("id" text NOT NULL, "priority" integer NOT NULL, "min_free_bytes" bigint NOT NULL, "low_since" timestamptz NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "storage_pools_min_free_bytes_check" CHECK (min_free_bytes >= 0));

-- +goose Down
-- reverse: create "storage_pools" table
DROP TABLE "storage_pools";
