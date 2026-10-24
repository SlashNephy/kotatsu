-- +goose Up
-- create "schema_compatibility" table
CREATE TABLE "schema_compatibility" ("id" boolean NOT NULL DEFAULT true, "min_binary_version" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "schema_compatibility_id_check" CHECK (id));

-- +goose Down
-- reverse: create "schema_compatibility" table
DROP TABLE "schema_compatibility";
