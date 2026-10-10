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

-- ユーザー (設計書 9.7)。ロールは admin (管理者)、member (メンバー)、viewer (視聴者) のどれか。
CREATE TABLE users (
  id uuid PRIMARY KEY,
  username text NOT NULL,
  display_name text NOT NULL,
  role text NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
  -- パスワードのハッシュ (設計書 9.3)。OIDC だけでログインするユーザーでは空。
  password_hash text,
  -- 無効にした日時。無効にしたユーザーのセッションは使えない。
  disabled_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

-- ユーザー名は大文字と小文字を区別せずに一意にする。citext は拡張を要するので使わない。
CREATE UNIQUE INDEX users_lower_username_idx ON users (lower(username));

-- サーバー側で持つセッション (設計書 9.5)。全インスタンスで共有する。
-- Cookie にはランダムなトークンを入れ、DB にはその SHA-256 だけを保存する。DB が漏れても、セッションを乗っ取れない。
CREATE TABLE sessions (
  id uuid PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL,
  -- 最後にパスワードなどで認証した日時。重要な操作の前の再認証 (設計書 9.5) の判定に使う。
  authenticated_at timestamptz NOT NULL,
  -- 最後に使われた日時。無操作の期限の判定に使う。書き込みを減らすため、一定の間隔でしか更新しない。
  last_seen_at timestamptz NOT NULL,
  -- 絶対的な期限。使われ続けていても、これを過ぎたら使えない。
  expires_at timestamptz NOT NULL,
  -- ユーザーが失効させた日時。
  revoked_at timestamptz,
  -- セッションの一覧で、どの端末かを見分けるための情報。
  user_agent text NOT NULL,
  source_ip inet
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- パスワードの確認を終え、第二要素の確認を待っているログイン (設計書 9.3)。
-- パスワードでのログインは、第二要素を確かめるまでセッションにしない。
-- Cookie にはランダムなトークンを入れ、DB にはその SHA-256 だけを保存する。
CREATE TABLE login_challenges (
  id uuid PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  user_agent text NOT NULL,
  source_ip inet
);

CREATE INDEX login_challenges_user_id_idx ON login_challenges (user_id);

-- ログインの試行回数の制限 (設計書 9.5)。アカウント単位と IP 単位の 2 種類の行を持つ。
-- 失敗が続くと待ち時間を指数的に延ばすが、上限を設けて永久にはロックしない。
CREATE TABLE login_throttles (
  -- 制限の単位。account:<小文字のユーザー名> か ip:<アドレス>。存在しないユーザー名も制限する。
  key text PRIMARY KEY,
  -- 連続した失敗の回数。成功するか、最後の失敗から十分に時間が経つと数え直す。
  failures integer NOT NULL,
  -- この時刻までは、試行を受け付けない。
  locked_until timestamptz NOT NULL,
  last_failed_at timestamptz NOT NULL
);

-- チャンネル種別 (設計書 3.1)。チューナーサーバーが返す文字列をそのまま主キーにする。
-- GR、BS、CS、SKY には表示名と並び順の既定値を持たせ、それ以外の種別は、同期で見つけたときに種別の文字列のまま足す。
-- 管理者は表示名と並び順を画面から変えられるので、同期は既にある行を上書きしない。
CREATE TABLE channel_types (
  type text PRIMARY KEY,
  display_name text NOT NULL,
  sort_order integer NOT NULL
);

-- サービス (設計書 3.2)。Mirakurun のサービス ID (networkId × 100000 + serviceId) を主キーにする。
-- 上流の整数は Go の int で受け取るので、変換で桁あふれを考えずに済むよう、整数の列は bigint にそろえる。
-- 上流から消えたサービスも、予約や録画から参照されうるので消さない。
CREATE TABLE services (
  id bigint PRIMARY KEY,
  network_id bigint NOT NULL,
  -- 放送のサービス ID (Mirakurun の serviceId)。主キーの id と区別するため sid と呼ぶ。
  sid bigint NOT NULL,
  name text NOT NULL,
  -- NFKC で正規化した名前 (設計書 3.5)。Mirakurun はサービス名を全角で返す。
  name_normalized text NOT NULL,
  service_type bigint NOT NULL,
  channel_type text NOT NULL REFERENCES channel_types (type),
  channel text NOT NULL,
  remote_control_key_id bigint,
  logo_id bigint,
  -- 番組表に出さない印と、並び順の上書き。どちらも管理者が画面から変え、同期は上書きしない。
  hidden boolean NOT NULL DEFAULT false,
  sort_order integer,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 放送中の番組と、これから放送される番組 (設計書 3.4)。
-- Mirakurun と同じく、サービスと eventId の組で一意にする。EID は再利用されうるが、上流が同時に持つ番組の中では一意である。
-- 放送時間が変わっても同じ行を更新するので、番組予約は (サービス, eventId) で番組を追える (設計書 4.6)。
-- 開始時刻で番組を見分けるのは、放送が終わった番組を program_archives へ移すときである。
CREATE TABLE programs (
  service_id bigint NOT NULL REFERENCES services (id),
  event_id bigint NOT NULL,
  -- Mirakurun の番組 ID。属性として持つだけで、番組の同一性の判定には使わない (設計書 3.2)。
  mirakurun_id bigint NOT NULL,
  start_at timestamptz NOT NULL,
  -- 終了時刻。延長などで終了時刻が未定のときは、Mirakurun の慣習どおり長さを 1 ミリ秒として扱い、end_undecided を立てる。
  end_at timestamptz NOT NULL,
  end_undecided boolean NOT NULL,
  is_free boolean NOT NULL,
  name text NOT NULL,
  description text NOT NULL,
  -- NFKC で正規化した番組名と説明 (設計書 3.5)。キーワードの照合と検索に使う。
  name_normalized text NOT NULL,
  description_normalized text NOT NULL,
  -- 詳細 (見出しから本文への対応)、ジャンル、関連する番組 (イベント共有とイベントリレー) は、上流の JSON のまま持つ。
  extended jsonb NOT NULL,
  genres jsonb NOT NULL,
  related_items jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (service_id, event_id)
);

CREATE INDEX programs_start_at_idx ON programs (start_at);
CREATE INDEX programs_service_id_start_at_idx ON programs (service_id, start_at);

-- EPG の変更の記録 (設計書 3.3)。反映のたびに、変更のあったサービスと時間帯を、通知の版数と一緒に残す。
-- 通知で届くのは版数だけなので、受け取った側は、自分が知っている版数より後の行を読んで、何が変わったかを知る。
CREATE TABLE epg_changes (
  version bigint NOT NULL,
  service_id bigint NOT NULL,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE INDEX epg_changes_version_idx ON epg_changes (version);
CREATE INDEX epg_changes_created_at_idx ON epg_changes (created_at);

-- 放送が終わった番組 (設計書 3.4)。neko が、放送の終わりから猶予が過ぎた番組を programs から移す。
-- EID は再利用されうるので、サービスと eventId に開始時刻を加えた組で一意にする (設計書 3.2)。
CREATE TABLE program_archives (
  service_id bigint NOT NULL REFERENCES services (id),
  event_id bigint NOT NULL,
  mirakurun_id bigint NOT NULL,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  end_undecided boolean NOT NULL,
  is_free boolean NOT NULL,
  name text NOT NULL,
  description text NOT NULL,
  name_normalized text NOT NULL,
  description_normalized text NOT NULL,
  extended jsonb NOT NULL,
  genres jsonb NOT NULL,
  related_items jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  archived_at timestamptz NOT NULL,
  PRIMARY KEY (service_id, event_id, start_at)
);

-- 過去の番組の検索は期間の指定が必須なので、期間で絞り込むインデックスを効かせる (設計書 3.4)。
CREATE INDEX program_archives_start_at_idx ON program_archives (start_at);
CREATE INDEX program_archives_service_id_start_at_idx ON program_archives (service_id, start_at);
-- 保持期間を過ぎたアーカイブを消すときに使う。
CREATE INDEX program_archives_end_at_idx ON program_archives (end_at);
-- 猶予が過ぎた番組を探すときに使う。
CREATE INDEX programs_end_at_idx ON programs (end_at);

-- 予約 (設計書 4.1)。番組予約 (program) と時刻予約 (time) を持つ。ルール予約 (rule) はルールの照合が作る。
-- どの予約も番組情報のコピーを持ち、番組が EPG から消えても内容を表示できるようにする。
-- そのため programs への外部キーは張らない。
CREATE TABLE reservations (
  id uuid PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('program', 'time', 'rule')),
  service_id bigint NOT NULL REFERENCES services (id),
  -- 番組予約とルール予約の番組の eventId。時刻予約では NULL。
  event_id bigint,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  -- 番組情報のコピー。時刻予約では、ユーザーが付けた名前だけを持つ。
  name text NOT NULL,
  description text NOT NULL,
  extended jsonb NOT NULL,
  genres jsonb NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  -- イベントリレーで引き継がれた番組を、引き継ぎ先でも自動で予約するかどうか (設計書 4.6)。
  follow_event_relay boolean NOT NULL DEFAULT true,
  -- 録画設定の上書き (設計書 4.7)。指定のない項目は全体の既定値を使う。
  settings jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  -- 作成者と更新者。ルール予約は neko が作るので、ユーザーがいないことがある。
  created_by uuid REFERENCES users (id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL,
  updated_by uuid REFERENCES users (id) ON DELETE SET NULL,
  -- チューナーの割り当ての結果 (設計書 4.5)。neko が計算し直すたびに書き換える。
  -- 状態は、まだ計算していない (pending)、正常 (ok)、一部のみ (partial)、競合 (conflict) のどれか。
  assignment_state text NOT NULL DEFAULT 'pending' CHECK (assignment_state IN ('pending', 'ok', 'partial', 'conflict')),
  -- 割り当てたチューナーの番号と、マージンを削った後の録画する予定の時刻。割り当てていなければ NULL。
  assigned_tuner bigint,
  planned_start_at timestamptz,
  planned_end_at timestamptz,
  -- イベントリレーで引き継いだ予約の、引き継ぎ元の予約 (設計書 4.6)。
  relayed_from uuid REFERENCES reservations (id) ON DELETE SET NULL,
  CONSTRAINT reservations_event_check CHECK ((kind = 'time') = (event_id IS NULL)),
  CONSTRAINT reservations_time_check CHECK (start_at < end_at)
);

-- 1 つの番組に対する予約は、常に 1 件だけにする (設計書 4.1)。
-- 番組は programs と同じく (サービス, eventId) で識別するので、放送時間が変わっても同じ予約のままになる。
-- 放送が終わった番組の予約は、録画の状態遷移 (#28) で片付け、EID の再利用で新しい番組を予約できなくならないようにする。
CREATE UNIQUE INDEX reservations_program_idx ON reservations (service_id, event_id) WHERE event_id IS NOT NULL;
CREATE INDEX reservations_start_at_idx ON reservations (start_at);
-- 引き継ぎ元の予約を消すときに、引き継いだ予約を探すのに使う。
CREATE INDEX reservations_relayed_from_idx ON reservations (relayed_from) WHERE relayed_from IS NOT NULL;

-- チューナーの構成 (設計書 3.6)。neko が、チューナーサーバーから定期的に、またイベントで取得して更新する。
-- チューナーの割り当て (neko) と、システムの状態の画面 (tenban) が読む。
-- 利用者のように刻々と変わる値は持たず、割り当てに使う構成だけを持つ。
CREATE TABLE tuners (
  -- チューナーサーバーでのチューナーの番号。
  tuner_index bigint PRIMARY KEY,
  name text NOT NULL,
  -- 受信できるチャンネル種別。1 本で複数の種別を受信できる機器もある。
  types text[] NOT NULL,
  is_available boolean NOT NULL,
  is_remote boolean NOT NULL,
  is_fault boolean NOT NULL,
  updated_at timestamptz NOT NULL
);

-- チューナーサーバーの状態 (設計書 3.7)。行は 1 つだけ持つ。
CREATE TABLE tuner_server_status (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  -- 接続できない間は縮退中とし、接続できなくなった日時を持つ。接続できていれば NULL。
  degraded_since timestamptz,
  -- 最後に接続に失敗した理由。
  last_error text NOT NULL,
  -- チューナーサーバーの時計 (HTTP の Date ヘッダー) と OS の時計の差 (ミリ秒)。分からなければ NULL。
  clock_skew_ms bigint,
  checked_at timestamptz NOT NULL
);

-- ルール (設計書 4.2)。条件は match パッケージの型で検証し、スキーマのバージョンを付けて JSONB に保存する (設計書 8.3)。
CREATE TABLE rules (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  memo text NOT NULL,
  enabled boolean NOT NULL,
  conditions jsonb NOT NULL,
  -- 録画設定の上書き (設計書 4.7)。
  settings jsonb NOT NULL,
  -- ルール自体の有効期間。番組の開始時刻がこの範囲に入るときだけ予約する。NULL なら区切らない。
  valid_from timestamptz,
  valid_until timestamptz,
  -- 重複の回避 (設計書 4.4)。同じ番組名を duplicate_window_days 日以内に録画済みなら予約しない。日数が NULL なら期間を区切らない。
  avoid_duplicates boolean NOT NULL,
  duplicate_window_days integer,
  -- ルール予約で、イベントリレーの引き継ぎ先も予約するかどうか (設計書 4.6)。
  follow_event_relay boolean NOT NULL,
  created_at timestamptz NOT NULL,
  created_by uuid REFERENCES users (id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL,
  updated_by uuid REFERENCES users (id) ON DELETE SET NULL,
  CONSTRAINT rules_valid_period_check CHECK (valid_from IS NULL OR valid_until IS NULL OR valid_from < valid_until),
  CONSTRAINT rules_duplicate_window_check CHECK (duplicate_window_days IS NULL OR duplicate_window_days > 0)
);

-- 予約と、一致したルールの多対多の関係 (設計書 4.1)。複数のルールに一致した番組も、予約は 1 件にまとめる。
CREATE TABLE reservation_rules (
  reservation_id uuid NOT NULL REFERENCES reservations (id) ON DELETE CASCADE,
  rule_id uuid NOT NULL REFERENCES rules (id) ON DELETE CASCADE,
  PRIMARY KEY (reservation_id, rule_id)
);

CREATE INDEX reservation_rules_rule_id_idx ON reservation_rules (rule_id);

-- 録画履歴 (設計書 4.4)。重複の回避に使う。正常に終了し、ドロップが閾値未満だった録画だけを記録し、録画を削除しても残す。
CREATE TABLE recording_history (
  id uuid PRIMARY KEY,
  service_id bigint NOT NULL REFERENCES services (id),
  -- 重複の判定に使う、番組名を正規化したもの。[新] や [再] のような記号を取り除き、話数は残す。
  title_key text NOT NULL,
  -- 画面に出す、元の番組名。
  name text NOT NULL,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL
);

-- 重複の回避の照合で、(正規化した番組名, サービス) と期間で引く (設計書 4.4)。
-- EPGStation では、このインデックスがないために照合が全件を走査し、利用者が手でインデックスを足していた。
CREATE INDEX recording_history_title_key_service_id_end_at_idx ON recording_history (title_key, service_id, end_at);

-- 番組表ビュー (設計書 10.3)。番組表に適用する条件を、ユーザーごとに名前を付けて保存する。
-- 条件は、ジャンルの包含と除外、番組の最短の長さ、チャンネルの並び、番組のないチャンネルを隠すかどうか。
CREATE TABLE guide_views (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name text NOT NULL,
  -- 条件。ジャンルの包含と除外、最短の長さ、チャンネルの並びを持つ。
  settings jsonb NOT NULL,
  hide_empty_services boolean NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX guide_views_user_id_name_idx ON guide_views (user_id, name);
