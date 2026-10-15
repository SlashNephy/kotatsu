# 設定

Kotatsuの設定は、YAMLのファイルか、`KOTATSU_`で始まる環境変数で渡します。
どちらで渡しても、使える項目は同じです。

家族が日常的に変える設定は、ここには含みません。
録画設定の既定値、EPGの保持期間、Webhook、通知などは、DBに保存し、KotatsuのAPIから変えます。

## 設定の読み込み

YAMLのファイルは、`--config`か環境変数`KOTATSU_CONFIG`でパスを渡します。

```bash
kotatsu all --config /etc/kotatsu/config.yaml
```

設定を読み込む順番は、次のとおりです。
同じ項目を何度も指定したときは、後のものを使います。

1. 既定値
2. YAMLのファイル
3. `KOTATSU_`で始まる環境変数
4. 名前が`_FILE`で終わる環境変数で指定したファイル

環境変数の名前は、YAMLのキーを`_`でつないで大文字にし、先頭に`KOTATSU_`を付けたものです。
たとえば、`database.url`は`KOTATSU_DATABASE_URL`になります。

値の形式は、項目の型で決まります。

| 型 | YAML | 環境変数 |
| --- | --- | --- |
| 文字列 | `listen: ":8080"` | `KOTATSU_SERVER_LISTEN=:8080` |
| 真偽値 | `pooler: true` | `KOTATSU_DATABASE_POOLER=true` |
| 一覧 | `scopes: [openid, email]` | `KOTATSU_AUTH_OIDC_SCOPES=openid,email` |
| 対応 | `pools: {main: /mnt/main}` | `KOTATSU_STORAGE_POOLS=main=/mnt/main` |

環境変数では、一覧と対応の要素をカンマで区切ってください。
対応の要素は「キー=値」の形で書きます。

### 秘密情報

下の表で「秘密」とした項目は、ファイルからも読み込めます。
環境変数の名前の末尾に`_FILE`を付け、値にファイルのパスを渡してください。
DockerのsecretsやKubernetesのSecretをマウントしたファイルを、そのまま使えます。

```bash
KOTATSU_AUTH_SECRET_KEY_FILE=/run/secrets/secret_key
```

ファイルの末尾の改行は無視します。
同じ項目に、`_FILE`の付いた環境変数と付いていない環境変数の両方を渡すと、Kotatsuは起動しません。

秘密情報の値は、ログやエラーメッセージに出しません。

## 項目

### server

tenbanのHTTPサーバーの設定です。

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `server.listen` | `KOTATSU_SERVER_LISTEN` | `:8080` | 待ち受けアドレス |
| `server.allowed_networks` | `KOTATSU_SERVER_ALLOWED_NETWORKS` | プライベートアドレス | 接続を受け付ける送信元のネットワーク |
| `server.trusted_proxies` | `KOTATSU_SERVER_TRUSTED_PROXIES` | なし | `X-Forwarded-For`を信頼するリバースプロキシのネットワーク |
| `server.client_ip_header` | `KOTATSU_SERVER_CLIENT_IP_HEADER` | `X-Forwarded-For` | 信頼済みのプロキシから届いたリクエストで、送信元を読むヘッダー（`X-Forwarded-For`か`X-Real-IP`） |
| `server.advertise_host` | `KOTATSU_SERVER_ADVERTISE_HOST` | なし | ほかのインスタンスからこのインスタンスに届くホスト |

`server.allowed_networks`と`server.trusted_proxies`には、CIDRかIPアドレスを書きます。

`server.allowed_networks`の既定値は、次のネットワークです。

- RFC 1918のプライベートアドレス（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`）
- ループバック（`127.0.0.0/8`、`::1/128`）
- Tailscaleが使う`100.64.0.0/10`
- IPv6のユニークローカルアドレス（`fc00::/7`）とリンクローカルアドレス（`fe80::/10`）

インターネットからの接続を受け付けるときは、`0.0.0.0/0`と`::/0`を明示してください。

`server.trusted_proxies`に含まれない送信元から`X-Forwarded-For`が届くと、Kotatsuはエラーを返します。
リバースプロキシを通すときは、そのアドレスを足してください。

`server.client_ip_header`が`X-Forwarded-For`のときは、`X-Forwarded-For`を右から読み、信頼済みのプロキシを飛ばした最初のアドレスを送信元にします。
`X-Forwarded-For`が無いときだけ、`X-Real-IP`を読みます。
プロキシが`X-Forwarded-For`を設定し直さないと、送信元が書いた値がそのまま届き、接続元の制限をすり抜けられます。
nginxでは、`X-Real-IP`だけでなく`X-Forwarded-For`も設定してください。

```nginx
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Host $host;
```

プロキシが`X-Real-IP`だけを設定するなら、`server.client_ip_header`を`X-Real-IP`にします。
このときは`X-Forwarded-For`を読まず、`X-Real-IP`の値を送信元にします。

`server.advertise_host`は、ロールを別のプロセスに分けたときに使います。
tenbanは、ライブ視聴と録画の配信を、別のプロセスのkyusuに中継します。
kyusuがこのホストをDBのインスタンスの一覧に登録し、tenbanはその一覧から中継先を決めます。
空のときは、待ち受けアドレスのホストを使います。
全アドレスで待ち受けているときは、ホスト名を使います。
Kubernetesではホスト名でPodに届かないので、PodのIPを渡してください。
Helm Chartは、この値に自動でPodのIPを入れます。

### metrics

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `metrics.listen` | `KOTATSU_METRICS_LISTEN` | `:9090` | `/metrics`の待ち受けアドレス |

`/metrics`は、APIとは別のアドレスで公開します。

### database

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `database.url` | `KOTATSU_DATABASE_URL` | なし（必須、秘密） | PostgreSQLの接続先 |
| `database.pooler` | `KOTATSU_DATABASE_POOLER` | `false` | PgBouncerなどのトランザクションプーリングを経由するかどうか |

`database.url`の形式は、`postgres://user:password@host:5432/kotatsu`です。

