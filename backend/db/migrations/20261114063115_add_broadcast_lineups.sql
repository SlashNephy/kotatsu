-- +goose Up
-- create "broadcast_lineups" table
CREATE TABLE "broadcast_lineups" ("network_id" integer NOT NULL, "version" smallint NOT NULL, "streams" jsonb NOT NULL, "observed_at" timestamptz NOT NULL, PRIMARY KEY ("network_id"));

-- +goose Down
-- reverse: create "broadcast_lineups" table
DROP TABLE "broadcast_lineups";
