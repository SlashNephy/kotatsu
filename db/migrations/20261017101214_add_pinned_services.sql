-- +goose Up
-- create "pinned_services" table
CREATE TABLE "pinned_services" ("user_id" uuid NOT NULL, "service_id" bigint NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "service_id"), CONSTRAINT "pinned_services_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "pinned_services_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "pinned_services_service_id_idx" to table: "pinned_services"
CREATE INDEX "pinned_services_service_id_idx" ON "pinned_services" ("service_id");

-- +goose Down
-- reverse: create index "pinned_services_service_id_idx" to table: "pinned_services"
DROP INDEX "pinned_services_service_id_idx";
-- reverse: create "pinned_services" table
DROP TABLE "pinned_services";
