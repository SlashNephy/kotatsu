-- +goose Up
-- modify "oidc_logins" table
ALTER TABLE "oidc_logins" ADD COLUMN "session_id" uuid NULL, ADD COLUMN "return_to" text NULL, ADD CONSTRAINT "oidc_logins_session_id_fkey" FOREIGN KEY ("session_id") REFERENCES "sessions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;

-- +goose Down
-- reverse: modify "oidc_logins" table
ALTER TABLE "oidc_logins" DROP CONSTRAINT "oidc_logins_session_id_fkey", DROP COLUMN "return_to", DROP COLUMN "session_id";
