-- +goose Up
-- create "anime_artworks" table
CREATE TABLE "anime_artworks" ("mal_id" integer NOT NULL, "image_url" text NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("mal_id"));

-- +goose Down
-- reverse: create "anime_artworks" table
DROP TABLE "anime_artworks";
