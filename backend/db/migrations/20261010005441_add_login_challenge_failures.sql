-- +goose Up
-- modify "login_challenges" table
ALTER TABLE "login_challenges" ADD COLUMN "failed_attempts" integer NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "login_challenges" table
ALTER TABLE "login_challenges" DROP COLUMN "failed_attempts";
