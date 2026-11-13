-- +goose Up
-- modify "tuner_server_status" table
ALTER TABLE "tuner_server_status" ADD COLUMN "server_kind" text NULL, ADD COLUMN "server_version" text NULL;

-- +goose Down
-- reverse: modify "tuner_server_status" table
ALTER TABLE "tuner_server_status" DROP COLUMN "server_version", DROP COLUMN "server_kind";
