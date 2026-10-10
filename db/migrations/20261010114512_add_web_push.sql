-- +goose Up
-- create "vapid_keys" table
CREATE TABLE "vapid_keys" ("id" boolean NOT NULL DEFAULT true, "public_key" bytea NOT NULL, "private_key_ciphertext" bytea NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "vapid_keys_id_check" CHECK (id));
-- create "web_push_subscriptions" table
CREATE TABLE "web_push_subscriptions" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "endpoint" text NOT NULL, "p256dh" bytea NOT NULL, "auth" bytea NOT NULL, "user_agent" text NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "web_push_subscriptions_endpoint_key" UNIQUE ("endpoint"), CONSTRAINT "web_push_subscriptions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "web_push_subscriptions_user_id_idx" to table: "web_push_subscriptions"
CREATE INDEX "web_push_subscriptions_user_id_idx" ON "web_push_subscriptions" ("user_id");

-- +goose Down
-- reverse: create index "web_push_subscriptions_user_id_idx" to table: "web_push_subscriptions"
DROP INDEX "web_push_subscriptions_user_id_idx";
-- reverse: create "web_push_subscriptions" table
DROP TABLE "web_push_subscriptions";
-- reverse: create "vapid_keys" table
DROP TABLE "vapid_keys";
