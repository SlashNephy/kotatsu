-- +goose Up
-- create "totp_enrollments" table
CREATE TABLE "totp_enrollments" ("user_id" uuid NOT NULL, "secret_encrypted" bytea NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "totp_enrollments_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create "user_recovery_codes" table
CREATE TABLE "user_recovery_codes" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "code_hash" bytea NOT NULL, "created_at" timestamptz NOT NULL, "used_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "user_recovery_codes_user_id_code_hash_key" UNIQUE ("user_id", "code_hash"), CONSTRAINT "user_recovery_codes_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create "user_totps" table
CREATE TABLE "user_totps" ("user_id" uuid NOT NULL, "secret_encrypted" bytea NOT NULL, "last_used_step" bigint NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "user_totps_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "user_totps" table
DROP TABLE "user_totps";
-- reverse: create "user_recovery_codes" table
DROP TABLE "user_recovery_codes";
-- reverse: create "totp_enrollments" table
DROP TABLE "totp_enrollments";
