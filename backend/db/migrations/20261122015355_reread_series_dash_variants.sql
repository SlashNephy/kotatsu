-- +goose Up
-- 番組名から読んだシリーズ名で、波ダッシュとダッシュの書き方を揃えた (#2039)。揃える文字を含むシリーズ名を空に戻し、読み直す。
-- 番組は次の全量の突き合わせで、録画履歴は kotatsu migrate で埋める。シリーズ名は NFKC の後に揃えているので、全角チルダは含まない。
UPDATE "programs" SET "series" = NULL, "episode_number" = NULL WHERE "series" ~ '[〜‐‑‒–—―−]';
UPDATE "recording_history" SET "series" = NULL WHERE "series" ~ '[〜‐‑‒–—―−]';

-- +goose Down
-- 前の読み方では、~ と - を含むシリーズ名の一部を、別の文字で読む。空に戻し、前の版に読み直させる。
UPDATE "programs" SET "series" = NULL, "episode_number" = NULL WHERE "series" ~ '[~-]';
UPDATE "recording_history" SET "series" = NULL WHERE "series" ~ '[~-]';
