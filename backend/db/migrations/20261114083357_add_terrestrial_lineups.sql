-- +goose Up
-- create "terrestrial_lineups" table
CREATE TABLE "terrestrial_lineups" ("network_id" integer NOT NULL, "transport_stream_id" integer NOT NULL, "version" smallint NOT NULL, "services" jsonb NOT NULL, "observed_at" timestamptz NOT NULL, PRIMARY KEY ("network_id"));

-- +goose Down
-- reverse: create "terrestrial_lineups" table
DROP TABLE "terrestrial_lineups";
