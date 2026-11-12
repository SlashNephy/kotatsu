-- +goose Up
-- modify "reservations" table
ALTER TABLE "reservations" ADD COLUMN "assignment_peers" uuid[] NOT NULL DEFAULT '{}';

-- +goose Down
-- reverse: modify "reservations" table
ALTER TABLE "reservations" DROP COLUMN "assignment_peers";
