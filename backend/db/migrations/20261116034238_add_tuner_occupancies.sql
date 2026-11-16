-- +goose Up
-- create "tuner_occupancies" table
CREATE TABLE "tuner_occupancies" ("tuner_index" bigint NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, PRIMARY KEY ("tuner_index", "start_at", "end_at"));

-- +goose Down
-- reverse: create "tuner_occupancies" table
DROP TABLE "tuner_occupancies";
