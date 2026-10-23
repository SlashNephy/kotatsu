-- +goose Up
-- modify "reservations" table
ALTER TABLE "reservations" ADD COLUMN "relayed_from" uuid NULL, ADD CONSTRAINT "reservations_relayed_from_fkey" FOREIGN KEY ("relayed_from") REFERENCES "reservations" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "reservations_relayed_from_idx" to table: "reservations"
CREATE INDEX "reservations_relayed_from_idx" ON "reservations" ("relayed_from") WHERE (relayed_from IS NOT NULL);

-- +goose Down
-- reverse: create index "reservations_relayed_from_idx" to table: "reservations"
DROP INDEX "reservations_relayed_from_idx";
-- reverse: modify "reservations" table
ALTER TABLE "reservations" DROP CONSTRAINT "reservations_relayed_from_fkey", DROP COLUMN "relayed_from";
