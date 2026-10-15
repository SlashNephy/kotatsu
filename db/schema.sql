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

-- 初回セットアップのトークン (設計書 9.2)。管理者が 1 人もいない間だけ発行し、管理者を作ったら消す。
-- トークンそのものは保存せず、SHA-256 だけを保存する。
CREATE TABLE setup_tokens (
  token_hash bytea PRIMARY KEY,
  created_at timestamptz NOT NULL
);

-- パスワードの確認を終え、第二要素の確認を待っているログイン (設計書 9.3)。
-- パスワードでのログインは、第二要素を確かめるまでセッションにしない。
-- Cookie にはランダムなトークンを入れ、DB にはその SHA-256 だけを保存する。
CREATE TABLE login_challenges (
  id uuid PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  -- 第二要素の確認に失敗した回数。上限に達したら、パスワードの確認からやり直させる。
  failed_attempts integer NOT NULL DEFAULT 0,
  -- 第二要素としてパスキーを確かめる手続きの途中の状態 (go-webauthn の SessionData)。始めていなければ空。
  webauthn_session jsonb,
  user_agent text NOT NULL,
  source_ip inet
);

CREATE INDEX login_challenges_user_id_idx ON login_challenges (user_id);

-- 第二要素の TOTP (設計書 9.3)。ユーザーごとに 1 つだけ持つ。
-- 秘密キーは、アプリの秘密キーで暗号化して保存する (設計書 8.4)。
CREATE TABLE user_totps (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  secret_encrypted bytea NOT NULL,
  -- 最後に受け付けたコードのステップ。同じコードを使い回させないため、これ以下のステップは受け付けない。
  last_used_step bigint NOT NULL,
  created_at timestamptz NOT NULL
);

-- 登録中の TOTP。認証アプリに秘密キーを読ませ、コードで確かめるまでの間だけ持つ。
CREATE TABLE totp_enrollments (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  secret_encrypted bytea NOT NULL,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);

-- リカバリーコード (設計書 9.3)。どれも 1 回だけ使える。
-- コードは 50 ビットのランダムな値なので、パスワードと違い、SHA-256 のハッシュで足りる。
CREATE TABLE user_recovery_codes (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  code_hash bytea NOT NULL,
  created_at timestamptz NOT NULL,
  used_at timestamptz,
  UNIQUE (user_id, code_hash)
);

-- OIDC の IdP のユーザーとの紐付け (設計書 9.4)。ユーザーは (issuer, subject) の組で特定する。
CREATE TABLE user_oidc_identities (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  issuer text NOT NULL,
  subject text NOT NULL,
  created_at timestamptz NOT NULL,
  last_used_at timestamptz,
  UNIQUE (issuer, subject)
);

CREATE INDEX user_oidc_identities_user_id_idx ON user_oidc_identities (user_id);

-- OIDC でのログインの途中の状態。IdP から戻ってくるまでの間だけ持ち、Cookie に入れたトークンで引く。
-- PKCE の code_verifier はサーバーにだけ置くので、暗号化しない。
CREATE TABLE oidc_logins (
  id uuid PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE,
  state text NOT NULL,
  nonce text NOT NULL,
  code_verifier text NOT NULL,
  -- IdP に渡したリダイレクト URI。トークンと引き換えるときに、同じ値を渡す。
  redirect_uri text NOT NULL,
  -- IdP での再認証 (設計書 9.5) なら、認証の日時を更新するセッション。紐付けなら、始めたセッション。ログインなら NULL。
  session_id uuid REFERENCES sessions (id) ON DELETE CASCADE,
  -- 再認証か紐付けを終えたときに戻す、画面のパス。ログインなら NULL。
  return_to text,
  -- ログイン中のユーザーを IdP のユーザーと紐付ける (設計書 9.4) なら true。session_id も、紐付けを始めたセッションにする。
  link boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);

CREATE INDEX oidc_logins_expires_at_idx ON oidc_logins (expires_at);

