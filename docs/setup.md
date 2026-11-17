# セットアップ

Kotatsuを動かすには、PostgreSQLと、チューナーサーバーが1つ必要です。
チューナーサーバーには、Mirakurun、mirakc、Mahironのどれかか、EDCBのEpgTimerSrvを使えます。EDCBを使うときは、[EDCBを使う](#edcbを使う)も読んでください。
PostgreSQLは17以降に対応し、CIでは17と18で動作を確認しています。
SQLiteなどのほかのDBには対応しません。

このドキュメントの手順は、Docker Compose、Helm、バイナリの3通りです。
設定の項目は[設定](configuration.md)に、運用の注意点は[運用](operations.md)にまとめました。

## コンテナイメージ

コンテナイメージは`ghcr.io/slashnephy/kotatsu`で配布しています。

| タグ | 内容 |
| --- | --- |
| `edge` | mainの最新版。FFmpegを含まない |
| `edge-ffmpeg` | mainの最新版。FFmpegとtsreadexとwhisper.cppを含む |
| `<バージョン>` | リリースした版。FFmpegを含まない |
| `<バージョン>-ffmpeg` | リリースした版。FFmpegとtsreadexとwhisper.cppを含む |

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
定期のバックアップを書き出すなら、書き出す先のvolumeを`backup.volume`に書きます。Chartが`KOTATSU_BACKUP_DIR`を渡し、allかnekonoteにマウントします（[運用](operations.md#定期に書き出す)）。

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

コンテナには、既定でCPUとメモリのrequestsと、メモリのlimitsを付けています（値は[設計書の11.1](spec.v1.md#111-配布物)）。
同時に多くの変換を動かすkyusuや、大きなモデルで字幕を生成するnekonoteでメモリが足りなければ、`all.resources`か`roles.<ロール>.resources`で上げてください。

```yaml
roles:
  kyusu:
    resources:
      limits:
        memory: 8Gi
```

`all`と`split`の例は、[deploy/kubernetes](../deploy/kubernetes)にも置いています。
同じディレクトリにある`all.yaml`と`split.yaml`は、その例から作ったマニフェストです。
Helmを使わずに、マニフェストを手で直して使うこともできます。
マニフェストは`default`のnamespaceに入れる形で作っているので、別のnamespaceに入れるときは、各リソースの`namespace`も直してください。

`split`でチューナーサーバーにEDCBを使うときは、nabeを動かすと、同じ局のライブ視聴と録画でチューナーを分け合えます（[EDCBを使う](#edcbを使う)）。
nabeは既定では動かさないので、`roles.nabe.replicas`を2にします。
2台のnabeは同じチューナーのTSを配るので、1台が止まっても、録画とタイムシフトはもう1台から受け取り続けます（#1850）。
1でも動きますが、nabeが止まると、受け手が自分でNetworkTVを開き直します。
Mirakurun互換のチューナーサーバーでは、nabeは何もしないので、動かしません。

```yaml
mode: split

config:
  TUNER_KIND: edcb
  TUNER_EDCB_ADDRESS: edcb.example.internal:4510

roles:
  nabe:
    replicas: 2
```

ほかのロールは、インスタンスの登録簿でnabeのPodのIPを引いてつなぐので、nabeのServiceはありません。
`networkPolicy.enabled`を入れると、nabeは、tenban、mikan、kyusuのPodからだけ受けます。

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

## バイナリで動かす

[リリース](https://github.com/SlashNephy/kotatsu/releases)に、Linux、macOS、Windows向けのアーカイブ（amd64とarm64）を添えています。
バイナリで動かすときは、PostgreSQLを自分で用意してください。
ここでは、Kotatsuと同じマシンにPostgreSQLを入れる手順を、OSごとに書きます。
コマンドはPostgreSQLの公式の案内（[ダウンロードのページ](https://www.postgresql.org/download/)）に沿っています。

### PostgreSQLを入れる

KotatsuはPostgreSQL 17以降を使います。
OSのパッケージのPostgreSQLが16以前なら、PostgreSQLの開発者が配るリポジトリ（PGDG）から18を入れてください。

| OS | OSのパッケージの版 | 入れ方 |
| --- | --- | --- |
| Windows | なし | [EDBのインストーラー](#windows) |
| macOS | なし | [Homebrew](#macos) |
| Debian 13（trixie） | 17 | [OSのパッケージ](#debianubunturaspberry-pi-os) |
| Debian 12（bookworm） | 15 | [PGDG](#debianubunturaspberry-pi-os) |
| Ubuntu 26.04 | 18 | [OSのパッケージ](#debianubunturaspberry-pi-os) |
| Ubuntu 24.04、22.04 | 16、14 | [PGDG](#debianubunturaspberry-pi-os) |
| Raspberry Pi OS（trixie） | 17 | [OSのパッケージ](#debianubunturaspberry-pi-os) |
| RHEL、Rocky Linux、AlmaLinux 8〜10 | 16以前 | [PGDG](#rhel系) |

#### Windows

公式の案内にある、EDBのインストーラーで入れます。

1. [Windows向けのダウンロードのページ](https://www.postgresql.org/download/windows/)から、EDBのインストーラーの18を取ってきて、実行します。
2. 入れる部品は、「PostgreSQL Server」と「Command Line Tools」があれば足ります。「pgAdmin 4」と「Stack Builder」は要りません。
3. 途中で、スーパーユーザー（`postgres`）のパスワードを決めます。[DBと利用者を作る](#dbと利用者を作る)で使います。
4. ポートは既定の5432のままにします。

PostgreSQLはWindowsのサービスとして動きます。
既定の場所に入れると、コマンドは`C:\Program Files\PostgreSQL\18\bin`に入ります。

#### macOS

Homebrewで入れます。

```bash
brew install postgresql@18
brew services start postgresql@18
export PATH="$(brew --prefix postgresql@18)/bin:$PATH"
```

- `postgresql@18`は、ほかの版と並べて入れられるように、`PATH`に入りません（keg-only）。`createuser`などのコマンドを使う前に、3行目で`PATH`に足してください。
- 入れるときに、HomebrewがDBのデータ（クラスター）を作ります。スーパーユーザーは、Homebrewで入れたmacOSのユーザーです。
- `brew services start`は、今と、ログインするたびにPostgreSQLを起動します。
- 公式の案内には、ほかにEDBのインストーラーとPostgres.appがあります。

#### Debian、Ubuntu、Raspberry Pi OS

OSのパッケージが17以降なら、そのまま入れます。

```bash
sudo apt install postgresql
```

OSのパッケージが16以前なら、PGDGのリポジトリを足してから、18を入れます。

```bash
sudo apt install -y postgresql-common
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh
sudo apt install postgresql-18
```

- どちらでも、入れるとDBのデータが作られ、PostgreSQLが起動します。
- PGDGはDebian 12と13、Ubuntu 22.04、24.04、26.04に対応します。Ubuntuのarm64はLTSの版だけです。
- KotatsuのLinux向けのアーカイブはamd64とarm64だけなので、Raspberry Piでは64ビット版のRaspberry Pi OSを使います（Raspberry Pi 3以降など、64ビットのCPUを持つ機種）。
- PGDGの案内には、Raspberry Pi OSが載っていません。bookwormを元にした古いRaspberry Pi OSでは、Debian 12向けのPGDGで入る見込みですが、確かめていません（推測）。trixieを元にした今のRaspberry Pi OSなら、OSのパッケージで足ります。

#### RHEL系

RHEL、Rocky Linux、AlmaLinuxのAppStreamのPostgreSQLは16以前なので、PGDGから入れます。
次は9のx86_64の例です。
8と10では`EL-9`の`9`を、arm64では`x86_64`を`aarch64`に変えてください。

```bash
sudo dnf install -y https://download.postgresql.org/pub/repos/yum/reporpms/EL-9-x86_64/pgdg-redhat-repo-latest.noarch.rpm
sudo dnf install -y postgresql18-server
sudo /usr/pgsql-18/bin/postgresql-18-setup initdb
sudo systemctl enable postgresql-18
sudo systemctl start postgresql-18
```

- 8では、`postgresql18-server`を入れる前に、`sudo dnf -qy module disable postgresql`でOSのPostgreSQLのモジュールを切ってください。
- PGDGはOSの新しいほうから2つのマイナーリリース（10.2と10.1など）にだけ対応します。古いマイナーリリースのままだと、リポジトリが見つからずに失敗するので、先にOSを更新してください。

### DBと利用者を作る

Kotatsuが使う利用者とDBを作ります。
DBの所有者はKotatsuの利用者にしてください。
PostgreSQL 15以降は、`public`スキーマに表を作れるのがDBの所有者だけなので、所有者でないと`kotatsu migrate`が失敗します。
Kotatsuの利用者に、スーパーユーザーの権限は要りません。

LinuxではOSの`postgres`ユーザーとして、macOSでは入れたユーザーのまま、次のコマンドを実行します。
1つ目のコマンドがKotatsuの利用者のパスワードを2回尋ねます。

```bash
# Linux
sudo -u postgres createuser --pwprompt kotatsu
sudo -u postgres createdb --owner=kotatsu kotatsu

# macOS
createuser --pwprompt kotatsu
createdb --owner=kotatsu kotatsu
```

WindowsではPowerShellで実行します。
Kotatsuの利用者のパスワードを2回尋ねた後、それぞれのコマンドが`postgres`のパスワードを尋ねます。

```powershell
& "C:\Program Files\PostgreSQL\18\bin\createuser.exe" -U postgres --pwprompt kotatsu
& "C:\Program Files\PostgreSQL\18\bin\createdb.exe" -U postgres --owner=kotatsu kotatsu
```

### Kotatsuをつなぐ

`KOTATSU_DATABASE_URL`に、作った利用者とDBを書きます（[設定](configuration.md#database)）。
パスワードに`@`、`:`、`/`などの記号が入るときは、URLエンコードしてください（`@`は`%40`、`/`は`%2F`）。

Kotatsuを起動するには、ほかにアプリの秘密キー（`KOTATSU_AUTH_SECRET_KEY`）とチューナーサーバーのURL（`KOTATSU_TUNER_URL`）も要ります。
秘密キーは、一度作ったファイルを使い続けてください。
なくすと、TOTPとWeb Pushの設定し直しと、Webhookの作り直しが必要になります。
チューナーサーバーにEDCBを使うときは、`KOTATSU_TUNER_URL`の代わりに[EDCBを使う](#edcbを使う)の設定をします。

```bash
openssl rand -base64 48 > secret_key.txt
chmod 600 secret_key.txt
export KOTATSU_DATABASE_URL='postgres://kotatsu:<パスワード>@127.0.0.1:5432/kotatsu'
export KOTATSU_AUTH_SECRET_KEY_FILE="$PWD/secret_key.txt"
export KOTATSU_TUNER_URL='http://<チューナーサーバー>:40772/'
```

WindowsではPowerShellで次のように設定します。

```powershell
$key = New-Object byte[] 48
[Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($key)
[Convert]::ToBase64String($key) | Set-Content secret_key.txt
$env:KOTATSU_DATABASE_URL = 'postgres://kotatsu:<パスワード>@127.0.0.1:5432/kotatsu'
$env:KOTATSU_AUTH_SECRET_KEY_FILE = "$PWD\secret_key.txt"
$env:KOTATSU_TUNER_URL = 'http://<チューナーサーバー>:40772/'
```

DBのスキーマを作ってから、Kotatsuを起動します。
更新するときも、新しい版で`kotatsu migrate`を実行してから起動し直します（[運用](operations.md)）。

```bash
kotatsu migrate
kotatsu all
```

ほかの設定は[設定](configuration.md)を見てください。
続けて、[最初の管理者を作る](#最初の管理者を作る)に進んでください。

### 小さいマシンで動かす

PostgreSQLの既定の設定は、メモリが少ないマシンには大きめです。
Raspberry Piのようなマシンでは、次のように小さくできます。

| 設定 | 既定 | 小さい設定 |
| --- | --- | --- |
| `shared_buffers` | 128MB | 16MB |
| `max_connections` | 100 | 30 |
| `work_mem` | 4MB | 1MB |
| `maintenance_work_mem` | 64MB | 16MB |
| `wal_buffers` | 4MB（`shared_buffers`から決まる） | 1MB |
| `max_wal_size` | 1GB | 64MB |
| `min_wal_size` | 80MB | 32MB |

PostgreSQL 18のコンテナに、Kotatsuのスキーマを作り、待機しているときのメモリを測りました（#1254）。

| 設定 | 待機しているときのメモリ |
| --- | --- |
| 既定 | 約20〜31MiB |
| 小さい設定 | 約7〜14MiB |

- 手元のamd64のPCで、データの入っていないDBを測った値です。Raspberry Piでは測っていません。
- 値はコンテナのメモリ（cgroup）で、ファイルのキャッシュとカーネルが使う分を含みます。同じ設定でも、測るたびに揺れました。
- データが増えると、キャッシュの分だけ増えます。
- 小さい設定の値は測るために選んだものです。小さいマシンで長く動かして確かめた値ではありません。`work_mem`を小さくすると、大きな並べ替えが遅くなることがあります（推測）。
- `max_connections`の30は`kotatsu all`に足りる見込みです（推測）。Kotatsuは接続の数の上限を、pgxの既定（CPUの数と4の大きいほう）のままにしています。

設定は`ALTER SYSTEM`で変えて、PostgreSQLを起動し直します。
Linux（Debian、Ubuntu、Raspberry Pi OS）では次のように実行します。

```bash
sudo -u postgres psql \
  -c "ALTER SYSTEM SET shared_buffers = '16MB'" \
  -c "ALTER SYSTEM SET max_connections = 30" \
  -c "ALTER SYSTEM SET work_mem = '1MB'" \
  -c "ALTER SYSTEM SET maintenance_work_mem = '16MB'" \
  -c "ALTER SYSTEM SET wal_buffers = '1MB'" \
  -c "ALTER SYSTEM SET max_wal_size = '64MB'" \
  -c "ALTER SYSTEM SET min_wal_size = '32MB'"
sudo systemctl restart postgresql
```

RHEL系では最後の行の`postgresql`を`postgresql-18`にします。
macOSでは、`sudo -u postgres psql`を`psql postgres`に、最後の行を`brew services restart postgresql@18`にします。
Windowsでは、`psql.exe -U postgres`で同じ`ALTER SYSTEM`を流し、管理者のPowerShellで`Get-Service postgresql* | Restart-Service`を実行します。

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
- `EpgDataCap_Bon.ini`の`[SET_TCP]`で、TSを名前付きパイプ（LinuxではFIFO）に送る設定（`IP0=[0.0.0.1]`、`Port0=0`、`Count=1`）を入れます。NetworkTVは、この送り先からTSを中継するので、設定がないと、NetworkTVは開けてもTSを受け取れません（2026年11月に、LinuxのEDCBで確かめました）。
- `EpgTimerSrv.ini`の`[SET]`に`CompatFlags=128`を入れることを勧めます（すでに値があれば、128を足した値にします）。EpgTimerSrvは、この互換動作を有効にしたときだけ、`EpgTimerSrv.ini`をKotatsuに返します。Kotatsuは、そこからEPGの取得の時刻と、取得に要る空きチューナーの数と、予約のマージンの既定を読みます。返さなければ、KotatsuはEPGの取得の時刻を知らないので、ライブ視聴は取得の時刻の前にチューナーを空けておけません。タイムシフトは、BonDriverごとに1本をいつも空けておきます（#1536、#1856）。

次のことに注意してください。

- CtrlCmdのTCPには、パスワードも暗号化もありません。信頼できるネットワークの中でだけつないでください。
- NetworkTVのIDは、EpgTimerSrv全体で1つの名前空間です。Kotatsuが使う範囲（`tuner.edcb.network_tv_id_start`と`_count`。既定は1000から32個）を、ほかのクライアント（KonomiTVは500から、EpgTimerの視聴やTVTestは小さい番号）と重ならないようにしてください。
- KotatsuとEDCBは、チューナーを取り合います。EDCBの予約が始まると、EDCBはKotatsuが使っているNetworkTVのチューナーを取っていくので、Kotatsuの録画やライブ視聴が切れることがあります。Kotatsuはこれに気づいてつなぎ直しを試み、つなぎ直せなければ、録画の失敗の理由を「EDCBの予約にチューナーを取られた」として残します。
- Kotatsuのチューナーの割り当ては、EDCBの予約がチューナーを使う時間帯を、そのチューナーを使わないものとして計算します（#1536）。EDCBの予約と重なってチューナーが足りないKotatsuの予約は、画面に「競合」や「一部のみ」と出ます。EDCBには優先度がなく、Kotatsuの録画からチューナーを譲らせられないので、EDCBでは、「競合」になった予約は録りに行きません。録りに行かなかった予約は、予定の開始が過ぎると、「チューナーを確保できなかった」録画の失敗として残り、録画の失敗を通知します。
- 同じチャンネルでも、別のサービス（NHK総合の1と2など）は、EDCBでは別のチューナーを使います。予約の割り当ても、タイムシフトの割り振りも、そのように数えます。
- EDCBのEPGの取得の時刻の5分前から2分後までは、ライブ視聴とタイムシフトに、EPGの取得に要る空きチューナーを使いません。タイムシフトは、一度開くと取得の時刻をまたいで録り続けるので、その時間に入ると、要る分のタイムシフトを止め、過ぎたら録り直します。取得している最中も、新しいNetworkTVが取得中のチューナーを選ぶときは開きません。NetworkTVを開くと、そのチューナーのEPGの取得が取り消されるからです。このときは、チューナーが足りないときと同じく、ライブ視聴が始まりません。録画は、すべてのチューナーを使います。
  - EpgTimerSrvは、BonDriverごとに、EPGの取得に使うチューナーの数（`EpgTimerSrv.ini`の各BonDriverの節の`EPGCount`）だけ空きチューナーがなければ、その回の取得をしません。Kotatsuはこの数を読んで空けます。既定（0）は、そのBonDriverのすべてのチューナーを使うので、取得の時刻の前後は、そのBonDriverのタイムシフトとライブ視聴が止まります。止める本数を減らしたいときは、`EPGCount=1`にしてください。
- EDCBはKotatsuの予約を知りません。EDCBの予約の割り当てや、EDCBのスリープの判断には、Kotatsuの予約は入りません。EDCBの「録画後にスリープ」や、予約の前に起こす機能は、Kotatsuの録画を考えずにPCを眠らせ、Kotatsuの録画の前にPCを起こしません。KotatsuとEDCBを同じPCで動かすときは、EDCBのスリープ（EpgTimerの設定の録画後の動作と、復帰のための設定）を使わず、Kotatsuの電源の管理（[WindowsとmacOSの電源の設定](#windowsとmacosの電源の設定)）に任せてください。Kotatsuは、録画とライブ視聴の間と次の録画の直前にスリープさせず、次の録画の前に起こす予約を置きます。EDCBの予約は、Kotatsuからは見えないので、この管理には入りません。EDCBの予約も録るなら、OSをスリープしない設定にしてください。
- EDCBのNetworkTVは、ほかの利用者とチューナーを分け合いません。そこで、Kotatsuは、1つのプロセスの中で、同じサービスのストリームを1本のNetworkTVで分け合います（#1535）。`kotatsu all`では、同じ局を録画しながらライブで見ても、チューナーは1本です。ロールを分けた構成では、ロールごとに開くので、同じ局でもチューナーを別に使います。分け合うには、`kotatsu nabe`（`kotatsu hub`）を動かしてください。2台動かすと、2台は同じチューナーのTSを配るので、1台が止まっても、受け手はもう1台から受け取り続けます（#1850）。Helmのチャートでは、`roles.nabe.replicas`を2にします（[Helmで動かす](#helmで動かす)）。nabeがNetworkTVを開き、tenban、mikan、kyusuはインスタンスの登録簿でnabeを見つけて、TSを受け取ります。設定は要りません。ただし、nabeと、nabeから受け取るロールには、同じアプリの秘密キー（`auth.secret_key`）を設定してください。nabeがいないか届かないときは、今までどおりロールごとにNetworkTVを開くので、録画は止まりません。Mirakurun互換のチューナーサーバーでは、nabeは何もしません。
- サービスのロゴは、EDCBが集めたもの（`LogoData`）を使います。EDCBがロゴを集めていなければ、Kotatsuが録画とタイムシフトのTSから集めたロゴを使います。

## WindowsとmacOSの電源の設定

WindowsとmacOSで`kotatsu all`を動かすと、録画、ライブ視聴、エンコードの間と次の録画の直前は、OSをスリープさせません。
次の録画の5分前には、スリープから起こす予約をOSに置きます。
切るときは、録画の既定値の画面（`/admin/recording`）の「OSのスリープ」を切ります。再起動しなくても、次の判定（30秒ごと）から、スリープを止めるのをやめ、起こす予約を消します。
録画の再生（ライブでないもの）の間は、スリープを止めません。タイムシフトを有効にしたサービスがあると、ずっと録るので、ずっとスリープさせないことになります。

- Windowsでは、起こす予約を、タスク スケジューラのタスク（`\Kotatsu\Wake`）として登録します。登録には管理者の権限が要るので、Kotatsuを管理者として動かしてください。管理者でなければ、スリープさせないことだけを行い、起動した後に1回だけ警告を出します。
- Windowsでは、電源オプションの詳細設定の「スリープ」で、「スリープ解除タイマーの許可」を有効にしてください。無効だと、予約を置いても起きません。
- macOSでは、起こす予約にrootが要ります。LaunchDaemonなどで、rootとして動かしてください。rootでなければ、スリープさせないことだけを行います。予約は`pmset -g sched`で確かめられます。
- macOSで止めるのは、操作が無いことで眠るのだけです。蓋を閉じたときと、メニューからスリープしたときは眠ります。
- 休止状態とシャットダウンからは起こせません。外から視聴することも、スリープを解きません。外からいつも観るなら、OSをスリープしない設定にしてください。
- タイムシフトを有効にしたサービスがあると、ずっとスリープしません。

## 字幕の自動生成

録画を終えた後に、録画の音声から字幕を書き起こし、再生画面で「自動生成」の字幕として出せます（#1682）。
放送の字幕のない番組を見るときに使えます。誤りが混ざるので、放送の字幕とは見分けて出します。
書き起こしには[whisper.cpp](https://github.com/ggml-org/whisper.cpp)の`whisper-cli`と、日本語を読めるモデルを使います。
`whisper-cli`は`-ffmpeg`のイメージに入っています。モデルは大きいのでイメージに含まず、管理画面からダウンロードします。
書き起こしはサーバーの中で行い、音声も字幕も外部へは送りません。

1. 録画の既定値の画面（`/admin/recording`）の「字幕の自動生成のモデル」で、書き起こしのモデルを選び、「ダウンロードする」を押します。
   日本語には、最初に選ばれている[kotoba-whisper v2.0](https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml)（約540 MB、Apache-2.0）を勧めます。
   nekonoteがHugging Faceからダウンロードし、大きさとSHA-256を確かめてから、データディレクトリの`whisper/ggml-model.bin`に置きます。
   ダウンロードしている間は、進み具合が出て、取りやめることもできます。
2. （任意）同じ欄で、声のある区間のモデル（Silero VAD、約0.9 MB、MIT）もダウンロードします。
   置かなくても動きますが、台詞の前後の無音や音楽の分だけ、字幕が早く出ることがあります。
3. 同じ画面の「録画が終わったら字幕を自動生成する」を入れます。ルールと予約ごとにも選べます。
   録画の詳細の「字幕を生成する」で、録り終えた録画の字幕を作ることもできます。

CPUだけで動かすと、録画の長さの3割ほど（8スレッド）の時間がかかります。
モデルが見つからないと、ジョブは取りやめ、録画の詳細に「置かれていない」と出します。管理者には、モデルを置く画面へのリンクを出します。

イメージの`whisper-cli`は、amd64ではAVX2とFMAを使います（x86-64-v3）。AVX2のないCPUでは、下の手順で自分でビルドした`whisper-cli`を置き、設定で渡してください。

### 手で置くとき

画面に出ないモデルを使うときや、イメージを使わずに動かすときは、`whisper-cli`とモデルを手で置けます。
手で置いたファイルも、画面には「手で置いたファイル」として出し、消せます。

1. `whisper-cli`をビルドして置きます。静的にリンクすると、Debianのイメージの中でもそのまま動きます。

   ```bash
   git clone --depth 1 --branch v1.9.5 https://github.com/ggml-org/whisper.cpp
   cmake -S whisper.cpp -B whisper.cpp/build -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF -DGGML_OPENMP=OFF -DCMAKE_EXE_LINKER_FLAGS=-static
   cmake --build whisper.cpp/build -j --target whisper-cli whisper-vad-speech-segments
   mkdir -p data/whisper
   cp whisper.cpp/build/bin/whisper-cli whisper.cpp/build/bin/whisper-vad-speech-segments data/whisper/
   ```

   置き場所は、設定で渡します。Docker Composeなら、`environment`に足します（ほかの項目は[設定](configuration.md#captions)）。

   ```yaml
   KOTATSU_CAPTIONS_WHISPER_COMMAND: /data/whisper/whisper-cli
   KOTATSU_CAPTIONS_VAD_COMMAND: /data/whisper/whisper-vad-speech-segments
   ```

2. モデル（ggmlの形式）を、データディレクトリの`whisper/ggml-model.bin`と`whisper/ggml-silero-vad.bin`に置きます。

   ```bash
   curl -fL -o data/whisper/ggml-model.bin https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-kotoba-whisper-v2.0-q5_0.bin
   curl -fL -o data/whisper/ggml-silero-vad.bin https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin
   sudo chown -R 65532:65532 data/whisper
   ```

   ほかの場所に置いたモデルは、設定の`captions.whisper_model`と`captions.vad_model`で渡します。
   設定で渡したモデルは、画面から置いたモデルより優先します。

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

## プールにあるTSを録画として取り込む

ほかの録画ソフトで録ったTS（`.ts`、`.m2ts`）は、保存先プールの中のディレクトリに置けば、`kotatsu import files プールID:ディレクトリ`で録画として取り込めます。
プールをマウントしたホストかコンテナで実行してください。

```sh
kotatsu import files main:old-recordings --dry-run
kotatsu import files main:old-recordings --user admin
```

- 番組名、局、放送時刻は、ファイルの中の番組の情報から読みます。読めなければ、ファイル名を番組名にします。
- Kotatsuにない局のファイルと、Blu-rayの`.m2ts`は取り込まず、理由を表示します。
- 書き込みが終わってから10分たっていないファイルは飛ばします。`--min-age`で変えられます。
- 何度実行しても、同じファイルを二重に取り込みません。
- 取り込んだ録画を消しても、ファイルは消しません。
