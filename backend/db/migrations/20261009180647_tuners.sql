-- +goose Up
-- create "tuner_server_status" table
CREATE TABLE "tuner_server_status" ("id" boolean NOT NULL DEFAULT true, "degraded_since" timestamptz NULL, "last_error" text NOT NULL, "clock_skew_ms" bigint NULL, "checked_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tuner_server_status_id_check" CHECK (id));
-- create "tuners" table
CREATE TABLE "tuners" ("tuner_index" bigint NOT NULL, "name" text NOT NULL, "types" text[] NOT NULL, "is_available" boolean NOT NULL, "is_remote" boolean NOT NULL, "is_fault" boolean NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("tuner_index"));

-- +goose Down
-- reverse: create "tuners" table
DROP TABLE "tuners";
-- reverse: create "tuner_server_status" table
DROP TABLE "tuner_server_status";