-- パスキー (設計書 9.3)。第二要素にも、単体でのログインにも使う。
CREATE TABLE user_passkeys (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- 認証器が決めたクレデンシャルの ID。ログインのときに、これでパスキーを引く。
  credential_id bytea NOT NULL UNIQUE,
  -- go-webauthn の Credential を JSON にしたもの。公開鍵、署名の回数、フラグを持つ。
  credential jsonb NOT NULL,
  -- 登録したときの RP ID (接続したホスト名)。パスキーは、このホスト名でしか使えない。
  rp_id text NOT NULL,
  name text NOT NULL,
  created_at timestamptz NOT NULL,
  last_used_at timestamptz
);

CREATE INDEX user_passkeys_user_id_idx ON user_passkeys (user_id);

-- パスキーの登録、パスキーだけでのログイン、パスキーでの再認証の手続きの途中の状態。Cookie に入れたトークンで引く。
-- パスワードの後の第二要素としての手続きは、login_challenges の webauthn_session に持つ。
CREATE TABLE webauthn_ceremonies (
  id uuid PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE,
  kind text NOT NULL CHECK (kind IN ('registration', 'login', 'reauthentication')),
  -- 登録や再認証をするユーザー。ログインでは、どのユーザーかを認証器が答えるまで分からないので空。
  user_id uuid REFERENCES users (id) ON DELETE CASCADE,
  -- go-webauthn の SessionData を JSON にしたもの。チャレンジを持つ。
  session jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);

CREATE INDEX webauthn_ceremonies_expires_at_idx ON webauthn_ceremonies (expires_at);

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

-- 画面から変える、全体の設定 (設計書 11.2)。1 行だけ持つ。行がなければ、既定値を使う。
CREATE TABLE system_settings (
  id smallint PRIMARY KEY CHECK (id = 1),
  -- 録画設定の全体の既定値 (設計書 4.7)。ルールの上書きと同じ形で、Go の既定値に重ねる。
  -- 新しい項目を足しても、古い行は Go の既定値のまま読める。
  recording_defaults jsonb NOT NULL,
  -- 過去の番組の保持期間 (日)。0 なら無期限 (設計書 3.4)。
  archive_retention_days integer NOT NULL CHECK (archive_retention_days >= 0),
  updated_at timestamptz NOT NULL
);

-- 通知するイベント (設計書 6.2)。各ロールが、出来事を書くトランザクションの中で記録する。
-- nekonote の配信のジョブが、ユーザーごとの通知に配り、配った日時を記録する。
CREATE TABLE notification_events (
  id uuid PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN (
    'recording_started', 'recording_completed', 'recording_failed',
    'reservation_conflict', 'storage_low', 'tuner_server_unreachable'
  )),
  occurred_at timestamptz NOT NULL,
  -- 画面に出す文の材料 (番組名や理由など)。種類ごとに項目が違う。
  payload jsonb NOT NULL,
  -- ユーザーごとの通知に配った日時。まだ配っていなければ NULL。
  dispatched_at timestamptz,
  -- 受け取るユーザー。NULL なら、有効なユーザー全員に届ける。手動の予約の録画は、予約したユーザーだけに届ける (設計書 6.2)。
  audience uuid[],
  -- ルール予約の録画の通知かどうか。ルール予約の録画の通知を受け取らない設定のユーザーには届けない。
  by_rule boolean NOT NULL DEFAULT false
);

-- 配っていないイベントを、古い順に引くのに使う。
CREATE INDEX notification_events_pending_idx ON notification_events (occurred_at) WHERE dispatched_at IS NULL;

-- 画面内の通知 (設計書 6.2)。既読はユーザーごとに管理する。
CREATE TABLE notifications (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  event_id uuid NOT NULL REFERENCES notification_events (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL,
  read_at timestamptz,
  UNIQUE (user_id, event_id)
);

CREATE INDEX notifications_user_id_created_at_idx ON notifications (user_id, created_at DESC, id DESC);

-- イベントの種類ごとの通知設定 (設計書 6.2)。行がなければ、どの通知先でも受け取る。
CREATE TABLE notification_preferences (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN (
    'recording_started', 'recording_completed', 'recording_failed',
    'reservation_conflict', 'storage_low', 'tuner_server_unreachable'
  )),
  -- 画面内の通知で受け取るかどうか。
  in_app boolean NOT NULL,
  -- Web Push で受け取るかどうか。購読していなければ届かない。
  web_push boolean NOT NULL,
  -- ルール予約の通知も受け取るかどうか。録画の通知と競合の通知でだけ使う。
  rule_reservations boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, kind)
);

