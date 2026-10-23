-- +goose Up
-- create "guide_views" table
CREATE TABLE "guide_views" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "name" text NOT NULL, "settings" jsonb NOT NULL, "hide_empty_services" boolean NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "guide_views_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "guide_views_user_id_name_idx" to table: "guide_views"
CREATE UNIQUE INDEX "guide_views_user_id_name_idx" ON "guide_views" ("user_id", "name");

-- +goose Down
-- reverse: create index "guide_views_user_id_name_idx" to table: "guide_views"
DROP INDEX "guide_views_user_id_name_idx";
-- reverse: create "guide_views" table
DROP TABLE "guide_views";
