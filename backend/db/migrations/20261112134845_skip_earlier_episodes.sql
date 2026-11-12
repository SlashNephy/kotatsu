-- +goose Up
-- modify "recording_history" table
ALTER TABLE "recording_history" ADD COLUMN "series" text NULL;
-- シリーズ名は Go の回の読み方 (episode.Parse) で読むので、SQL では埋めない。足す前の履歴は、kotatsu migrate が Go で埋める (#1499)。
-- create index "recording_history_series_idx" to table: "recording_history"
CREATE INDEX "recording_history_series_idx" ON "recording_history" ("series");
-- modify "rules" table
ALTER TABLE "rules" ADD COLUMN "skip_earlier_episodes" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "rules" table
ALTER TABLE "rules" DROP COLUMN "skip_earlier_episodes";
-- reverse: create index "recording_history_series_idx" to table: "recording_history"
DROP INDEX "recording_history_series_idx";
-- reverse: modify "recording_history" table
ALTER TABLE "recording_history" DROP COLUMN "series";
