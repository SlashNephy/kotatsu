-- +goose Up
-- modify "reservations" table
ALTER TABLE "reservations" ADD COLUMN "relayed_from" uuid NULL, ADD CONSTRAINT "reservations_relayed_from_fkey" FOREIGN KEY ("relayed_from") REFERENCES "reservations" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;

-- +goose Down
-- reverse: modify "reservations" table
ALTER TABLE "reservations" DROP CONSTRAINT "reservations_relayed_from_fkey", DROP COLUMN "relayed_from";
