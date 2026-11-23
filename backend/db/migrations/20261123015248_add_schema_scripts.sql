-- +goose Up
-- modify "schema_compatibility" table
ALTER TABLE "schema_compatibility" ADD COLUMN "schema_version" bigint NOT NULL DEFAULT 0;
-- create "schema_scripts" table
CREATE TABLE "schema_scripts" ("version" bigint NOT NULL, "applied_at" timestamptz NOT NULL, PRIMARY KEY ("version"));

-- +goose Down
-- reverse: create "schema_scripts" table
DROP TABLE "schema_scripts";
-- reverse: modify "schema_compatibility" table
ALTER TABLE "schema_compatibility" DROP COLUMN "schema_version";
