-- +goose Up
-- create "players" table
CREATE TABLE "players" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "session_id" uuid NOT NULL, "state" jsonb NOT NULL, "started_at" timestamptz NOT NULL, "last_seen_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "players_session_id_fkey" FOREIGN KEY ("session_id") REFERENCES "sessions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "players_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "players_user_id_idx" to table: "players"
CREATE INDEX "players_user_id_idx" ON "players" ("user_id", "last_seen_at");
-- create "player_commands" table
CREATE TABLE "player_commands" ("id" bigint NOT NULL GENERATED ALWAYS AS IDENTITY, "player_id" uuid NOT NULL, "command" jsonb NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "player_commands_player_id_fkey" FOREIGN KEY ("player_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "player_commands_player_id_idx" to table: "player_commands"
CREATE INDEX "player_commands_player_id_idx" ON "player_commands" ("player_id", "id");
-- create "player_versions" table
CREATE TABLE "player_versions" ("user_id" uuid NOT NULL, "version" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "player_versions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "player_versions" table
DROP TABLE "player_versions";
-- reverse: create index "player_commands_player_id_idx" to table: "player_commands"
DROP INDEX "player_commands_player_id_idx";
-- reverse: create "player_commands" table
DROP TABLE "player_commands";
-- reverse: create index "players_user_id_idx" to table: "players"
DROP INDEX "players_user_id_idx";
-- reverse: create "players" table
DROP TABLE "players";
