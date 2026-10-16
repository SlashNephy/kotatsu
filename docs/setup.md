# セットアップ

Kotatsuを動かすには、PostgreSQLと、チューナーサーバーが1つ必要です。
チューナーサーバーには、Mirakurun、mirakc、Mahironのどれかを使えます。
CIでは、PostgreSQL 17で動作を確認しています。

このドキュメントの手順は、Docker ComposeとHelmの2通りです。
設定の項目は[設定](configuration.md)に、運用の注意点は[運用](operations.md)にまとめました。

## コンテナイメージ

コンテナイメージは`ghcr.io/slashnephy/kotatsu`で配布しています。

| タグ | 内容 |
| --- | --- |
| `edge` | mainの最新版。FFmpegを含まない |
| `edge-ffmpeg` | mainの最新版。FFmpegとtsreadexを含む |
| `<バージョン>` | リリースした版。FFmpegを含まない |
| `<バージョン>-ffmpeg` | リリースした版。FFmpegとtsreadexを含む |

サムネイルを作るnekonoteと、サーバー側でトランスコードするkyusuは、FFmpegを使います。
1つのプロセスで全ロールを動かすときは、`-ffmpeg`のイメージを使ってください。

コンテナは、UID 65532の一般ユーザーで動きます。
ルートファイルシステムを読み取り専用にしても動きます。

## Docker Composeで動かす

[deploy/compose/compose.yaml](../deploy/compose/compose.yaml)は、PostgreSQLとKotatsuを1台のマシンで動かす例です。
Kotatsuは、全ロールを1つのコンテナで動かします。

まず、作業用のディレクトリにcompose.yamlを置きます。

```bash
mkdir kotatsu && cd kotatsu
curl -fsSLO https://raw.githubusercontent.com/SlashNephy/kotatsu/main/deploy/compose/compose.yaml
```

次は、アプリの秘密キーの作成です。
秘密キーをなくすと、TOTPとWeb Pushの設定し直しと、Webhookの作り直しが必要になります。
作ったファイルは、必ずバックアップしてください。

```bash
openssl rand -base64 48 > secret_key.txt
chmod 644 secret_key.txt
```

録画を置くディレクトリは、UID 65532が書き込めるようにしておきます。

```bash
mkdir recordings
sudo chown 65532:65532 recordings
```

compose.yamlの`KOTATSU_TUNER_URL`を、チューナーサーバーのURLに書き換えてください。
ほかの設定を変えるときも、`environment`に`KOTATSU_`で始まる環境変数を足します。

最後に、起動します。

```bash
docker compose up -d
```

`kotatsu`のサービスは、`migrate`のサービスがDBのスキーマを作り終えてから起動します。
更新するときも、`docker compose pull`の後に同じコマンドを実行すれば、スキーマを更新してから起動し直します。

