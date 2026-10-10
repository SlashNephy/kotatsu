-- +goose Up
-- create "webhooks" table
CREATE TABLE "webhooks" ("id" uuid NOT NULL, "name" text NOT NULL, "url" text NOT NULL, "format" text NOT NULL, "kinds" text[] NOT NULL, "secret_ciphertext" bytea NOT NULL, "enabled" boolean NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "webhooks_format_check" CHECK (format = ANY (ARRAY['json'::text, 'discord'::text, 'slack'::text])));

-- +goose Down
-- reverse: create "webhooks" table
DROP TABLE "webhooks";
