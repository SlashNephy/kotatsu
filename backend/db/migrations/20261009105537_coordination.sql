-- +goose Up
-- create "change_versions" table
CREATE TABLE "change_versions" ("channel" text NOT NULL, "version" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("channel"));
-- create "instances" table
CREATE TABLE "instances" ("id" uuid NOT NULL, "role" text NOT NULL, "address" text NOT NULL, "started_at" timestamptz NOT NULL, "heartbeat_at" timestamptz NOT NULL, PRIMARY KEY ("id", "role"));
-- create index "instances_role_heartbeat_at_idx" to table: "instances"
CREATE INDEX "instances_role_heartbeat_at_idx" ON "instances" ("role", "heartbeat_at");
-- create "leases" table
CREATE TABLE "leases" ("name" text NOT NULL, "holder" uuid NOT NULL, "generation" bigint NOT NULL, "acquired_at" timestamptz NOT NULL, "renewed_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("name"));

-- +goose Down
-- reverse: create "leases" table
DROP TABLE "leases";
-- reverse: create index "instances_role_heartbeat_at_idx" to table: "instances"
DROP INDEX "instances_role_heartbeat_at_idx";
-- reverse: create "instances" table
DROP TABLE "instances";
-- reverse: create "change_versions" table
DROP TABLE "change_versions";
