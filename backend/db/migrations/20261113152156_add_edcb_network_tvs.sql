-- +goose Up
-- create "edcb_network_tvs" table
CREATE TABLE "edcb_network_tvs" ("id" integer NOT NULL, "instance_id" uuid NOT NULL, "network_id" integer NOT NULL, "service_id" integer NOT NULL, "acquired_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "edcb_network_tvs_id_check" CHECK (id > 0));

-- +goose Down
-- reverse: create "edcb_network_tvs" table
DROP TABLE "edcb_network_tvs";
