-- +goose Up
-- modify "webauthn_ceremonies" table
ALTER TABLE "webauthn_ceremonies" DROP CONSTRAINT "webauthn_ceremonies_kind_check", ADD CONSTRAINT "webauthn_ceremonies_kind_check" CHECK (kind = ANY (ARRAY['registration'::text, 'login'::text, 'reauthentication'::text]));

-- +goose Down
-- reverse: modify "webauthn_ceremonies" table
ALTER TABLE "webauthn_ceremonies" DROP CONSTRAINT "webauthn_ceremonies_kind_check", ADD CONSTRAINT "webauthn_ceremonies_kind_check" CHECK (kind = ANY (ARRAY['registration'::text, 'login'::text]));
