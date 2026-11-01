-- +goose Up
-- create "captures" table
CREATE TABLE "captures" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "recording_id" uuid NULL, "position_ms" bigint NULL, "service_name" text NOT NULL, "program_name" text NULL, "service_name_normalized" text NOT NULL, "program_name_normalized" text NOT NULL, "caption" text NULL, "broadcast_at" timestamptz NOT NULL, "taken_at" timestamptz NOT NULL, "size" bigint NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "captures_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "captures_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "captures_position_ms_check" CHECK (position_ms >= 0));
-- create index "captures_recording_id_idx" to table: "captures"
CREATE INDEX "captures_recording_id_idx" ON "captures" ("recording_id");
-- create index "captures_user_id_created_at_idx" to table: "captures"
CREATE INDEX "captures_user_id_created_at_idx" ON "captures" ("user_id", "created_at" DESC, "id" DESC);

-- +goose Down
-- reverse: create index "captures_user_id_created_at_idx" to table: "captures"
DROP INDEX "captures_user_id_created_at_idx";
-- reverse: create index "captures_recording_id_idx" to table: "captures"
DROP INDEX "captures_recording_id_idx";
-- reverse: create "captures" table
DROP TABLE "captures";
