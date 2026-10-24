-- +goose Up
-- create "notification_preferences" table
CREATE TABLE "notification_preferences" ("user_id" uuid NOT NULL, "kind" text NOT NULL, "in_app" boolean NOT NULL, "web_push" boolean NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "kind"), CONSTRAINT "notification_preferences_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text])));
-- create "notification_events" table
CREATE TABLE "notification_events" ("id" uuid NOT NULL, "kind" text NOT NULL, "occurred_at" timestamptz NOT NULL, "payload" jsonb NOT NULL, "dispatched_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text])));
-- create index "notification_events_pending_idx" to table: "notification_events"
CREATE INDEX "notification_events_pending_idx" ON "notification_events" ("occurred_at") WHERE (dispatched_at IS NULL);
-- create "notifications" table
CREATE TABLE "notifications" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "event_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "read_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "notifications_user_id_event_id_key" UNIQUE ("user_id", "event_id"), CONSTRAINT "notifications_event_id_fkey" FOREIGN KEY ("event_id") REFERENCES "notification_events" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "notifications_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "notifications_user_id_created_at_idx" to table: "notifications"
CREATE INDEX "notifications_user_id_created_at_idx" ON "notifications" ("user_id", "created_at" DESC, "id" DESC);

-- +goose Down
-- reverse: create index "notifications_user_id_created_at_idx" to table: "notifications"
DROP INDEX "notifications_user_id_created_at_idx";
-- reverse: create "notifications" table
DROP TABLE "notifications";
-- reverse: create index "notification_events_pending_idx" to table: "notification_events"
DROP INDEX "notification_events_pending_idx";
-- reverse: create "notification_events" table
DROP TABLE "notification_events";
-- reverse: create "notification_preferences" table
DROP TABLE "notification_preferences";
