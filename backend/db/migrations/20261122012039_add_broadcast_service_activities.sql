-- +goose Up
-- create "broadcast_service_activities" table
CREATE TABLE "broadcast_service_activities" ("network_id" integer NOT NULL, "service_id" integer NOT NULL, "transport_stream_id" integer NOT NULL, "has_events" boolean NOT NULL, "observed_at" timestamptz NOT NULL, PRIMARY KEY ("network_id", "service_id"));

-- +goose Down
-- reverse: create "broadcast_service_activities" table
DROP TABLE "broadcast_service_activities";
