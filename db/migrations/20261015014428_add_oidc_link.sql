-- +goose Up
-- modify "oidc_logins" table
ALTER TABLE "oidc_logins" ADD COLUMN "link" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "oidc_logins" table
ALTER TABLE "oidc_logins" DROP COLUMN "link";