続けて、[最初の管理者を作る](#最初の管理者を作る)に進んでください。

## Helmで動かす

[charts/kotatsu](../charts/kotatsu)のHelm Chartは、Kubernetesで動かすためのものです。
PostgreSQLはChartに含めません。
[CloudNativePG](https://cloudnative-pg.io/)などで、別に用意してください。

### Secretを作る

Chartは、DBの接続先とアプリの秘密キーを、既存のSecretから読みます。
Secretは、Chartを入れる前に作っておいてください。

```bash
kubectl create secret generic kotatsu \
  --from-literal=database-url='postgres://kotatsu:<パスワード>@<ホスト>:5432/kotatsu' \
  --from-literal=secret-key="$(openssl rand -base64 48)"
```

Secretの名前とキーは、values.yamlの`secrets`で変えられます。

### values.yamlを書く

Chartは、`mode`で2つの構成を選べます。

| `mode` | 構成 |
| --- | --- |
| `all` | 全ロールを1つのPodで動かす。既定値 |
| `split` | ロールごとにDeploymentを分け、レプリカ数を個別に決める |

次は、`all`の構成で、NFSのプールを1つ使う例です。

```yaml
mode: all

config:
  TUNER_URL: http://mirakurun.example.internal:40772/

storage:
  pools:
    - id: main
      volume:
        nfs:
          server: nas.example.internal
          path: /export/recordings
```

`config`には、環境変数`KOTATSU_`の後ろの名前と値を書きます。
`config`の値は、Deploymentの環境変数にそのまま載ります。
資格情報を含むチューナーサーバーのURLと、OIDCのクライアントシークレットは、`secrets.tunerURL`と`secrets.oidcClientSecret`にSecretの名前とキーを書いて渡してください。
ほかの設定をSecretから渡すときは、`extraEnv`と`extraEnvFrom`に、Kubernetesの`env`と`envFrom`の形で書きます。

```yaml
secrets:
  tunerURL:
    name: kotatsu
    key: tuner-url
  oidcClientSecret:
    name: kotatsu
    key: oidc-client-secret
```

`KOTATSU_STORAGE_POOLS`は、`storage.pools`からChartが作るので、`config`には書きません。
新しい録画の付属ファイル（索引、代表画像、プレビュー画像）を、録画ファイルの横でなく別の場所にまとめたいときは、`storage.sidecar.volume`にvolumeを書きます。Chartが`KOTATSU_STORAGE_SIDECAR_DIR`を渡し、プールと同じロールにマウントします。すでにある付属ファイルは移しません。

Ingressを使うときは、`ingress.enabled`と一緒に`ingress.trustedProxies`を書きます。
Kotatsuは、ここに書いたネットワークから届いた`X-Forwarded-For`だけを信頼します。
普通は、Ingressのコントローラーが動くPodのネットワークのCIDRを書きます。
書かないと、Ingressを通る全リクエストが400になるので、Chartは描画を止めます。

```yaml
ingress:
  enabled: true
  className: nginx
  host: kotatsu.example.com
  trustedProxies:
    - 10.244.0.0/16
```

コントローラーの前で送信元のアドレスが書き換わる（SNAT）構成では、送信元がノードのアドレスに見え、接続元の制限（`server.allowed_networks`）が効きません。
たとえば、コントローラーのServiceが`externalTrafficPolicy: Cluster`だと、こうなります。
`externalTrafficPolicy: Local`にするなどして、送信元を保ってください。

`all`と`split`の例は、[deploy/kubernetes](../deploy/kubernetes)にも置いています。
同じディレクトリにある`all.yaml`と`split.yaml`は、その例から作ったマニフェストです。
Helmを使わずに、マニフェストを手で直して使うこともできます。

### インストールする

リリースした版のChartは、GitHub PagesのHelmリポジトリ（`https://slashnephy.github.io/kotatsu`）で配布しています。

```bash
helm repo add kotatsu https://slashnephy.github.io/kotatsu
helm repo update
helm install kotatsu kotatsu/kotatsu --version <バージョン> -f values.yaml
```

リリースしていないmainの版を使うときは、リポジトリのChartをそのまま指定してください。

```bash
helm install kotatsu ./charts/kotatsu -f values.yaml
```

インストールと更新の前に、Chartのフックが`kotatsu migrate`のJobを実行し、DBのスキーマを更新します。

続けて、[最初の管理者を作る](#最初の管理者を作る)に進んでください。

## 最初の管理者を作る

管理者がまだいないときは、セットアップトークンを使って最初の管理者を作ります。

全ロールを1つのプロセスで動かしているときは、起動時のログの`setup_token`にセットアップトークンが出ます。

```bash
docker compose logs kotatsu | grep setup_token
```

ロールを分けているときは、`kotatsu admin setup-token`でトークンを発行してください。

```bash
kubectl exec deploy/kotatsu-tenban -- kotatsu admin setup-token
```

ブラウザで`http://<ホスト>:8080/setup`を開き、トークンを入力して管理者を作ってください。
作った管理者でログインすると、認証アプリの登録を求められます。

## HTTPSについて

パスキーと、PWAとして動かすためのService Workerは、ブラウザがSecure Contextでしか使わせません。
Secure Contextになるのは、HTTPSで開いたときと、`localhost`で開いたときです。

LANの中で`http://192.168.x.x:8080/`のように開くと、パスキーでのログイン、Web Pushの通知、ホーム画面への追加を使えません。
パスワードとTOTPでのログインは、HTTPでも使えます。
これらの機能を使うときは、リバースプロキシやIngressでHTTPSを終端してください。

リバースプロキシを通すときは、そのアドレスを`server.trusted_proxies`に足してください。
足さないと、Kotatsuは`X-Forwarded-For`を信頼せず、エラーを返します。

## EPGStationから移行する

EPGStationからは、`kotatsu import epgstation`で移行できます。
手順はREADMEの[特徴](../README.md#特徴)を見てください。
