-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "failure_detail" text NULL;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "failure_detail";
