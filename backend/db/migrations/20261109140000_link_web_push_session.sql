-- +goose Up
-- modify "web_push_subscriptions" table
ALTER TABLE "web_push_subscriptions" ADD COLUMN "session_id" uuid NULL, ADD CONSTRAINT "web_push_subscriptions_session_id_fkey" FOREIGN KEY ("session_id") REFERENCES "sessions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "web_push_subscriptions_session_id_idx" to table: "web_push_subscriptions"
CREATE INDEX "web_push_subscriptions_session_id_idx" ON "web_push_subscriptions" ("session_id");

-- +goose Down
-- reverse: create index "web_push_subscriptions_session_id_idx" to table: "web_push_subscriptions"
DROP INDEX "web_push_subscriptions_session_id_idx";
-- reverse: modify "web_push_subscriptions" table
ALTER TABLE "web_push_subscriptions" DROP CONSTRAINT "web_push_subscriptions_session_id_fkey", DROP COLUMN "session_id";
