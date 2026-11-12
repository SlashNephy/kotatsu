-- +goose Up
-- create "live_scramble_verdicts" table
CREATE TABLE "live_scramble_verdicts" ("service_id" bigint NOT NULL, "decided_at" timestamptz NOT NULL, PRIMARY KEY ("service_id"));

-- +goose Down
-- reverse: create "live_scramble_verdicts" table
DROP TABLE "live_scramble_verdicts";