-- ユーザーの設定の版数 (設計書 9.1)。ユーザーが自分の設定を変えるたびに上げ、同じユーザーのほかの端末に、subscription で知らせる。
CREATE TABLE user_setting_versions (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  version bigint NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 端末をまたいで同期する、プレイヤーの設定 (設計書 9.1)。行がなければ既定値を使う。
CREATE TABLE user_settings (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  -- 実況のコメントを出すかどうか。出さなければ、NX-Jikkyo に接続しない (設計書 7.4)。
  jikkyo_enabled boolean NOT NULL DEFAULT true,
  -- 字幕と文字スーパーを出すかどうか。
  captions_enabled boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL
);

-- Web Push の購読 (設計書 6.2)。ブラウザの PushSubscription を、ユーザーごとに持つ。
CREATE TABLE web_push_subscriptions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- プッシュサービスの送り先の URL。端末とブラウザごとに 1 つ決まる。
  endpoint text NOT NULL UNIQUE,
  -- 通知を暗号化する、ブラウザの P-256 の公開鍵 (非圧縮の 65 バイト) と、認証用の秘密 (16 バイト) (RFC 8291)。
  p256dh bytea NOT NULL,
  auth bytea NOT NULL,
  -- 購読した端末を、ユーザーが見分けるための User-Agent。
  user_agent text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE INDEX web_push_subscriptions_user_id_idx ON web_push_subscriptions (user_id);

-- Web Push の送り手を示す VAPID の鍵 (設計書 6.2、RFC 8292)。行は 1 つだけ持ち、初めて使うときに作る。
CREATE TABLE vapid_keys (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  -- P-256 の公開鍵 (非圧縮の 65 バイト)。ブラウザが購読するときの applicationServerKey に渡す。
  public_key bytea NOT NULL,
  -- 秘密鍵。アプリの秘密キーで暗号化する (設計書 8.4)。
  private_key_ciphertext bytea NOT NULL,
  created_at timestamptz NOT NULL
);

-- 管理者が設定する Webhook (設計書 6.2)。通知するイベントを、外部のサービスに送る。
CREATE TABLE webhooks (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  url text NOT NULL,
  -- 送る形式。汎用の JSON、Discord、Slack のいずれか。
  format text NOT NULL CHECK (format IN ('json', 'discord', 'slack')),
  -- 送るイベントの種類。
  kinds text[] NOT NULL,
  -- 署名用の秘密キー。アプリの秘密キーで暗号化する (設計書 8.4)。
  secret_ciphertext bytea NOT NULL,
  enabled boolean NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
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
  -- 詳細 (見出しから本文への対応)、ジャンル、関連する番組 (イベント共有とイベントリレー)、音声の構成は、上流の JSON のまま持つ。
  extended jsonb NOT NULL,
  genres jsonb NOT NULL,
  related_items jsonb NOT NULL,
  audios jsonb NOT NULL DEFAULT '[]',
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
  audios jsonb NOT NULL DEFAULT '[]',
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

-- 録画の mikan への割り振り (設計書 1.5)。neko が録画開始の少し前に書き、mikan が行をロックして取り、リースを延長しながら録画する。
CREATE TABLE recording_assignments (
  id uuid PRIMARY KEY,
  reservation_id uuid NOT NULL UNIQUE REFERENCES reservations (id) ON DELETE CASCADE,
  -- チューナーサーバーに渡す優先度 (設計書 1.6)。競合すると判定された録画は低くする。
  priority integer NOT NULL,
  -- 状態は、mikan を待つ (waiting)、録画中 (recording)、引き継ぎ待ち (handover)、終わった (finished) のどれか。
  state text NOT NULL CHECK (state IN ('waiting', 'recording', 'handover', 'finished')),
  -- mikan が取るたびに上げる世代番号。古い mikan は、リースの延長に失敗した時点で書き込みをやめる。
  generation bigint NOT NULL,
  -- 担当の mikan のインスタンスと、リースの期限。誰も取っていなければ NULL。
  holder uuid,
  lease_expires_at timestamptz,
  -- ストリームを開いた mikan の世代番号。引き継ぎ待ちの古い mikan は、これが自分の世代より進んだら、ストリームを閉じる (設計書 5.4)。
  stream_generation bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
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

-- 保存先プールの設定 (設計書 5.6、11.2)。マウント先は各ロールの設定で与え、優先順位と容量の閾値を DB に置く。
-- 行がなければ、優先順位 0、容量の閾値 0 (容量の管理をしない) として扱う。
CREATE TABLE storage_pools (
  id text PRIMARY KEY,
  -- 選ばれる優先順位。大きいほど先に選ぶ。
  priority integer NOT NULL,
  -- 容量の閾値 (バイト)。空き容量がこれを下回ったら、保護されていない古い録画から消す。0 なら消さない。
  min_free_bytes bigint NOT NULL CHECK (min_free_bytes >= 0),
  -- 古い録画を消しても、空き容量が閾値を下回ったままになった日時。不足を何度も通知しないために使う。
  low_since timestamptz,
  updated_at timestamptz NOT NULL
);

-- 録画 (設計書 5.5、8.2)。1 つの予約の録画は、引き継ぎや再接続でファイルが分かれても、1 つの録画にまとめる。
CREATE TABLE recordings (
  id uuid PRIMARY KEY,
  -- 録画した予約。予約を消しても録画は残すので、NULL にする。
  reservation_id uuid UNIQUE REFERENCES reservations (id) ON DELETE SET NULL,
  service_id bigint NOT NULL REFERENCES services (id),
  -- 番組の eventId。時刻予約では NULL。
  event_id bigint,
  -- 番組情報。録画を始めたときの予約からコピーし、放送後に確定したものに置き換える。
  name text NOT NULL,
  description text NOT NULL,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  -- 状態は、録画中 (recording)、完了 (completed)、エラーあり完了 (completed_with_errors)、一部のみ (partial)、失敗 (failed) のどれか。
  state text NOT NULL CHECK (state IN ('recording', 'completed', 'completed_with_errors', 'partial', 'failed')),
  -- 失敗の理由 (設計書 5.5)。失敗していなければ NULL。
  failure_reason text CHECK (failure_reason IN (
    'tuner_unavailable', 'tuner_server_unreachable', 'stream_interrupted', 'storage_unavailable',
    'storage_full', 'program_not_found', 'cancelled'
  )),
  -- 保護の印。容量の管理で消さない (設計書 6.1)。
  protected boolean NOT NULL DEFAULT false,
  -- ドロップ、エラー、スクランブルの集計 (設計書 5.3)。全 PID の合計。
  drops bigint NOT NULL DEFAULT 0,
  errors bigint NOT NULL DEFAULT 0,
  scrambled bigint NOT NULL DEFAULT 0,
  -- 途切れた区間 (設計書 5.2)。[{"from": ..., "to": ...}] の形で、ストリームを開き直すまでの間を記録する。
  gaps jsonb NOT NULL DEFAULT '[]',
  -- 録画を始めたときの番組の音声の構成 (programs.audios の写し)。時刻予約や、番組がないときは空にする。
  audios jsonb NOT NULL DEFAULT '[]',
  -- 録画中に番組が EPG から消えたかどうか (設計書 5.1)。消えたら、予定の終わりまで録り続ける。
  program_lost boolean NOT NULL DEFAULT false,
  -- 録画中の TS の EIT[p/f] で、予約した番組が放送中になった日時と、放送中でなくなった日時 (設計書 5.1)。分からなければ NULL。
  broadcast_started_at timestamptz,
  broadcast_ended_at timestamptz,
  -- 予約した番組が、予定の終わりまで EIT[p/f] の放送中にも次の番組にも出てこなかったかどうか。録った TS は残す。
  not_broadcast boolean NOT NULL DEFAULT false,
  -- not_broadcast のとき、予定の時間に放送されていた番組の eventId。
  aired_event_id bigint,
  -- 実際に録画を始めた日時と、終えた日時。録画中なら終えた日時は NULL。
  recorded_from timestamptz NOT NULL,
  recorded_until timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 録画のファイル (設計書 5.2、5.6、8.2)。セグメントは、ストリームを開き直すたびに作る。索引は、セグメントごとの付属ファイル。
-- 代表画像とプレビュー画像は、録画を終えた後に nekonote が作り、最初のセグメントの付属ファイルのディレクトリに置く。
CREATE TABLE recording_files (
  id uuid PRIMARY KEY,
  recording_id uuid NOT NULL REFERENCES recordings (id) ON DELETE CASCADE,
  -- 種類は、TS のセグメント (segment)、セグメントの索引 (index)、代表画像 (thumbnail)、シークバー用のプレビュー画像 (preview)、
  -- その他のファイル (other) のどれか。その他のファイルは、EPGStation から取り込んだ、エンコード済みの MP4 のような TS 以外のファイル (設計書 6.3)。
  kind text NOT NULL CHECK (kind IN ('segment', 'index', 'thumbnail', 'preview', 'other')),
  -- 保存先プールの ID と、プール内の相対パス (設計書 5.6)。
  pool_id text NOT NULL,
  path text NOT NULL,
  size bigint NOT NULL,
  -- ファイルが表す時間の範囲。書いている間は、終わりを NULL にする。
  started_at timestamptz NOT NULL,
  ended_at timestamptz,
  created_at timestamptz NOT NULL,
  UNIQUE (pool_id, path)
);

CREATE INDEX recording_files_recording_id_idx ON recording_files (recording_id, started_at);

-- 録画を一覧で新しい順に並べ、ページを送るのに使う。
CREATE INDEX recordings_recorded_from_id_idx ON recordings (recorded_from DESC, id DESC);

-- 録画のタグ (設計書 8.2)。名前は、前後の空白を落として重ならないようにする。
CREATE TABLE tags (
  id uuid PRIMARY KEY,
  name text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL
);

CREATE TABLE recording_tags (
  recording_id uuid NOT NULL REFERENCES recordings (id) ON DELETE CASCADE,
  tag_id uuid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
  PRIMARY KEY (recording_id, tag_id)
);

-- タグで録画を絞り込むのに使う。
CREATE INDEX recording_tags_tag_id_idx ON recording_tags (tag_id, recording_id);

-- ユーザーごと、録画ごとの再生位置 (設計書 8.2)。続きから再生するのに使う。
CREATE TABLE watch_progress (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  recording_id uuid NOT NULL REFERENCES recordings (id) ON DELETE CASCADE,
  -- 録画の始まりからの再生位置 (ミリ秒)。
  position_ms bigint NOT NULL CHECK (position_ms >= 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, recording_id)
);

-- 録画を消すときに、その録画の再生位置を消すのに使う。
CREATE INDEX watch_progress_recording_id_idx ON watch_progress (recording_id);

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

-- ほかの録画ソフトから取り込んだデータの、取り込み元の ID と、取り込んで作った行の対応 (設計書 6.3)。
-- 取り込みを何度実行しても、同じデータを二重に取り込まないために使う。
-- 取り込んだ後に Kotatsu で消した行も、取り込み直さないよう、対応は残す。そのため target_id には外部キーを付けない。
CREATE TABLE import_sources (
  -- 取り込み元のソフト。今は epgstation だけ。
  source text NOT NULL,
  -- 取り込み元のデータの種類 (rule、reserve、recorded、recorded_history)。
  kind text NOT NULL,
  -- 取り込み元での ID。
  source_id bigint NOT NULL,
  -- 取り込んで作った行の ID。種類によって、ルール、予約、録画、録画履歴のどれかを指す。
  target_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (source, kind, source_id)
);
