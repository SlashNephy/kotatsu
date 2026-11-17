-- +goose Up
-- create "google_apps" table
CREATE TABLE "google_apps" ("id" smallint NOT NULL, "client_id" text NOT NULL, "client_secret_encrypted" bytea NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "google_apps_id_check" CHECK (id = 1));
-- create "google_drive_connections" table
CREATE TABLE "google_drive_connections" ("id" smallint NOT NULL, "refresh_token_encrypted" bytea NOT NULL, "account_name" text NOT NULL, "connected_at" timestamptz NOT NULL, "revoked_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "google_drive_connections_id_check" CHECK (id = 1));
-- create "google_authorizations" table
CREATE TABLE "google_authorizations" ("id" uuid NOT NULL, "state_hash" bytea NOT NULL, "session_id" uuid NOT NULL, "user_id" uuid NOT NULL, "purpose" text NOT NULL, "code_verifier" text NOT NULL, "redirect_uri" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "google_authorizations_state_hash_key" UNIQUE ("state_hash"), CONSTRAINT "google_authorizations_session_id_fkey" FOREIGN KEY ("session_id") REFERENCES "sessions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "google_authorizations_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "google_authorizations_purpose_check" CHECK (purpose = 'drive'::text));
-- create index "google_authorizations_expires_at_idx" to table: "google_authorizations"
CREATE INDEX "google_authorizations_expires_at_idx" ON "google_authorizations" ("expires_at");

-- +goose Down
-- reverse: create index "google_authorizations_expires_at_idx" to table: "google_authorizations"
DROP INDEX "google_authorizations_expires_at_idx";
-- reverse: create "google_authorizations" table
DROP TABLE "google_authorizations";
-- reverse: create "google_drive_connections" table
DROP TABLE "google_drive_connections";
-- reverse: create "google_apps" table
DROP TABLE "google_apps";
