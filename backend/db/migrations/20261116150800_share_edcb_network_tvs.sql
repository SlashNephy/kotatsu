-- +goose Up
-- modify "edcb_network_tvs" table
ALTER TABLE "edcb_network_tvs" DROP COLUMN "instance_id", ADD COLUMN "released_at" timestamptz NULL, ADD COLUMN "closing_at" timestamptz NULL;
-- create "edcb_network_tv_holders" table
CREATE TABLE "edcb_network_tv_holders" ("network_tv_id" integer NOT NULL, "instance_id" uuid NOT NULL, "acquired_at" timestamptz NOT NULL, PRIMARY KEY ("network_tv_id", "instance_id"), CONSTRAINT "edcb_network_tv_holders_network_tv_id_fkey" FOREIGN KEY ("network_tv_id") REFERENCES "edcb_network_tvs" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "edcb_network_tv_holders" table
DROP TABLE "edcb_network_tv_holders";
-- reverse: modify "edcb_network_tvs" table
ALTER TABLE "edcb_network_tvs" DROP COLUMN "closing_at", DROP COLUMN "released_at", ADD COLUMN "instance_id" uuid NOT NULL;
