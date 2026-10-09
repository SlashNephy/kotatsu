-- +goose Up
-- modify "reservations" table
ALTER TABLE "reservations" ADD CONSTRAINT "reservations_assignment_state_check" CHECK (assignment_state = ANY (ARRAY['pending'::text, 'ok'::text, 'partial'::text, 'conflict'::text])), ADD COLUMN "assignment_state" text NOT NULL DEFAULT 'pending', ADD COLUMN "assigned_tuner" bigint NULL, ADD COLUMN "planned_start_at" timestamptz NULL, ADD COLUMN "planned_end_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "reservations" table
ALTER TABLE "reservations" DROP CONSTRAINT "reservations_assignment_state_check", DROP COLUMN "planned_end_at", DROP COLUMN "planned_start_at", DROP COLUMN "assigned_tuner", DROP COLUMN "assignment_state";
