-- +goose Up
-- create "service_logos" table
CREATE TABLE "service_logos" ("network_id" bigint NOT NULL, "logo_id" bigint NOT NULL, "logo_type" smallint NOT NULL, "version" bigint NOT NULL, "data" bytea NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("network_id", "logo_id", "logo_type"));

-- +goose Down
-- reverse: create "service_logos" table
DROP TABLE "service_logos";
