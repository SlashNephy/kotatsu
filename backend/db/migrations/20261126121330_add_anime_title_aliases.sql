-- +goose Up
-- create "anime_title_aliases" table
CREATE TABLE "anime_title_aliases" ("title_key" text NOT NULL, "syobocal_tid" integer NOT NULL, "count_offset" double precision NOT NULL, PRIMARY KEY ("title_key", "syobocal_tid", "count_offset"));

-- +goose Down
-- reverse: create "anime_title_aliases" table
DROP TABLE "anime_title_aliases";
