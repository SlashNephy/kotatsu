-- +goose Up
-- create index "recordings_service_id_event_id_idx" to table: "recordings"
CREATE INDEX "recordings_service_id_event_id_idx" ON "recordings" ("service_id", "event_id");

-- +goose Down
-- reverse: create index "recordings_service_id_event_id_idx" to table: "recordings"
DROP INDEX "recordings_service_id_event_id_idx";
