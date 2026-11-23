-- 番組名から読んだシリーズ名で、波ダッシュとダッシュと長音符の書き方を揃えた (#2039)。揃える文字を含むシリーズ名を空に戻し、読み直す。
-- 番組は次の全量の突き合わせで、録画履歴は kotatsu migrate で埋める。シリーズ名は NFKC の後に揃えているので、全角チルダは含まない。
UPDATE "programs" SET "series" = NULL, "episode_number" = NULL WHERE "series" ~ '[〜‐‑‒–—―−ー]';
UPDATE "recording_history" SET "series" = NULL WHERE "series" ~ '[〜‐‑‒–—―−ー]';
