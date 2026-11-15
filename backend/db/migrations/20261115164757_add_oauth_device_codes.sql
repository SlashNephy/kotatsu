-- +goose Up
-- modify "oauth_grants" table
ALTER TABLE "oauth_grants" ADD CONSTRAINT "oauth_grants_check" CHECK ((kind = 'device'::text) OR (revoked_at IS NULL)), ADD CONSTRAINT "oauth_grants_kind_check" CHECK (kind = ANY (ARRAY['mcp'::text, 'device'::text])), ADD COLUMN "kind" text NOT NULL DEFAULT 'mcp', ADD COLUMN "revoked_at" timestamptz NULL;
-- create "oauth_device_codes" table
CREATE TABLE "oauth_device_codes" ("device_code_hash" bytea NOT NULL, "user_code" text NOT NULL, "client_id" text NOT NULL, "device_name" text NOT NULL, "scopes" text[] NOT NULL, "resource" text NOT NULL, "source_network" cidr NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, "last_polled_at" timestamptz NULL, "approved_by" uuid NULL, "approved_at" timestamptz NULL, "used_at" timestamptz NULL, PRIMARY KEY ("device_code_hash"), CONSTRAINT "oauth_device_codes_user_code_key" UNIQUE ("user_code"), CONSTRAINT "oauth_device_codes_approved_by_fkey" FOREIGN KEY ("approved_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "oauth_device_codes_check" CHECK ((approved_by IS NULL) = (approved_at IS NULL)), CONSTRAINT "oauth_device_codes_check1" CHECK ((used_at IS NULL) OR (approved_at IS NOT NULL)));
-- create index "oauth_device_codes_expires_at_idx" to table: "oauth_device_codes"
CREATE INDEX "oauth_device_codes_expires_at_idx" ON "oauth_device_codes" ("expires_at");
-- create index "oauth_device_codes_source_network_idx" to table: "oauth_device_codes"
CREATE INDEX "oauth_device_codes_source_network_idx" ON "oauth_device_codes" ("source_network");

-- +goose Down
-- reverse: create index "oauth_device_codes_source_network_idx" to table: "oauth_device_codes"
DROP INDEX "oauth_device_codes_source_network_idx";
-- reverse: create index "oauth_device_codes_expires_at_idx" to table: "oauth_device_codes"
DROP INDEX "oauth_device_codes_expires_at_idx";
-- reverse: create "oauth_device_codes" table
DROP TABLE "oauth_device_codes";
-- reverse: modify "oauth_grants" table
ALTER TABLE "oauth_grants" DROP COLUMN "revoked_at", DROP COLUMN "kind", DROP CONSTRAINT "oauth_grants_kind_check", DROP CONSTRAINT "oauth_grants_check";
