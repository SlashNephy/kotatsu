-- +goose Up
-- create "users" table
CREATE TABLE "users" ("id" uuid NOT NULL, "username" text NOT NULL, "display_name" text NOT NULL, "role" text NOT NULL, "password_hash" text NULL, "disabled_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "users_role_check" CHECK (role = ANY (ARRAY['admin'::text, 'member'::text, 'viewer'::text])));
-- create index "users_lower_username_idx" to table: "users"
CREATE UNIQUE INDEX "users_lower_username_idx" ON "users" ((lower(username)));

-- +goose Down
-- reverse: create index "users_lower_username_idx" to table: "users"
DROP INDEX "users_lower_username_idx";
-- reverse: create "users" table
DROP TABLE "users";