`database.pooler`を`true`にすると、KotatsuはPostgreSQLの`LISTEN`を使いません。
変更の通知とジョブの取得を、ポーリングだけで行います。

### telemetry

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `telemetry.otlp_endpoint` | `KOTATSU_TELEMETRY_OTLP_ENDPOINT` | なし | トレースを送るOTLP/HTTPの送信先 |

`http://collector:4318`のように、ベースのURLを書きます。
トレースの送り先は、末尾に`/v1/traces`を付けたURLです。
空のときは、トレースを送りません。

### audit

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `audit.retention_days` | `KOTATSU_AUDIT_RETENTION_DAYS` | `365` | 監査ログの保持期間（日） |

保持期間を過ぎた監査ログは、定期的に消します。

### storage

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `storage.pools` | `KOTATSU_STORAGE_POOLS` | なし | プールのIDから、マウント先のディレクトリへの対応 |

プールは、録画を保存するディレクトリです。
複数のディスクを、別々のプールとして登録できます。

```yaml
storage:
  pools:
    main: /mnt/main
    sub: /mnt/sub
```

```bash
KOTATSU_STORAGE_POOLS=main=/mnt/main,sub=/mnt/sub
```

DBには、録画のパスを「プールのID」と「プールの中の相対パス」で記録します。
そのため、ロールごとにマウント先が違っていても構いません。
同じプールは、録画を書くmikan、配信するtenbanとkyusu、解析するnekonoteのすべてにマウントしてください。

プールの優先順位と、残しておく空き容量は、DBに保存し、KotatsuのAPIから変えます。

### tuner

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `tuner.url` | `KOTATSU_TUNER_URL` | なし（秘密） | チューナーサーバーのURL |

`http://mirakurun:40772/`のように書きます。
BASIC認証が要るときは、URLに資格情報を含めてください。
空のときは、nekoが番組表を同期しません。

### auth

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `auth.secret_key` | `KOTATSU_AUTH_SECRET_KEY` | なし（秘密） | アプリの秘密キー |
| `auth.hibp.enabled` | `KOTATSU_AUTH_HIBP_ENABLED` | `false` | パスワードがHave I Been Pwnedで漏洩済みかを照会するかどうか |
| `auth.hibp.url` | `KOTATSU_AUTH_HIBP_URL` | `https://api.pwnedpasswords.com` | Pwned PasswordsのAPI |

`auth.secret_key`には、32バイト以上のランダムな文字列を渡します。
`openssl rand -base64 48`の出力を使ってください。
tenban、nekonote、kyusuは、これがないと起動しません。
ロールを別のプロセスに分けるときは、すべてのプロセスに同じ秘密キーを渡してください。

Kotatsuは、TOTPの秘密キーやWebhookの署名用の秘密キーを、この秘密キーで暗号化してDBに保存します。
秘密キーを変えたりなくしたりすると、暗号化した値を読めなくなります。
その場合は、TOTPとWeb Pushの設定し直しが必要です。

`auth.hibp.enabled`を`true`にすると、パスワードを設定するときに外部のAPIに照会します。
照会には、パスワードのハッシュの先頭5文字だけを送ります。

### auth.oidc

OpenID ConnectのIdPでログインするための設定です。
`auth.oidc.issuer`が空のときは、OIDCでのログインを受け付けません。

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `auth.oidc.issuer` | `KOTATSU_AUTH_OIDC_ISSUER` | なし | IdPのissuerのURL |
| `auth.oidc.client_id` | `KOTATSU_AUTH_OIDC_CLIENT_ID` | なし | クライアントID |
| `auth.oidc.client_secret` | `KOTATSU_AUTH_OIDC_CLIENT_SECRET` | なし（秘密） | クライアントシークレット |
| `auth.oidc.redirect_url` | `KOTATSU_AUTH_OIDC_REDIRECT_URL` | 接続したオリジン＋`/auth/oidc/callback` | IdPに登録したリダイレクトURI |
| `auth.oidc.scopes` | `KOTATSU_AUTH_OIDC_SCOPES` | `openid`、`profile`、`email` | IdPに求めるスコープ |
| `auth.oidc.display_name` | `KOTATSU_AUTH_OIDC_DISPLAY_NAME` | `OIDC` | ログイン画面のボタンに出すIdPの名前 |
| `auth.oidc.username_claim` | `KOTATSU_AUTH_OIDC_USERNAME_CLAIM` | `preferred_username` | 自動で作るユーザーのユーザー名にするclaim |

