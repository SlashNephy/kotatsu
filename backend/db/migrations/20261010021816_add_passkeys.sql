-- +goose Up
-- modify "login_challenges" table
ALTER TABLE "login_challenges" ADD COLUMN "webauthn_session" jsonb NULL;
-- create "user_passkeys" table
CREATE TABLE "user_passkeys" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "credential_id" bytea NOT NULL, "credential" jsonb NOT NULL, "rp_id" text NOT NULL, "name" text NOT NULL, "created_at" timestamptz NOT NULL, "last_used_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "user_passkeys_credential_id_key" UNIQUE ("credential_id"), CONSTRAINT "user_passkeys_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "user_passkeys_user_id_idx" to table: "user_passkeys"
CREATE INDEX "user_passkeys_user_id_idx" ON "user_passkeys" ("user_id");
-- create "webauthn_ceremonies" table
CREATE TABLE "webauthn_ceremonies" ("id" uuid NOT NULL, "token_hash" bytea NOT NULL, "kind" text NOT NULL, "user_id" uuid NULL, "session" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "webauthn_ceremonies_token_hash_key" UNIQUE ("token_hash"), CONSTRAINT "webauthn_ceremonies_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "webauthn_ceremonies_kind_check" CHECK (kind = ANY (ARRAY['registration'::text, 'login'::text])));
-- create index "webauthn_ceremonies_expires_at_idx" to table: "webauthn_ceremonies"
CREATE INDEX "webauthn_ceremonies_expires_at_idx" ON "webauthn_ceremonies" ("expires_at");

-- +goose Down
-- reverse: create index "webauthn_ceremonies_expires_at_idx" to table: "webauthn_ceremonies"
DROP INDEX "webauthn_ceremonies_expires_at_idx";
-- reverse: create "webauthn_ceremonies" table
DROP TABLE "webauthn_ceremonies";
-- reverse: create index "user_passkeys_user_id_idx" to table: "user_passkeys"
DROP INDEX "user_passkeys_user_id_idx";
-- reverse: create "user_passkeys" table
DROP TABLE "user_passkeys";
-- reverse: modify "login_challenges" table
ALTER TABLE "login_challenges" DROP COLUMN "webauthn_session";
