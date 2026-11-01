-- +goose Up
-- create "watch_rules" table
CREATE TABLE "watch_rules" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "name" text NOT NULL, "enabled" boolean NOT NULL, "conditions" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "watch_rules_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "watch_rules_user_id_created_at_idx" to table: "watch_rules"
CREATE INDEX "watch_rules_user_id_created_at_idx" ON "watch_rules" ("user_id", "created_at", "id");
-- modify "watch_reservations" table
ALTER TABLE "watch_reservations" ADD COLUMN "watch_rule_id" uuid NULL, ADD CONSTRAINT "watch_reservations_watch_rule_id_fkey" FOREIGN KEY ("watch_rule_id") REFERENCES "watch_rules" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "watch_reservations_watch_rule_id_idx" to table: "watch_reservations"
CREATE INDEX "watch_reservations_watch_rule_id_idx" ON "watch_reservations" ("watch_rule_id");
-- create "watch_rule_dismissals" table
CREATE TABLE "watch_rule_dismissals" ("user_id" uuid NOT NULL, "service_id" bigint NOT NULL, "event_id" bigint NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "service_id", "event_id"), CONSTRAINT "watch_rule_dismissals_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "watch_rule_dismissals_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "watch_rule_dismissals" table
DROP TABLE "watch_rule_dismissals";
-- reverse: create index "watch_reservations_watch_rule_id_idx" to table: "watch_reservations"
DROP INDEX "watch_reservations_watch_rule_id_idx";
-- reverse: modify "watch_reservations" table
ALTER TABLE "watch_reservations" DROP CONSTRAINT "watch_reservations_watch_rule_id_fkey", DROP COLUMN "watch_rule_id";
-- reverse: create index "watch_rules_user_id_created_at_idx" to table: "watch_rules"
DROP INDEX "watch_rules_user_id_created_at_idx";
-- reverse: create "watch_rules" table
DROP TABLE "watch_rules";
