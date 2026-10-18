-- +goose Up
-- modify "user_oidc_identities" table
ALTER TABLE "user_oidc_identities" ADD COLUMN "picture" text NULL;

-- +goose Down
-- reverse: modify "user_oidc_identities" table
ALTER TABLE "user_oidc_identities" DROP COLUMN "picture";
