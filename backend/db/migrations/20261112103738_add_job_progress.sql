-- +goose Up
-- create "job_progress" table
CREATE TABLE "job_progress" ("job_id" bigint NOT NULL, "attempt" integer NOT NULL, "ratio" double precision NOT NULL, "remaining_ms" bigint NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("job_id"), CONSTRAINT "job_progress_ratio_check" CHECK ((ratio >= (0)::double precision) AND (ratio <= (1)::double precision)), CONSTRAINT "job_progress_remaining_ms_check" CHECK (remaining_ms >= 0));

-- +goose Down
-- reverse: create "job_progress" table
DROP TABLE "job_progress";
