-- +goose Up
-- modify "calendar_feeds" table
ALTER TABLE "calendar_feeds" DROP CONSTRAINT "calendar_feeds_pkey", ADD CONSTRAINT "calendar_feeds_kind_check" CHECK (kind = ANY (ARRAY['calendar'::text, 'iptv'::text])), ADD COLUMN "kind" text NOT NULL DEFAULT 'calendar', ADD PRIMARY KEY ("user_id", "kind");

-- +goose Down
-- reverse: modify "calendar_feeds" table
ALTER TABLE "calendar_feeds" DROP CONSTRAINT "calendar_feeds_pkey", DROP COLUMN "kind", DROP CONSTRAINT "calendar_feeds_kind_check", ADD PRIMARY KEY ("user_id");
