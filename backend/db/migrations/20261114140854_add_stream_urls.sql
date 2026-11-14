-- +goose Up
-- create "stream_urls" table
CREATE TABLE "stream_urls" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "resource" text NOT NULL, "target_name" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, "last_used_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "stream_urls_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "stream_urls_expires_at_idx" to table: "stream_urls"
CREATE INDEX "stream_urls_expires_at_idx" ON "stream_urls" ("expires_at");

-- +goose Down
-- reverse: create index "stream_urls_expires_at_idx" to table: "stream_urls"
DROP INDEX "stream_urls_expires_at_idx";
-- reverse: create "stream_urls" table
DROP TABLE "stream_urls";
