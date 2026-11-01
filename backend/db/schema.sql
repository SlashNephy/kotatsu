-- Kotatsu のスキーマの原本。変更したら mise run //backend:db:diff でマイグレーションを生成する。

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
  -- MCP のクライアントを経由した操作では、クライアントの ID (クライアントのメタデータの URL。#964)。
  actor_client text,
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

-- 読み取り専用の API トークン (#431、設計書 9.8)。EPGStation 互換の読み取り API (/api/*) を、外のクライアントから使うのに発行する。
-- トークンは発行したときにだけ見せ、DB には SHA-256 だけを保存する。トークンは、発行したユーザーのロールのまま動く。
CREATE TABLE api_tokens (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- name は、どのクライアントに渡したかを見分けるための名前。1〜64 文字。
  name text NOT NULL,
  token_hash bytea NOT NULL UNIQUE,
  created_at timestamptz NOT NULL,
  -- 最後に使われた日時。書き込みを減らすため、一定の間隔でしか更新しない。使われていなければ NULL。
  last_used_at timestamptz
);

CREATE INDEX api_tokens_user_id_idx ON api_tokens (user_id);

-- MCP のクライアントの認可コード (#963、設計書 9.9)。同意したときに作り、トークンと引き換えたら消す。
-- 認可コードそのものは持たず、SHA-256 だけを保存する。
CREATE TABLE oauth_authorization_codes (
  code_hash bytea PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- client_id は、クライアントのメタデータの URL (CIMD)。client_name は、同意したときのクライアントの名前。
  client_id text NOT NULL,
  client_name text NOT NULL,
  redirect_uri text NOT NULL,
  -- code_challenge は、PKCE の S256 の値。
  code_challenge text NOT NULL,
  scopes text[] NOT NULL,
  -- resource は、トークンを使ってよい MCP サーバーの URL (RFC 8707)。
  resource text NOT NULL,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);

-- MCP のクライアントとの連携 (#963、設計書 9.9)。認可コードをトークンと引き換えたときに作り、
-- そこから入れ替えていくトークンの系統をまとめる。取り消すと行を消し、系統のトークンもまとめて消える。
CREATE TABLE oauth_grants (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  client_id text NOT NULL,
  client_name text NOT NULL,
  scopes text[] NOT NULL,
  resource text NOT NULL,
  created_at timestamptz NOT NULL,
  -- 最後にアクセストークンを使った日時。書き込みを減らすため、一定の間隔でしか更新しない。使われていなければ NULL。
  last_used_at timestamptz
);

CREATE INDEX oauth_grants_user_id_idx ON oauth_grants (user_id);

-- MCP のクライアントに発行したアクセストークンとリフレッシュトークン (#963、設計書 9.9)。SHA-256 だけを保存する。
CREATE TABLE oauth_tokens (
  token_hash bytea PRIMARY KEY,
  grant_id uuid NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('access', 'refresh')),
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  -- used_at は、リフレッシュトークンを使って入れ替えた日時。使い済みのものがもう一度来たら、盗まれたとみなして系統ごと取り消す。
  used_at timestamptz,
  CHECK (kind = 'refresh' OR used_at IS NULL)
);

CREATE INDEX oauth_tokens_grant_id_idx ON oauth_tokens (grant_id);
CREATE INDEX oauth_tokens_expires_at_idx ON oauth_tokens (expires_at);

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
  -- ID トークンの picture のクレーム (#778)。ログインするたびに書き直す。なければ NULL。
  -- 画像は、ブラウザに IdP から読ませず、tenban が中継する (/media/users/{id}/avatar)。
  picture text,
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
  -- 配り終えた通知の保持期間 (日)。既読か未読かにかかわらず、これより古い通知を消す。0 なら無期限 (#551)。
  notification_retention_days integer NOT NULL DEFAULT 90 CHECK (notification_retention_days >= 0),
  -- MCP のクライアントに、認可サーバーと MCP サーバーを使わせるかどうか (#963)。既定では使わせない。
  mcp_enabled boolean NOT NULL DEFAULT false,
  -- データ放送の文書の双方向の通信を、tenban が中継するかどうか (#501)。既定では中継しない。
  data_broadcast_ip_enabled boolean NOT NULL DEFAULT false,
  -- しょぼいカレンダー、Annict、AniList との連携 (アニメの番組と作品の結びつき) を使うかどうか (設計書 3.8)。
  -- 外部へ問い合わせるので、既定では使わない (設計書 0.3)。
  anime_linking_enabled boolean NOT NULL DEFAULT false,
  -- ルールを作るときの既定値 (重複の回避と期間、イベントリレーへの追従。#470)。変えた項目だけを持ち、Go の既定値に重ねる。
  rule_defaults jsonb NOT NULL DEFAULT '{}',
  -- 字幕の外字 (DRCS) を文字に置き換える表 (#825)。{"外字の画像の MD5 (小文字の 16 進)": "文字"} の形。
  drcs_replacements jsonb NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL
);

-- 通知するイベント (設計書 6.2)。各ロールが、出来事を書くトランザクションの中で記録する。
-- nekonote の配信のジョブが、ユーザーごとの通知に配り、配った日時を記録する。
CREATE TABLE notification_events (
  id uuid PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN (
    'recording_started', 'recording_completed', 'recording_failed',
    'reservation_conflict', 'storage_low', 'tuner_server_unreachable',
    'watch_reminder', 'anime_recorded', 'anime_record_failed'
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

-- 配り終えたイベントを、保持期間を過ぎたものから消すのに使う (#551)。
CREATE INDEX notification_events_dispatched_idx ON notification_events (occurred_at) WHERE dispatched_at IS NOT NULL;

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

-- 通知のイベントを消すと、ユーザーごとの通知も消える (ON DELETE CASCADE)。そのときにイベントで引く (#551)。
CREATE INDEX notifications_event_id_idx ON notifications (event_id);

-- イベントの種類ごとの通知設定 (設計書 6.2)。行がなければ、どの通知先でも受け取る。
CREATE TABLE notification_preferences (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN (
    'recording_started', 'recording_completed', 'recording_failed',
    'reservation_conflict', 'storage_low', 'tuner_server_unreachable',
    'watch_reminder', 'anime_recorded', 'anime_record_failed'
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

-- 再生中のプレイヤー (#459、設計書 7.7)。プレイヤーを開いた端末が登録し、状態を送り続ける。
-- 同じユーザーのほかの端末は、一覧から選んで操作する。last_seen_at が古いものは、閉じたとみなす。
-- セッションを消すと、そのセッションで開いたプレイヤーも消す。端末の名前は、セッションの User-Agent から作る。
CREATE TABLE players (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  session_id uuid NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
  -- 再生の状態 (何を見ているか、再生位置、音量など)。GraphQL の PlayerStateInput をそのまま入れる。
  state jsonb NOT NULL,
  started_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL
);

CREATE INDEX players_user_id_idx ON players (user_id, last_seen_at);

-- プレイヤーへの操作 (#459)。操作される端末は、届いた順に、前に読んだものより新しい操作を読む。古い操作は、送るときに消す。
CREATE TABLE player_commands (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  player_id uuid NOT NULL REFERENCES players (id) ON DELETE CASCADE,
  command jsonb NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE INDEX player_commands_player_id_idx ON player_commands (player_id, id);

-- ユーザーごとの、再生中のプレイヤーの一覧の版。登録、取り消し、知らせるべき状態の変化で上げる。
-- user_setting_versions と分けるのは、再生の状態が変わるたびに、設定の画面を読み直させないため。
CREATE TABLE player_versions (
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
  -- 視聴予約の通知を、放送の何分前に送るか (設計書 4.8)。
  watch_reminder_minutes integer NOT NULL DEFAULT 5 CHECK (watch_reminder_minutes BETWEEN 0 AND 60),
  -- 実況のコメントの表示とミュートの設定 (#488)。{opacity, speed, scale, showAll, mutedKeywords, mutedUsers} の形。NULL なら既定値。
  comment_settings jsonb,
  -- 字幕の見た目と、文字スーパーを出すかどうかの設定 (#491)。{superimposeLive, superimposeRecorded, stroke, backgroundOpacity} の形。NULL なら既定値。
  caption_settings jsonb,
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
  -- 詳細を、見出しの順に「見出し\n本文\n」とつないで NFKC で正規化したもの (設計書 3.5)。ルールの照合と番組の検索に使う。
  -- この列を足す前に書いた番組は NULL で、照合では extended から正規化し直す (#530)。
  extended_normalized text,
  -- 詳細 (見出しから本文への対応)、ジャンル、関連する番組 (イベント共有とイベントリレー)、音声の構成は、上流の JSON のまま持つ。
  extended jsonb NOT NULL,
  genres jsonb NOT NULL,
  related_items jsonb NOT NULL,
  audios jsonb NOT NULL DEFAULT '[]',
  -- 映像の解像度 (2160p、4320p など、Mirakurun の video.resolution)。上流が返さなければ NULL。BS4K と 8K を見分けるのに使う (#806)。
  video_resolution text,
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
  -- programs の extended_normalized を移したもの。番組の検索に使う。列を足す前に移した番組は NULL で、検索では extended から正規化し直す (#852)。
  extended_normalized text,
  extended jsonb NOT NULL,
  genres jsonb NOT NULL,
  related_items jsonb NOT NULL,
  audios jsonb NOT NULL DEFAULT '[]',
  video_resolution text,
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
-- 放送が終わって録画も終わった予約は、neko の追従 (設計書 4.6) が猶予の後に消し、EID の再利用で新しい番組を予約できなくならないようにする (#510)。
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
  updated_at timestamptz NOT NULL,
  -- 容量の管理 (設計書 6.1) が最後に測った、ファイルシステムの容量と空き容量 (バイト) と、測った日時。/metrics に出す (#957)。
  -- 測っていないか、設定から外したプールでは NULL。
  total_bytes bigint,
  free_bytes bigint,
  measured_at timestamptz
);

-- ユーザーがピン留めしたサービス (#485)。端末をまたいで同期し、ライブの一覧と番組表の「ピン留め」のタブに出す。
CREATE TABLE pinned_services (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, service_id)
);

-- サービスを消したときに、ピン留めを消すのに使う。
CREATE INDEX pinned_services_service_id_idx ON pinned_services (service_id);

-- 録画 (設計書 5.5、8.2)。1 つの予約の録画は、引き継ぎや再接続でファイルが分かれても、1 つの録画にまとめる。
CREATE TABLE recordings (
  id uuid PRIMARY KEY,
  -- EPGStation 互換の API (#431、設計書 9.8) で使う整数の ID。EPGStation から取り込んだ録画は、取り込み元の ID を引き継ぐ。
  api_id bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY UNIQUE,
  -- 録画した予約。予約を消しても録画は残すので、NULL にする。
  reservation_id uuid UNIQUE REFERENCES reservations (id) ON DELETE SET NULL,
  service_id bigint NOT NULL REFERENCES services (id),
  -- 番組の eventId。時刻予約では NULL。
  event_id bigint,
  -- 番組情報。録画を始めたときの予約からコピーし、放送後に確定したものに置き換える。
  name text NOT NULL,
  description text NOT NULL,
  -- NFKC で正規化した番組名と概要 (設計書 3.5)。録画の検索に使う。列を足す前の録画は、kotatsu migrate が埋める。
  -- 列を知らない古いバイナリが書いた録画では NULL になり、検索では正規化していない番組名と概要に当てる。
  name_normalized text,
  description_normalized text,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  -- 状態は、録画中 (recording)、完了 (completed)、エラーあり完了 (completed_with_errors)、一部のみ (partial)、失敗 (failed) のどれか。
  state text NOT NULL CHECK (state IN ('recording', 'completed', 'completed_with_errors', 'partial', 'failed')),
  -- 失敗の理由 (設計書 5.5)。失敗していなければ NULL。
  failure_reason text CHECK (failure_reason IN (
    'tuner_unavailable', 'tuner_server_unreachable', 'stream_interrupted', 'storage_unavailable',
    'storage_full', 'program_not_found', 'cancelled', 'recorder_unavailable'
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
  -- 録画を始めたときの番組の映像の解像度 (programs.video_resolution の写し)。分からなければ NULL。
  video_resolution text,
  -- 録画を始めたときの予約のジャンル (reservations.genres の写し)。予約を消しても、ジャンルで絞れるようにする (#468)。
  genres jsonb NOT NULL DEFAULT '[]',
  -- 録画を始めたときに予約に一致していたルール (最も古いもの) と、その名前 (#468)。ルールを消すと ID は NULL になり、名前は残る。
  -- 番組予約と時刻予約の録画は、どちらも NULL。
  rule_id uuid REFERENCES rules (id) ON DELETE SET NULL,
  rule_name text,
  -- 録画中に番組が EPG から消えたかどうか (設計書 5.1)。消えたら、予定の終わりまで録り続ける。
  program_lost boolean NOT NULL DEFAULT false,
  -- 録画中の TS の EIT[p/f] で、予約した番組が放送中になった日時と、放送中でなくなった日時 (設計書 5.1)。分からなければ NULL。
  broadcast_started_at timestamptz,
  broadcast_ended_at timestamptz,
  -- 番組が遅れて始まったので、それまでに書いた前の番組の TS を消し、放送中になるまで待ってから書き始めたかどうか (#1046)。
  -- 待った録画は、冒頭が欠けたかを、予定の開始でなく放送の開始と比べて確かめる。
  waited_for_broadcast boolean NOT NULL DEFAULT false,
  -- 予約した番組が、予定の終わりまで EIT[p/f] の放送中にも次の番組にも出てこなかったかどうか。録った TS は残す。
  -- 遅れて始まるのを待った番組が延長の上限まで放送中にならなかったときも立てる。このときは、前の番組の TS を残さない (#1046)。
  not_broadcast boolean NOT NULL DEFAULT false,
  -- not_broadcast のとき、予定の時間に放送されていた番組の eventId。
  aired_event_id bigint,
  -- 実際に録画を始めた日時と、終えた日時。録画中なら終えた日時は NULL。
  recorded_from timestamptz NOT NULL,
  recorded_until timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 失敗の理由ごとの、録画の失敗の累計と、最後に失敗した日時 (設計書 5.5、11.3、#960)。
-- 録画が失敗に変わった文の中で足す。録画を消しても減らないので、/metrics で増え続けるカウンターとして出せる。
-- マイグレーションが理由ごとに 0 件の行を作っておき、一度も失敗していない理由も 0 として出す。
CREATE TABLE recording_failure_counts (
  reason text PRIMARY KEY CHECK (reason IN (
    'tuner_unavailable', 'tuner_server_unreachable', 'stream_interrupted', 'storage_unavailable',
    'storage_full', 'program_not_found', 'cancelled', 'recorder_unavailable'
  )),
  count bigint NOT NULL DEFAULT 0,
  -- 一度も失敗していなければ NULL。
  last_failed_at timestamptz,
  -- 管理者が累計を 0 に戻した日時 (#1035)。戻していなければ NULL。カウンターの作成の時刻にして、受け手にリセットを伝える。
  reset_at timestamptz
);

-- 録画のファイル (設計書 5.2、5.6、8.2)。セグメントは、ストリームを開き直すたびに作る。
-- 索引、代表画像、プレビュー画像のような生成物は記録しない。データディレクトリの、セグメントのファイルの ID のディレクトリに置く。
CREATE TABLE recording_files (
  id uuid PRIMARY KEY,
  recording_id uuid NOT NULL REFERENCES recordings (id) ON DELETE CASCADE,
  -- 種類は、TS のセグメント (segment) か、その他のファイル (other)。
  -- その他のファイルは、EPGStation から取り込んだ、エンコード済みの MP4 のような TS 以外のファイル (設計書 6.3)。
  kind text NOT NULL CHECK (kind IN ('segment', 'other')),
  -- 保存先プールの ID と、プール内の相対パス (設計書 5.6)。
  pool_id text NOT NULL,
  path text NOT NULL,
  size bigint NOT NULL,
  -- ファイルが表す時間の範囲。書いている間は、終わりを NULL にする。
  started_at timestamptz NOT NULL,
  ended_at timestamptz,
  -- セグメントの PID ごとの品質の集計 (#473)。[{pid, kind, packets, drops, errors, scrambled}] の形で、セグメントを閉じるときに書く。
  -- セグメント以外と、書き終えていないセグメントと、これを足す前に閉じたセグメントは NULL。
  pid_stats jsonb,
  created_at timestamptz NOT NULL,
  UNIQUE (pool_id, path)
);

CREATE INDEX recording_files_recording_id_idx ON recording_files (recording_id, started_at);

-- 録画中の録画の、書いているセグメントの様子 (#829、設計書 5.3)。mikan が 10 秒ごとに書き、システムの状態の画面と
-- /metrics が読む。セグメントを閉じたら 0 に戻す (閉じたセグメントの数は、recordings と recording_files に足すため)。
-- 引き継ぎで 2 つの mikan が同じ録画を書く間は、後から書いた方が残る。
CREATE TABLE recording_live_stats (
  recording_id uuid PRIMARY KEY REFERENCES recordings (id) ON DELETE CASCADE,
  -- 書いているセグメントのバイト数と、ドロップ、エラー、スクランブルの数。
  bytes bigint NOT NULL,
  drops bigint NOT NULL,
  errors bigint NOT NULL,
  scrambled bigint NOT NULL,
  -- 直前の間隔で書いたビットレート (bps)。
  bitrate bigint NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 録画の CM の区間を見分けた記録 (設計書 6.1、#479)。nekonote のジョブが、録画を終えた後に、見分け方 (detector) ごとに書く。
-- 行があれば、見分けたことを表す (CM が見つからなくても書く)。見分け方を直したら版を上げ、古い版の録画を見分け直せるようにする。
CREATE TABLE recording_cm_detections (
  recording_id uuid NOT NULL REFERENCES recordings (id) ON DELETE CASCADE,
  detector text NOT NULL,
  detector_version integer NOT NULL,
  detected_at timestamptz NOT NULL,
  PRIMARY KEY (recording_id, detector)
);

-- 見分けた CM の区間。見分け直したら、同じ見分け方の区間をすべて置き換える。
-- 始まりと終わりは、録画を始めた日時 (recordings.recorded_from) からのミリ秒。途切れた区間も、時間に含めて数える。
CREATE TABLE recording_cm_sections (
  recording_id uuid NOT NULL,
  detector text NOT NULL,
  start_offset_ms bigint NOT NULL CHECK (start_offset_ms >= 0),
  end_offset_ms bigint NOT NULL,
  PRIMARY KEY (recording_id, detector, start_offset_ms),
  FOREIGN KEY (recording_id, detector) REFERENCES recording_cm_detections (recording_id, detector) ON DELETE CASCADE,
  CHECK (end_offset_ms > start_offset_ms)
);

-- 録画を一覧で新しい順に並べ、ページを送るのに使う。
CREATE INDEX recordings_recorded_from_id_idx ON recordings (recorded_from DESC, id DESC);

-- ルールで録画を絞るのと、ルールを消したときに録画の rule_id を NULL にするのに使う (#468)。
CREATE INDEX recordings_rule_id_idx ON recordings (rule_id) WHERE rule_id IS NOT NULL;

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
-- 条件は、ジャンルの包含と除外、番組の最短の長さ、無料放送だけかどうか、チャンネルの並び、番組のないチャンネルを隠すかどうか。
CREATE TABLE guide_views (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name text NOT NULL,
  -- 条件。ジャンルの包含と除外、最短の長さ、無料放送だけかどうか、チャンネルの並びを持つ。
  settings jsonb NOT NULL,
  hide_empty_services boolean NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX guide_views_user_id_name_idx ON guide_views (user_id, name);

-- 視聴ルール (設計書 4.8)。条件に一致する番組に、ルールの持ち主の視聴予約を自動で付ける。
-- 録画のルール (rules) は家族で共有するが、視聴ルールはユーザーごとの個人のものなので、分けて持つ。
-- 条件は、録画のルールと同じ形 (型付きの JSONB) で持ち、同じ照合関数で照合する。
CREATE TABLE watch_rules (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name text NOT NULL,
  enabled boolean NOT NULL,
  conditions jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

-- ユーザーごとに、作った順に引くのに使う。
CREATE INDEX watch_rules_user_id_created_at_idx ON watch_rules (user_id, created_at, id);

-- 視聴予約 (設計書 4.8)。録画はせず、放送の少し前に、予約したユーザーだけに通知する。
-- 録画の予約と違ってチューナーを使わない個人のものなので、reservations とは分ける。
-- 番組は (サービス, eventId) で持ち、放送時間が変わっても通知の時刻が付いていく。番組が EPG から消えたら消す。
CREATE TABLE watch_reservations (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  event_id bigint NOT NULL,
  created_at timestamptz NOT NULL,
  -- 視聴予約を作った視聴ルール。手で付けた視聴予約では NULL。
  -- 視聴ルールを消しても視聴予約は残すので、NULL にして、手で付けたものと同じく扱う。
  watch_rule_id uuid REFERENCES watch_rules (id) ON DELETE SET NULL,
  PRIMARY KEY (user_id, service_id, event_id)
);

-- 番組ごとに引く (通知のジョブと、番組が消えたときの片付け) のに使う。
CREATE INDEX watch_reservations_service_id_event_id_idx ON watch_reservations (service_id, event_id);

-- 視聴ルールを消したときに、作った視聴予約を引くのに使う。
CREATE INDEX watch_reservations_watch_rule_id_idx ON watch_reservations (watch_rule_id);

-- ユーザーが手で外したか、通知を送った視聴予約の番組 (設計書 4.8)。視聴ルールに一致しても、付け直さない。
-- 番組が programs からなくなったら (アーカイブに移ったか、EPG から消えたら)、照合のときに消す。
CREATE TABLE watch_rule_dismissals (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  event_id bigint NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, service_id, event_id)
);

-- キャプチャ (設計書 7.5、#908)。視聴画面で撮った画像を、撮ったユーザーだけのものとして残す。画像は、データディレクトリの
-- 「captures/ユーザーの ID/キャプチャの ID.png」に置き、行には撮った場面の情報を持つ。枚数の上限は設けない。
CREATE TABLE captures (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- 録画で撮ったときの録画。録画を消してもキャプチャは残すので、NULL にする。ライブとタイムシフトで撮ったものも NULL。
  recording_id uuid REFERENCES recordings (id) ON DELETE SET NULL,
  -- 録画で撮ったときの、録画の始まりからの再生位置 (ミリ秒)。撮った場面から再生するのに使う。
  position_ms bigint CHECK (position_ms >= 0),
  service_name text NOT NULL,
  -- 番組名。番組が分からなければ NULL。
  program_name text,
  -- NFKC で正規化したサービス名と番組名 (設計書 3.5)。キャプチャを探すのに使う。番組名が分からなければ空にする。
  service_name_normalized text NOT NULL,
  program_name_normalized text NOT NULL,
  -- 撮ったときに出ていた字幕の文 (#458)。出ていなければ NULL。
  caption text,
  -- 撮った場面の放送時刻と、撮った時刻 (端末の時計)。
  broadcast_at timestamptz NOT NULL,
  taken_at timestamptz NOT NULL,
  -- 画像の大きさ (バイト)。
  size bigint NOT NULL,
  created_at timestamptz NOT NULL
);

-- ユーザーのキャプチャを、新しい順に引くのに使う。
CREATE INDEX captures_user_id_created_at_idx ON captures (user_id, created_at DESC, id DESC);
-- 録画を消したときに、録画の参照を NULL にするのに使う。
CREATE INDEX captures_recording_id_idx ON captures (recording_id);

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

-- タイムシフトで常時録るチャンネルと、その保持時間 (設計書 5.7)。管理者が画面から設定する。行がなくなったチャンネルは、録るのをやめ、録ったチャンクも破棄する。
CREATE TABLE timeshift_channels (
  service_id bigint PRIMARY KEY REFERENCES services (id) ON DELETE CASCADE,
  -- どれだけ過去まで遡れるようにするか。範囲は 10 分から 24 時間。
  retention_seconds integer NOT NULL CHECK (retention_seconds BETWEEN 600 AND 86400),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  updated_by uuid REFERENCES users (id) ON DELETE SET NULL
);

-- タイムシフトの mikan への割り振り (設計書 5.7)。recording_assignments と同じ仕組みで、neko がチューナーの空きを見て書き、mikan が行をロックして取り、リースを延長しながら録る。
-- 録画と違って終わりがないので、行は予約ではなくサービスに紐づく。録るのをやめるときは、neko が行を消す。
CREATE TABLE timeshift_assignments (
  id uuid PRIMARY KEY,
  service_id bigint NOT NULL UNIQUE REFERENCES services (id) ON DELETE CASCADE,
  -- チューナーサーバーに渡す優先度 (設計書 1.6)。録画にも視聴にも譲るので低くする。
  priority integer NOT NULL,
  -- 状態は、mikan を待つ (waiting)、録っている (recording)、引き継ぎ待ち (handover)、終わった (finished) のどれか。
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

-- タイムシフトで録ったチャンク (設計書 5.7)。mikan が一定の長さで区切って書いたファイルを、1 行ずつ記録する。
-- ended_at が NULL のチャンクは、書いている途中。破棄のジョブは、保持時間を過ぎたものか、チャンネルの設定がなくなったものを消す。
CREATE TABLE timeshift_chunks (
  id uuid PRIMARY KEY,
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  -- タイムシフト用のディレクトリからの相対パス。
  path text NOT NULL UNIQUE,
  size bigint NOT NULL,
  started_at timestamptz NOT NULL,
  ended_at timestamptz,
  created_at timestamptz NOT NULL
);
CREATE INDEX timeshift_chunks_service_started_at_idx ON timeshift_chunks (service_id, started_at);

-- タイムシフトで録っている間の、放送中の番組の履歴 (設計書 5.7)。チャンクの時間軸に番組名を重ねて見せるのに使う。
-- 番組の変わり目を mikan が記録するので、ended_at が NULL の行が、今放送中の番組。
CREATE TABLE timeshift_programs (
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  started_at timestamptz NOT NULL,
  event_id bigint NOT NULL,
  name text NOT NULL,
  ended_at timestamptz,
  PRIMARY KEY (service_id, started_at)
);

-- タイムシフトのバッファから録画に昇格している、放送の終わった番組 (#1057、設計書 5.7)。nekonote のジョブが書き出し終えたら消す。
-- 行のある間は、破棄のジョブが、from_at から to_at までと重なるチャンクを消さない。neko も、録画中のまま残った録画として失敗にしない。
CREATE TABLE timeshift_promotions (
  recording_id uuid PRIMARY KEY REFERENCES recordings (id) ON DELETE CASCADE,
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  -- 書き出す範囲。番組の始まりから終わりに、前後のマージンを足したもの。
  from_at timestamptz NOT NULL,
  to_at timestamptz NOT NULL,
  -- 昇格したユーザー。完了の通知を届ける。ユーザーを消したら NULL にし、全員に届ける。
  created_by uuid REFERENCES users (id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL,
  -- 同じ番組を二重に昇格しない。
  UNIQUE (service_id, from_at)
);

-- しょぼいカレンダーのタイトルの一覧 (設計書 3.8)。しょぼいカレンダーで引けない番組の番組名を解析し、作品名から TID を引くのに使う。
-- nekonote のジョブが 1 日ごとに、前に取り込んでから更新されたタイトルを取り込む。
CREATE TABLE syobocal_titles (
  tid integer PRIMARY KEY,
  title text NOT NULL,
  -- 作品名を引く鍵。NFKC で正規化し、空白を除き、小文字にしたもの。
  title_key text NOT NULL,
  last_update timestamptz
);
CREATE INDEX syobocal_titles_title_key_idx ON syobocal_titles (title_key);

-- アニメの番組と作品の結びつき (設計書 3.8)。saya-definitions から取り込んだ、放送のサービスと、
-- しょぼいカレンダーのチャンネル (ChID)、Annict のチャンネルの対応。nekonote のジョブが 1 日ごとに取り込み直す。
-- saya-definitions は地上波のネットワーク ID を持たない (地域で違う) ので、地上波の行は network_id を 0 にし、
-- 種別とサービス ID で照らし合わせる。BS と CS は、ネットワーク ID とサービス ID で照らし合わせる。
CREATE TABLE anime_channel_definitions (
  channel_type text NOT NULL,
  network_id bigint NOT NULL,
  sid bigint NOT NULL,
  -- saya-definitions のチャンネルの名前。画面で、対応の出どころを示すのに使う。
  name text NOT NULL,
  syobocal_channel_id integer,
  annict_channel_id integer,
  PRIMARY KEY (channel_type, network_id, sid)
);

-- 管理者が画面で上書きした、サービスとしょぼいカレンダーの ChID、Annict のチャンネルの対応 (設計書 3.8)。
-- 項目ごとに上書きし、NULL の項目は取り込んだ対応を使う。取り込み直しても消さない。
CREATE TABLE anime_channel_overrides (
  service_id bigint PRIMARY KEY REFERENCES services (id) ON DELETE CASCADE,
  syobocal_channel_id integer CHECK (syobocal_channel_id > 0),
  annict_channel_id integer CHECK (annict_channel_id > 0),
  updated_at timestamptz NOT NULL,
  CHECK (syobocal_channel_id IS NOT NULL OR annict_channel_id IS NOT NULL)
);

-- arm から取り込んだ、しょぼいカレンダーの TID と、Annict、AniList、MyAnimeList の作品 ID の対応 (設計書 3.8)。
-- 1 つの TID に複数の作品が対応することがある (分割された作品など) ので、TID ごとに並びの番号 (position) を振る。
-- 取り込み元ごとに行を持ち、取り込み直すときは、その取り込み元の行だけを入れ替える。
CREATE TABLE anime_works (
  source text NOT NULL CHECK (source IN ('arm')),
  syobocal_tid integer NOT NULL,
  position smallint NOT NULL,
  annict_work_id integer,
  anilist_id integer,
  mal_id integer,
  PRIMARY KEY (source, syobocal_tid, position)
);
CREATE INDEX anime_works_syobocal_tid_idx ON anime_works (syobocal_tid);
-- 見たいリストの作品 (#928) から、TID を引くのに使う。
CREATE INDEX anime_works_annict_work_id_idx ON anime_works (annict_work_id) WHERE annict_work_id IS NOT NULL;
CREATE INDEX anime_works_anilist_id_idx ON anime_works (anilist_id) WHERE anilist_id IS NOT NULL;

-- 連携先の情報を取り込んだ様子 (設計書 3.8)。取り込み元ごとに 1 行。管理者の画面に出す。
CREATE TABLE anime_imports (
  source text PRIMARY KEY CHECK (source IN ('saya_definitions', 'arm', 'syobocal_titles')),
  -- 最後に取り込んだ内容の ETag。変わっていなければ、取り込み元は 304 を返し、読み直さない。
  etag text,
  -- 最後に問い合わせた日時。1 日ごとの取り込みは、これから決める。
  checked_at timestamptz NOT NULL,
  -- 最後に取り込めた日時と、そのときの件数。一度も取り込めていなければ NULL と 0。
  imported_at timestamptz,
  item_count integer NOT NULL,
  -- 最後の問い合わせが失敗した理由。成功したら NULL に戻す。
  error text
);

-- しょぼいカレンダーの放送時間データ (ProgLookup の ProgItem) のキャッシュ (設計書 3.8)。
-- 番組ごとではなく、ChID と期間ごとにまとめて引いたものを、PID ごとに持つ。放送時間は、db.php が返した StTime と EdTime のまま持つ。
CREATE TABLE syobocal_programs (
  pid integer PRIMARY KEY,
  tid integer NOT NULL,
  channel_id integer NOT NULL,
  start_at timestamptz NOT NULL,
  end_at timestamptz NOT NULL,
  -- StOffset (秒)。実際の放送は、start_at と end_at をこれだけずらしたもの。
  offset_seconds integer NOT NULL,
  -- 話数。話数のない放送 (特番など) では空。
  count text NOT NULL,
  -- 放送時間データのサブタイトル (複数話の放送では "#1～#2" の形) と、タイトルのサブタイトルの表から引いたサブタイトル。
  sub_title text NOT NULL,
  st_sub_title text NOT NULL,
  flag integer NOT NULL,
  deleted boolean NOT NULL,
  fetched_at timestamptz NOT NULL
);
CREATE INDEX syobocal_programs_channel_id_start_at_idx ON syobocal_programs (channel_id, start_at);
CREATE INDEX syobocal_programs_end_at_idx ON syobocal_programs (end_at);

-- ChID と日 (日本時間) ごとに、最後にしょぼいカレンダーの放送時間データを引いた日時 (設計書 3.8)。キャッシュが古いかを決めるのに使う。
CREATE TABLE syobocal_lookups (
  channel_id integer NOT NULL,
  -- 日本時間の日の始まり。
  day_start timestamptz NOT NULL,
  fetched_at timestamptz NOT NULL,
  PRIMARY KEY (channel_id, day_start)
);

-- 番組を照合した記録 (設計書 3.8)。どの話にも結びつかなかった番組も、照合したことを残す。
-- programs の主キーの EID は使い回され、番組はアーカイブへも移るので、programs とは別に、サービスと開始時刻で持つ。
-- アーカイブへ移す処理には手を入れずに、過去の番組の結びつきも残る。
CREATE TABLE program_anime_matches (
  service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
  start_at timestamptz NOT NULL,
  -- 照合したときの番組のイベント ID。放送時間が変わった番組の、前の結びつきを消すのに使う。
  event_id bigint NOT NULL,
  matched_at timestamptz NOT NULL,
  -- 放送が終わった後に照合し直し、結びつきを確定させた日時。確定させた番組は、それからは照合し直さない。
  finalized_at timestamptz,
  PRIMARY KEY (service_id, start_at)
);
CREATE INDEX program_anime_matches_service_id_event_id_idx ON program_anime_matches (service_id, event_id);

-- 番組に結びついた話 (設計書 3.8)。2 話連続の放送のように、1 つの番組が複数の話に結びつくので、番組と話は 1 対多にする。
-- Annict や AniList の作品 ID は持たず、TID から作品 ID の対応 (anime_works) を引く。arm が後から直っても、過去の番組まで直る。
CREATE TABLE program_anime_episodes (
  service_id bigint NOT NULL,
  start_at timestamptz NOT NULL,
  -- 番組の中での並びの番号。放送の順に 0 から振る。
  position smallint NOT NULL,
  -- 結びつけた出どころ。syobocal はしょぼいカレンダーの放送時間データ、title は番組名の解析。
  source text NOT NULL CHECK (source IN ('syobocal', 'title')),
  syobocal_tid integer,
  syobocal_pid integer,
  -- 話数の範囲。1 話だけなら同じ値。話数のない放送 (特番など) では NULL。
  count_start double precision,
  count_end double precision,
  subtitle text,
  -- しょぼいカレンダーの [新] と [終] の印。
  is_first boolean NOT NULL,
  is_last boolean NOT NULL,
  PRIMARY KEY (service_id, start_at, position),
  FOREIGN KEY (service_id, start_at) REFERENCES program_anime_matches (service_id, start_at) ON DELETE CASCADE
);

-- 番組の照合が読み終えた、EPG の変更の記録 (epg_changes) の版数 (設計書 3.8)。1 行だけ持つ。
CREATE TABLE anime_match_cursor (
  id smallint PRIMARY KEY CHECK (id = 1),
  epg_version bigint NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 録画に写した、番組の結びつき (設計書 3.8)。解像度と同じく (設計書 3.3)、録画を始めたときの番組の結びつきを写す。
-- EPGStation から取り込んだ録画は、nekonote のジョブが、サービスと開始時刻から照合して書く。行があれば、照合したか写したことを表す。
CREATE TABLE recording_anime_matches (
  recording_id uuid PRIMARY KEY REFERENCES recordings (id) ON DELETE CASCADE,
  matched_at timestamptz NOT NULL
);

-- 録画に結びついた話 (設計書 3.8)。列は program_anime_episodes と同じ。
CREATE TABLE recording_anime_episodes (
  recording_id uuid NOT NULL REFERENCES recording_anime_matches (recording_id) ON DELETE CASCADE,
  position smallint NOT NULL,
  source text NOT NULL CHECK (source IN ('syobocal', 'title')),
  syobocal_tid integer,
  syobocal_pid integer,
  count_start double precision,
  count_end double precision,
  subtitle text,
  is_first boolean NOT NULL,
  is_last boolean NOT NULL,
  PRIMARY KEY (recording_id, position)
);

-- 管理者が Annict と AniList に登録したアプリ (#927)。利用者は、このアプリで認可して、見たアニメを記録させる。
-- クライアントシークレットは、アプリの秘密キーで暗号化する (設計書 8.4)。行がなければ、その記録先には繋げない。
CREATE TABLE anime_record_apps (
  destination text PRIMARY KEY CHECK (destination IN ('annict', 'anilist')),
  client_id text NOT NULL,
  client_secret_encrypted bytea NOT NULL,
  updated_at timestamptz NOT NULL
);

-- 記録先への認可の途中の状態 (#927)。認可を始めたセッションのまま戻ってきたことを確かめるため、セッションを持つ。
-- state は、SHA-256 だけを持つ。戻ってきたら、引くと同時に消す。
CREATE TABLE anime_record_authorizations (
  id uuid PRIMARY KEY,
  state_hash bytea NOT NULL UNIQUE,
  session_id uuid NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  destination text NOT NULL CHECK (destination IN ('annict', 'anilist')),
  code_verifier text NOT NULL,
  -- 記録先に渡したリダイレクト URI。トークンと引き換えるときに、同じ値を渡す。
  redirect_uri text NOT NULL,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);
CREATE INDEX anime_record_authorizations_expires_at_idx ON anime_record_authorizations (expires_at);

-- ユーザーごとの、記録先との連携 (#927)。アクセストークンは、アプリの秘密キーで暗号化する (設計書 8.4)。
CREATE TABLE anime_record_connections (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  destination text NOT NULL CHECK (destination IN ('annict', 'anilist')),
  access_token_encrypted bytea NOT NULL,
  -- 記録先のアカウントの名前。設定の画面に、どのアカウントと連携したかを出す。
  account_name text NOT NULL,
  connected_at timestamptz NOT NULL,
  -- 見たいリストを取り込んだ様子 (#928)。最後に問い合わせた日時、最後に取り込めた日時、最後の問い合わせが失敗した理由。
  -- 一度も問い合わせていなければ NULL。取り込めたら、理由を NULL に戻す。
  wishlist_checked_at timestamptz,
  wishlist_imported_at timestamptz,
  wishlist_error text,
  -- 取り込めなかった見たいリストを、問い合わせ直す日時。記録先が 429 で Retry-After を返したら、その長さ (1 時間まで) だけ待つ。
  wishlist_retry_at timestamptz,
  PRIMARY KEY (user_id, destination)
);

-- ユーザーごとの、記録の設定 (#927)。行がなければ既定値を使う。
CREATE TABLE anime_record_settings (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  -- 番組の長さのうち、何 % を見たら記録するか。
  threshold_percent smallint NOT NULL DEFAULT 20 CHECK (threshold_percent BETWEEN 1 AND 100),
  -- 作品の状態を変えるかどうか。第 1 話 ([新]) を見たら「見てる」、最終話 ([終]) 以外を見たら「見てる」、最終話を見たら「見た」にする。
  watching_on_first boolean NOT NULL DEFAULT false,
  watching_on_episode boolean NOT NULL DEFAULT false,
  watched_on_last boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL
);

-- ユーザーが番組を見た区間 (#927)。プレイヤーが送った、実際に再生した放送の時刻の区間を、番組ごとに重ねずにまとめる。
-- 番組は、program_anime_episodes と同じく、サービスと開始時刻で指す。ライブ視聴と録画の再生で見た区間は、同じ行にまとめる。
CREATE TABLE anime_watch_progress (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  service_id bigint NOT NULL,
  start_at timestamptz NOT NULL,
  -- 見た区間。[[始まりの UNIX ミリ秒, 終わりの UNIX ミリ秒], ...] の形で、重ならないよう始まりの順に並べる。
  played jsonb NOT NULL,
  -- 見た区間の長さの合計 (ミリ秒)。
  played_ms bigint NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, service_id, start_at)
);
CREATE INDEX anime_watch_progress_updated_at_idx ON anime_watch_progress (updated_at);

-- 記録先に送る、1 話ごとの記録 (#927)。同じユーザー、番組、話、記録先の組は 1 回だけ記録する。
-- 送れたら recorded にし、見直しても送り直さない。送れなければ、River の再試行で送り直し、諦めたら failed にする。
CREATE TABLE anime_records (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  service_id bigint NOT NULL,
  start_at timestamptz NOT NULL,
  destination text NOT NULL CHECK (destination IN ('annict', 'anilist')),
  syobocal_tid integer NOT NULL,
  -- 話数。話数のない放送 (劇場版など) では NULL。
  episode_number double precision,
  subtitle text,
  is_first boolean NOT NULL,
  is_last boolean NOT NULL,
  -- 通知に出す番組名。
  program_name text NOT NULL,
  state text NOT NULL CHECK (state IN ('pending', 'recorded', 'failed')),
  -- 記録した作品かエピソードの、記録先のページ。
  url text,
  -- 記録できなかった理由。
  error text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE NULLS NOT DISTINCT (user_id, service_id, start_at, destination, syobocal_tid, episode_number)
);

-- ユーザーが Annict と AniList で「見たい」「見てる」にした作品 (#928)。記録先との連携 (#927) のトークンで、nekonote のジョブが取り込む。
-- 作品は、記録先の作品 ID だけを持ち、TID は使うときに作品 ID の対応 (anime_works) から引く。arm が後から直っても、候補まで直る。
-- 取り込み直すときは、ユーザーと記録先の行をすべて入れ替える。連携を外すと、行も消える。
CREATE TABLE anime_wishlist_entries (
  user_id uuid NOT NULL,
  destination text NOT NULL,
  -- 記録先の作品 ID。Annict の作品 ID か、AniList の ID。
  work_id integer NOT NULL,
  -- 見たい (wanna_watch。AniList の PLANNING) か、見てる (watching。AniList の CURRENT)。
  status text NOT NULL CHECK (status IN ('wanna_watch', 'watching')),
  -- 記録先の作品名。候補の見出しと、作るルールの名前に使う。
  title text NOT NULL,
  -- 次に見る話 (#1230)。Annict の nextEpisode から取り込む。AniList は話を持たないので、どれも NULL。
  -- next_episode_id は、Annict のエピソードのグローバル ID (createRecord に渡すもの)。
  next_episode_id text,
  next_episode_number integer,
  next_episode_number_text text,
  next_episode_title text,
  -- 見終えた話数 (#1230)。AniList は progress を、Annict は次に見る話の話数の 1 つ前を持つ。分からなければ NULL。
  -- 取り込みの間に kotatsu が記録を送れたら、その話数まで進める。
  progress integer CHECK (progress >= 0),
  -- 作品の全話数。分からなければ NULL。
  episodes_count integer,
  -- 作品の放送の時期。Annict の AUTUMN と AniList の FALL は、どちらも autumn にする。
  season_year integer,
  season_name text CHECK (season_name IN ('winter', 'spring', 'summer', 'autumn')),
  -- 作品の画像の URL。Annict の recommendedImageUrl か、AniList の coverImage.large。
  image_url text,
  fetched_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, destination, work_id),
  FOREIGN KEY (user_id, destination) REFERENCES anime_record_connections (user_id, destination) ON DELETE CASCADE
);
