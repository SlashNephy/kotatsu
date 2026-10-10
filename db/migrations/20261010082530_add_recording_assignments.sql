-- +goose Up
-- create "recording_assignments" table
CREATE TABLE "recording_assignments" ("id" uuid NOT NULL, "reservation_id" uuid NOT NULL, "priority" integer NOT NULL, "state" text NOT NULL, "generation" bigint NOT NULL, "holder" uuid NULL, "lease_expires_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "recording_assignments_reservation_id_key" UNIQUE ("reservation_id"), CONSTRAINT "recording_assignments_reservation_id_fkey" FOREIGN KEY ("reservation_id") REFERENCES "reservations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "recording_assignments_state_check" CHECK (state = ANY (ARRAY['waiting'::text, 'recording'::text, 'handover'::text, 'finished'::text])));

-- +goose Down
-- reverse: create "recording_assignments" table
DROP TABLE "recording_assignments";
