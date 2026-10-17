-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "genres" jsonb NOT NULL DEFAULT '[]', ADD COLUMN "rule_id" uuid NULL, ADD COLUMN "rule_name" text NULL, ADD CONSTRAINT "recordings_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "rules" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "recordings_rule_id_idx" to table: "recordings"
CREATE INDEX "recordings_rule_id_idx" ON "recordings" ("rule_id") WHERE (rule_id IS NOT NULL);
-- 予約が残っている録画は、予約からジャンルと、一致したルール (最も古いもの) を写す (#468)。
UPDATE "recordings" AS rec SET "genres" = r."genres" FROM "reservations" AS r WHERE rec."reservation_id" = r."id";
UPDATE "recordings" AS rec SET "rule_id" = m."rule_id", "rule_name" = m."rule_name"
FROM (
  SELECT DISTINCT ON (rr."reservation_id") rr."reservation_id", ru."id" AS "rule_id", ru."name" AS "rule_name"
  FROM "reservation_rules" AS rr JOIN "rules" AS ru ON ru."id" = rr."rule_id"
  ORDER BY rr."reservation_id", ru."created_at", ru."id"
) AS m
WHERE rec."reservation_id" = m."reservation_id";

-- +goose Down
-- reverse: create index "recordings_rule_id_idx" to table: "recordings"
DROP INDEX "recordings_rule_id_idx";
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP CONSTRAINT "recordings_rule_id_fkey", DROP COLUMN "rule_name", DROP COLUMN "rule_id", DROP COLUMN "genres";
