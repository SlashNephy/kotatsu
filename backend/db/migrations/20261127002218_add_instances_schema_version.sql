-- +goose Up
-- modify "instances" table
ALTER TABLE "instances" ADD COLUMN "schema_version" bigint NULL;

-- +goose Down
-- reverse: modify "instances" table
ALTER TABLE "instances" DROP COLUMN "schema_version";
