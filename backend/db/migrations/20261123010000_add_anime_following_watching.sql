-- +goose Up
-- create "anime_following_watching" table
CREATE TABLE "anime_following_watching" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "username" text NOT NULL, "name" text NOT NULL, "avatar_url" text NULL, "work_id" integer NOT NULL, "title" text NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "destination", "username", "work_id"), CONSTRAINT "anime_following_watching_user_id_destination_fkey" FOREIGN KEY ("user_id", "destination") REFERENCES "anime_record_connections" ("user_id", "destination") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "anime_following_watching" table
DROP TABLE "anime_following_watching";
