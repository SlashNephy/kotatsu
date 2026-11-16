-- +goose Up
-- create "rule_groups" table
CREATE TABLE "rule_groups" ("id" uuid NOT NULL, "name" text NOT NULL, "position" integer NOT NULL, "created_at" timestamptz NOT NULL, "created_by" uuid NULL, "updated_at" timestamptz NOT NULL, "updated_by" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "rule_groups_name_key" UNIQUE ("name"), CONSTRAINT "rule_groups_created_by_fkey" FOREIGN KEY ("created_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "rule_groups_updated_by_fkey" FOREIGN KEY ("updated_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- modify "rules" table
ALTER TABLE "rules" ADD COLUMN "group_id" uuid NULL, ADD CONSTRAINT "rules_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "rule_groups" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "rules_group_id_idx" to table: "rules"
CREATE INDEX "rules_group_id_idx" ON "rules" ("group_id");

-- +goose Down
-- reverse: create index "rules_group_id_idx" to table: "rules"
DROP INDEX "rules_group_id_idx";
-- reverse: modify "rules" table
ALTER TABLE "rules" DROP CONSTRAINT "rules_group_id_fkey", DROP COLUMN "group_id";
-- reverse: create "rule_groups" table
DROP TABLE "rule_groups";
