# セットアップ

Kotatsuを動かすには、PostgreSQLと、チューナーサーバーが1つ必要です。
チューナーサーバーには、Mirakurun、mirakc、Mahironのどれかか、EDCBのEpgTimerSrvを使えます。EDCBを使うときは、[EDCBを使う](#edcbを使う)も読んでください。
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

### 配布物を確かめる

配布物には、このリポジトリのGitHub Actionsで作ったことを示す出所の証明（Sigstoreの署名）を付けています。
`checksums.txt`は壊れたファイルを見分けられますが、リリースごと差し替えられると、ハッシュも一緒に差し替えられてしまいます。
入れる前に、[GitHub CLI](https://cli.github.com/)で確かめられます。

```bash
# コンテナイメージ
gh attestation verify oci://ghcr.io/slashnephy/kotatsu:<バージョン> -R SlashNephy/kotatsu
# リリースに添えたアーカイブとHelm Chart
gh attestation verify kotatsu_<バージョン>_linux_amd64.tar.gz -R SlashNephy/kotatsu
```

コンテナイメージは`edge`と`2026.11.4`以降の版（タグに`v`は付きません）から、アーカイブとHelm Chartは`v2026.11.4`以降のリリースから、証明を付けています。

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

録画を置くディレクトリと、索引や代表画像などの生成物を置くディレクトリは、UID 65532が書き込めるようにしておきます。

```bash
mkdir recordings data
sudo chown 65532:65532 recordings data
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

[deploy/helm/kotatsu](../deploy/helm/kotatsu)のHelm Chartは、Kubernetesで動かすためのものです。
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
  data:
    volume:
      persistentVolumeClaim:
        claimName: kotatsu-data
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
録画の生成物（索引、代表画像、プレビュー画像）を置くデータディレクトリのvolumeを、`storage.data.volume`に必ず書きます。Chartが`KOTATSU_STORAGE_DATA_DIR`を渡し、プールと同じロールにマウントします。生成物は録画ファイルのプールに置かないので、プールをクラウドと同期していても混ざりません。

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

コントローラーの前で送信元のアドレスが書き換わる（SNAT）構成では、送信元がノードのアドレスに見え、接続元の制限（`server.allowed_private_networks`と`server.allowed_external_networks`）が効きません。
たとえば、コントローラーのServiceが`externalTrafficPolicy: Cluster`だと、こうなります。
`externalTrafficPolicy: Local`にするなどして、送信元を保ってください。

`all`と`split`の例は、[deploy/kubernetes](../deploy/kubernetes)にも置いています。
同じディレクトリにある`all.yaml`と`split.yaml`は、その例から作ったマニフェストです。
Helmを使わずに、マニフェストを手で直して使うこともできます。
マニフェストは`default`のnamespaceに入れる形で作っているので、別のnamespaceに入れるときは、各リソースの`namespace`も直してください。

CPUが飽和しがちなノードでは、応答が遅れただけでPodが再起動されないよう、プローブの待ち時間を延ばせます。
`probes`は全体の設定で、`all`や`roles`の各ロールの`probes`で項目ごとに上書きできます。

```yaml
probes:
  liveness:
    timeoutSeconds: 5
    periodSeconds: 30
    failureThreshold: 4
  readiness:
    timeoutSeconds: 5
```

### インストールする

リリースした版のChartは、GitHub PagesのHelmリポジトリ（`https://slashnephy.github.io/kotatsu`）で配布しています。

```bash
helm repo add kotatsu https://slashnephy.github.io/kotatsu
helm repo update
helm install kotatsu kotatsu/kotatsu --version <バージョン> -f values.yaml
```

リリースしていないmainの版を使うときは、リポジトリのChartをそのまま指定してください。

```bash
helm install kotatsu ./deploy/helm/kotatsu -f values.yaml
```

インストールと更新の前に、Chartのフックが`kotatsu migrate`のJobを実行し、DBのスキーマを更新します。

続けて、[最初の管理者を作る](#最初の管理者を作る)に進んでください。

## EDCBを使う

チューナーサーバーにEDCB（[xtne6f/EDCB](https://github.com/xtne6f/EDCB)）のEpgTimerSrvを使えます。
KotatsuはEpgTimerSrvとTCP（CtrlCmd）で話し、EPGの同期、チューナーの把握、録画、タイムシフト、ライブ視聴に使います。
録画とライブ視聴のTSは、EDCBのNetworkTVで受け取ります。
設定の項目は[設定](configuration.md#tuner)の`tuner.kind`と`tuner.edcb.*`です。

Kotatsuをつなぐ前に、EDCBの側で次のことを設定してください。

- EpgTimerSrvの設定で、TCPでの接続を有効にします（`EnableTCPSrv`と`TCPPort`。既定のポートは4510です）。
- 別のホストからつなぐときは、`TCPAccessControlList`にKotatsuのホストを入れます。既定は`+127.0.0.1,+192.168.0.0/16`です。一覧から外れたホストからの接続は、何も返さずに切られます。
- Windowsでは、EpgTimerSrvを初めて起動したときのダイアログで許可しないと、ファイアウォールに受信を遮断するルールが残ります。別のホストからつなぐ前に、EpgTimerSrvの受信を許可してください。
- `EpgTimerSrv.ini`の`[TVTEST]`に、Kotatsuのライブ視聴と録画に使うBonDriverを並べます。NetworkTVは、ここに並べたBonDriverしか使わないので、空のままだと1本も開けません。同じ名前を、Kotatsuの設定の`tuner.edcb.bon_drivers`にも並べてください。EpgTimerSrvはこの一覧をKotatsuに返さないので、Kotatsuはチューナーの一覧で使えるチューナーを見分けるのに、設定の一覧を使います。

次のことに注意してください。

- CtrlCmdのTCPには、パスワードも暗号化もありません。信頼できるネットワークの中でだけつないでください。
- NetworkTVのIDは、EpgTimerSrv全体で1つの名前空間です。Kotatsuが使う範囲（`tuner.edcb.network_tv_id_start`と`_count`。既定は1000から32個）を、ほかのクライアント（KonomiTVは500から、EpgTimerの視聴やTVTestは小さい番号）と重ならないようにしてください。
- KotatsuとEDCBは、チューナーを取り合います。EDCBの予約が始まると、EDCBはKotatsuが使っているNetworkTVのチューナーを取っていくので、Kotatsuの録画やライブ視聴が切れることがあります。Kotatsuはこれに気づいてつなぎ直しを試み、つなぎ直せなければ、録画の失敗の理由を「EDCBの予約にチューナーを取られた」として残します。
- EDCBはKotatsuの予約を知りません。EDCBの予約の割り当てや、EDCBのスリープの判断には、Kotatsuの予約は入りません。
- EDCBのNetworkTVは、ほかの利用者とチューナーを分け合いません。Kotatsuで同じ局を録画しながらライブで見ると、チューナーを2本使います。
- サービスのロゴは、EDCBが集めたもの（`LogoData`）を使います。EDCBがロゴを集めていなければ、Kotatsuが録画とタイムシフトのTSから集めたロゴを使います。

## WindowsとmacOSの電源の設定

WindowsとmacOSで`kotatsu all`を動かすと、録画、ライブ視聴、エンコードの間と次の録画の直前は、OSをスリープさせません。
次の録画の5分前には、スリープから起こす予約をOSに置きます。
切るときは、設定の`power.manage_sleep`を`false`にします（[設定](configuration.md#power)）。

- Windowsでは、起こす予約を、タスク スケジューラのタスク（`\Kotatsu\Wake`）として登録します。登録には管理者の権限が要るので、Kotatsuを管理者として動かしてください。管理者でなければ、スリープさせないことだけを行い、起動した後に1回だけ警告を出します。
- Windowsでは、電源オプションの詳細設定の「スリープ」で、「スリープ解除タイマーの許可」を有効にしてください。無効だと、予約を置いても起きません。
- macOSでは、起こす予約にrootが要ります。LaunchDaemonなどで、rootとして動かしてください。rootでなければ、スリープさせないことだけを行います。予約は`pmset -g sched`で確かめられます。
- macOSで止めるのは、操作が無いことで眠るのだけです。蓋を閉じたときと、メニューからスリープしたときは眠ります。
- 休止状態とシャットダウンからは起こせません。外から視聴することも、スリープを解きません。外からいつも観るなら、OSをスリープしない設定にしてください。
- タイムシフトを有効にしたサービスがあると、ずっとスリープしません。

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

ブラウザで`http://<ホスト>:8080/setup`を開き、トークンを入力し、[免責事項](../README.md#免責事項)に同意して管理者を作ってください。
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
ルール、手動予約、録画済みの番組とファイル、録画履歴を取り込みます。

- 取り込む前にKotatsuを起動し、チューナーサーバーからチャンネルを同期しておいてください。
- EPGStationの録画の保存先ディレクトリは、`--pool 名前=プールID`でKotatsuのプールに対応付けます。
- 取り込んだルールと予約の作成者には、`--user ユーザー名`で指定したユーザーを記録します。
- `--dry-run`を付けると、取り込む件数だけを表示します。

取り込んだ録画は、nekonoteがTSを読み直して索引とドロップの集計を作り、続けてサムネイルを作ります。

## EDCBから移行する

EDCB（EpgTimer、またはEDCBをバックエンドにしたKonomiTV）からは、`kotatsu import edcb --address host:port`で移行できます。
キーワード予約、手動の予約、録画済みの一覧（重複の回避に使う録画履歴として）を、EpgTimerSrvからTCPで読んで取り込みます。

- EpgTimerSrvのTCPでの接続を有効にし、Kotatsuのホストからつなげるようにしておいてください（[EDCBを使う](#edcbを使う)）。
- 取り込む前にKotatsuを起動し、チューナーサーバーからチャンネルと番組表を同期しておいてください。手動の予約は、Kotatsuの番組表にある番組だけを取り込みます。
- 取り込んだルールと予約の作成者には、`--user ユーザー名`で指定したユーザーを記録します。
- `--dry-run`を付けると、取り込む件数と、取り込めないものの理由だけを表示します。書き込みは巻き戻します。
- 何度実行しても、同じものを二重に取り込みません。
- 録画のファイルは移せません。キーワード予約のうち、Kotatsuの条件で表せないもの（有料だけの条件など）は取り込まず、理由を表示します。
