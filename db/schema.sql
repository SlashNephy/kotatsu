-- Kotatsu のスキーマの原本。変更したら mise run db:diff でマイグレーションを生成する。

-- このスキーマで動かしてよいバイナリの下限を、kotatsu migrate が記録する。
-- ローリング更新の間は新旧のバイナリが同時に動くので、DB のスキーマがバイナリより新しくても、
-- バイナリが知っている最新のマイグレーションがこの下限以上なら起動を許す。
CREATE TABLE schema_compatibility (
  -- 行を 1 つに限るための列。
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  min_binary_version bigint NOT NULL,
  updated_at timestamptz NOT NULL
);

-- リーダー選出に使うリース (設計書 1.4)。neko は名前 neko のリースを取ったインスタンスだけがアクティブになる。
-- 期限は各インスタンスの時計で決めるので、インスタンス間の時計のずれは NTP で抑えられている前提にする。
CREATE TABLE leases (
  name text PRIMARY KEY,
  -- リースを持っているインスタンスの ID。
  holder uuid NOT NULL,
  -- リースを取り直すたびに増える世代番号。古いリーダーの書き込みを拒否するのに使う。
  generation bigint NOT NULL,
  acquired_at timestamptz NOT NULL,
  renewed_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);

-- 各ロールのインスタンスの登録簿 (設計書 1.4)。all では 1 つのプロセスがロールごとに 1 行ずつ書く。
CREATE TABLE instances (
  id uuid NOT NULL,
  role text NOT NULL,
  -- 他のインスタンスからこのインスタンスに届くアドレス (host:port)。
  address text NOT NULL,
  started_at timestamptz NOT NULL,
  heartbeat_at timestamptz NOT NULL,
  PRIMARY KEY (id, role)
);

CREATE INDEX instances_role_heartbeat_at_idx ON instances (role, heartbeat_at);

-- 変更通知のチャンネルごとの版数 (設計書 1.4)。変更を書くトランザクションの中で版数を上げ、NOTIFY も送る。
-- LISTEN を使えないときは、版数をポーリングして変更を知る。
CREATE TABLE change_versions (
  channel text PRIMARY KEY,
  version bigint NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 監査ログ (設計書 8.5)。誰が何をしたかを記録する。
CREATE TABLE audit_events (
  id uuid PRIMARY KEY,
  occurred_at timestamptz NOT NULL,
  -- 操作者の種類。user ならユーザーの操作、system ならシステムのロールの操作。
  actor_type text NOT NULL,
  -- 操作したユーザーの ID。ユーザーを消しても記録は残すので、users への外部キーは張らない。
  actor_user_id uuid,
  -- 操作したシステムのロールの名前 (neko など)。
  actor_role text,
  -- 操作の種類 (rule.create など)。
  action text NOT NULL,
  -- 対象の種類と ID。ID の形は対象ごとに異なるので文字列で持つ。
  target_type text NOT NULL,
  target_id text,
  -- 変更の要約。
  summary jsonb NOT NULL,
  -- 送信元。システムの操作では空。
  source_ip inet,
  user_agent text,
  CONSTRAINT audit_events_actor_check CHECK (
    (actor_type = 'user' AND actor_user_id IS NOT NULL AND actor_role IS NULL)
    OR (actor_type = 'system' AND actor_user_id IS NULL AND actor_role IS NOT NULL)
  )
);

-- 保持期間を過ぎた記録を消すときと、新しい順に並べるときに使う。
CREATE INDEX audit_events_occurred_at_idx ON audit_events (occurred_at);