Kotatsuは、issuerの`/.well-known/openid-configuration`からエンドポイントを調べます。
`auth.oidc.scopes`には、`openid`を必ず含めてください。

自動作成を使わないときは、管理者があらかじめユーザーを作り、IdPのsubjectと紐付けておきます。

```bash
kotatsu user link-oidc <ユーザー名> <subject>
```

### auth.oidc.auto_create

OIDCで初めてログインしたユーザーを、自動で作るための設定です。

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `auth.oidc.auto_create.enabled` | `KOTATSU_AUTH_OIDC_AUTO_CREATE_ENABLED` | `false` | 自動で作るかどうか |
| `auth.oidc.auto_create.groups_claim` | `KOTATSU_AUTH_OIDC_AUTO_CREATE_GROUPS_CLAIM` | `groups` | ユーザーのグループを入れたclaim |
| `auth.oidc.auto_create.admin_groups` | `KOTATSU_AUTH_OIDC_AUTO_CREATE_ADMIN_GROUPS` | なし | 管理者にするグループ |
| `auth.oidc.auto_create.member_groups` | `KOTATSU_AUTH_OIDC_AUTO_CREATE_MEMBER_GROUPS` | なし | メンバーにするグループ |
| `auth.oidc.auto_create.viewer_groups` | `KOTATSU_AUTH_OIDC_AUTO_CREATE_VIEWER_GROUPS` | なし | 閲覧者にするグループ |

有効にしても、Kotatsuが作るのは、3つのグループの一覧のどれかに属するユーザーだけです。
グループのclaimには、文字列の配列か、1つの文字列を入れてください。

ロールは、属するグループの中で最も強いものにします。
ロールを決めるのはユーザーを作るときだけで、後のログインでは変えません。

### web_push

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `web_push.subject` | `KOTATSU_WEB_PUSH_SUBJECT` | KotatsuのリポジトリのURL | プッシュサービスに示す送り手の連絡先 |

`mailto:`か`https:`のURIを書きます。
プッシュサービスは、送り手に問題があるときに、ここに連絡します。

### thumbnail

nekonoteが、録画の代表画像と、シークバー用のプレビュー画像を作るときの設定です。

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `thumbnail.disable_preview` | `KOTATSU_THUMBNAIL_DISABLE_PREVIEW` | `false` | プレビュー画像を作らないかどうか |

性能の低いマシンでは、`true`にすると、代表画像だけを作って負荷を減らせます。

### kyusu

kyusuが、ライブ視聴と録画の再生のために、サーバー側でトランスコードするときの設定です。

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `kyusu.encoder` | `KOTATSU_KYUSU_ENCODER` | `x264` | 映像のエンコーダー |
| `kyusu.vaapi_device` | `KOTATSU_KYUSU_VAAPI_DEVICE` | `/dev/dri/renderD128` | VAAPIで使うデバイス |
| `kyusu.work_dir` | `KOTATSU_KYUSU_WORK_DIR` | OSの一時ディレクトリ | HLSのセグメントを置くディレクトリ |

`kyusu.encoder`には、次のどれかを書きます。

| 値 | エンコーダー |
| --- | --- |
| `x264` | ソフトウェアのx264（HEVCではx265）。GPUがなくても動く |
| `vaapi` | VAAPI（IntelとAMDのGPU） |
| `qsv` | Intel Quick Sync Video |
| `nvenc` | NVIDIA NVENC |

KubernetesでPodを動かすときは、`kyusu.work_dir`にemptyDirのパスを渡してください。

### jikkyo

ニコニコ実況のコメントを、ライブ視聴と録画の再生で表示するための設定です。
サービスと実況のチャンネルの対応表は、KonomiTVのものを同梱しています。

| YAML | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `jikkyo.channels` | `KOTATSU_JIKKYO_CHANNELS` | なし | 同梱した対応表に重ねる、サービスと実況のチャンネルの対応 |

キーには「ネットワークID:サービスID」を、値には`jk1`のような実況のチャンネルのIDを書きます。
同梱した対応表と同じサービスを書くと、こちらを優先します。

```yaml
jikkyo:
  channels:
    "4:101": jk101
    "32736:1024": jk1
```

環境変数では、カンマで区切って渡します。

```sh
KOTATSU_JIKKYO_CHANNELS=4:101=jk101,32736:1024=jk1
```
