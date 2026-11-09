-- +goose Up
-- create "anime_following_fetches" table
CREATE TABLE "anime_following_fetches" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "syobocal_tid" integer NOT NULL, "checked_at" timestamptz NOT NULL, "fetched_at" timestamptz NULL, "error" text NULL, "retry_at" timestamptz NULL, PRIMARY KEY ("user_id", "destination", "syobocal_tid"), CONSTRAINT "anime_following_fetches_user_id_destination_fkey" FOREIGN KEY ("user_id", "destination") REFERENCES "anime_record_connections" ("user_id", "destination") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create "anime_following_viewings" table
CREATE TABLE "anime_following_viewings" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "syobocal_tid" integer NOT NULL, "username" text NOT NULL, "name" text NOT NULL, "avatar_url" text NULL, "status" text NOT NULL, "progress" integer NULL, "episodes_count" integer NULL, "score" double precision NULL, "notes" text NULL, PRIMARY KEY ("user_id", "destination", "syobocal_tid", "username"), CONSTRAINT "anime_following_viewings_user_id_destination_syobocal_tid_fkey" FOREIGN KEY ("user_id", "destination", "syobocal_tid") REFERENCES "anime_following_fetches" ("user_id", "destination", "syobocal_tid") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_following_viewings_status_check" CHECK (status = ANY (ARRAY['watched'::text, 'watching'::text, 'repeating'::text, 'on_hold'::text, 'stop_watching'::text, 'wanna_watch'::text])));

-- +goose Down
-- reverse: create "anime_following_viewings" table
DROP TABLE "anime_following_viewings";
-- reverse: create "anime_following_fetches" table
DROP TABLE "anime_following_fetches";
