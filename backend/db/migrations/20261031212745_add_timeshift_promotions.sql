-- +goose Up
-- create "timeshift_promotions" table
CREATE TABLE "timeshift_promotions" ("recording_id" uuid NOT NULL, "service_id" bigint NOT NULL, "from_at" timestamptz NOT NULL, "to_at" timestamptz NOT NULL, "created_by" uuid NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "timeshift_promotions_service_id_from_at_key" UNIQUE ("service_id", "from_at"), CONSTRAINT "timeshift_promotions_created_by_fkey" FOREIGN KEY ("created_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "timeshift_promotions_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "timeshift_promotions_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "timeshift_promotions" table
DROP TABLE "timeshift_promotions";
