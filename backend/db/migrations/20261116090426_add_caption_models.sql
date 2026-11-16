-- +goose Up
-- create "caption_models" table
CREATE TABLE "caption_models" ("kind" text NOT NULL, "model_id" text NOT NULL, "size" bigint NOT NULL, "sha256" text NOT NULL, "installed_at" timestamptz NOT NULL, PRIMARY KEY ("kind"));

-- +goose Down
-- reverse: create "caption_models" table
DROP TABLE "caption_models";
