# 運用

Kotatsuを動かし続けるときの、バックアップ、更新、監視の手順です。
起動までの手順は[セットアップ](setup.md)を見てください。

## バックアップ

Kotatsuは、DBとキャプチャの画像を、1つの暗号化したファイルに書き出せます（`kotatsu admin backup`）。
戻すときは、空のDBに`kotatsu admin restore`で流し込みます。
録画のファイルは扱わないので、別の手段でコピーしてください。

| 対象 | 内容 | 手段 |
| --- | --- | --- |
| DB | ユーザー、ルール、予約、録画の一覧と履歴、過去の番組、視聴の記録、設定 | `kotatsu admin backup` |
| キャプチャの画像 | データディレクトリの`captures/` | `kotatsu admin backup` |
| アプリの秘密キー | `auth.secret_key`の値 | `kotatsu admin backup` |
| 録画のファイル | プールの録画 | ファイルのコピー、NASの機能、スナップショット |

`kotatsu admin backup`は、次のものを書き出しません。

- 番組表：戻した後に、チューナーサーバーから取り直します。
- ログインしたセッション：戻した後は、全員がログインし直します。
- 後処理などのジョブ：戻した後に、要るものはまた入ります。
- データディレクトリの生成物（シーク用の索引、サムネイル、シークバー用のプレビュー画像）：`kotatsu admin reanalyze`で作り直せます（次の節）。

### 鍵を作る

バックアップは、[age](https://age-encryption.org/)の公開鍵で暗号化します。
読むには、組になる秘密鍵が要ります。
秘密鍵は、Kotatsuを動かすマシンとは別の場所（手元のPCやパスワードマネージャー）に置いてください。
サーバーには公開鍵だけを置くので、サーバーが漏れても、バックアップは読めません。

ageは、多くのパッケージマネージャーで入れられます（`brew install age`、`apt install age`など）。
`age-keygen`で鍵を作ると、秘密鍵をファイルに書き、公開鍵（`age1`で始まる文字列）を表示します。

```bash
age-keygen -o kotatsu-backup-key.txt
# Public key: age1...
```

秘密鍵をなくすと、バックアップを戻せません。
秘密鍵のファイルは、なくさないよう2か所以上に残してください。

### 書き出す

データディレクトリをマウントしたロールで、公開鍵を渡して実行します。
`--recipient`は、何回でも渡せます。どれか1つの秘密鍵があれば読めます。

```bash
kotatsu admin backup --output /backups/kotatsu-20261117.age --recipient age1...
```

Docker Composeの例では、`kotatsu`のコンテナで実行します。
書き出す先は、コンテナにマウントしたディレクトリ（例では`/data`）にしてください。

```bash
docker compose exec kotatsu kotatsu admin backup --output /data/kotatsu-20261117.age --recipient age1...
```

Kubernetesでは、データディレクトリをマウントしたPod（`deploy/kotatsu-all`、ロールを分けているときは`deploy/kotatsu-nekonote`）で実行します。

```bash
kubectl exec deploy/kotatsu-all -- kotatsu admin backup --output /data/kotatsu-20261117.age --recipient age1...
```

書き出すファイルは、持ち主だけが読める許可（0600）で作ります。
同じ名前のファイルがあれば、上書きせずに止まります。
書き出す間も、録画や視聴は止めずに続けられます。
DBは1つのトランザクションの中で読むので、書き出す間に変わっても、表どうしは同じ時点のものになります。

### 定期に書き出す

nekonoteに、定期にバックアップを書き出させることもできます。

1. nekonoteの設定の`backup.dir`（`KOTATSU_BACKUP_DIR`）に、書き出す先のディレクトリを書き、そのディレクトリをnekonoteにマウントします（[設定](configuration.md#backup)）。
2. 管理者の「設定」→「バックアップ」（`/admin/backup`）で、公開鍵を登録します。
3. 同じ画面で、書き出す間隔（既定は24時間）と、残す世代の数（既定は7）を決めます。

書き出したファイルは`kotatsu-backup-20261117T030000Z.age`のような名前で、残す世代の数を超えると、古いものから消します。
最後の書き出しの結果は、同じ画面に出ます。「今すぐ書き出す」で、間隔を待たずに書き出させることもできます。
書き出せなかったときは、理由を画面に出し、1時間後に書き出し直します。

Docker Composeの例では、`compose.yaml`の`KOTATSU_BACKUP_DIR`と`./backups`の行のコメントを外し、`./backups`を作ってから起動し直します。

```bash
mkdir backups && sudo chown 65532:65532 backups
docker compose up -d
```

Helm Chartでは、`backup.volume`に書き出す先のvolumeを渡します。
allかnekonoteのPodにマウントし、`KOTATSU_BACKUP_DIR`を渡します。

```yaml
backup:
  volume:
    persistentVolumeClaim:
      claimName: kotatsu-backups
```

### 外へ写す

Kotatsuは、書き出す先のディレクトリにだけ書きます。
同じディスクが壊れても戻せるよう、書き出したファイルを、別のマシンやクラウドストレージへ写してください。
ファイルは暗号化してあるので、そのまま置いても中は読めません。

[rclone](https://rclone.org/)なら、たとえば次のコマンドをcronで1日ごとに動かします（`remote:`は、`rclone config`で作った写す先）。

```bash
rclone copy /srv/kotatsu/backups remote:kotatsu-backups --include 'kotatsu-backup-*.age'
```

`rclone copy`は、Kotatsuが古い世代を消しても、写した先のファイルを消しません。
写した先も同じ世代の数にそろえるなら、`rclone sync`を使います。
写す先の古いファイルは、ストレージの機能（保持期間やライフサイクルの設定）で消すこともできます。

Kubernetesでは、同じvolumeをマウントしたCronJobでrcloneを動かせます。
rcloneの設定ファイル（`rclone.conf`）は、Secretにして渡します。
nekonoteとCronJobのPodが同じvolumeをマウントするので、ReadWriteManyのvolume（NFSなど）にするか、ReadWriteOnceなら両方を同じノードで動かしてください。

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: kotatsu-backup-copy
spec:
  schedule: "30 4 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: rclone
              image: rclone/rclone:<版>
              args: ["copy", "/backups", "remote:kotatsu-backups", "--include", "kotatsu-backup-*.age", "--config", "/config/rclone.conf"]
              volumeMounts:
                - name: backups
                  mountPath: /backups
                  readOnly: true
                - name: config
                  mountPath: /config
                  readOnly: true
          volumes:
            - name: backups
              persistentVolumeClaim:
                claimName: kotatsu-backups
            - name: config
              secret:
                secretName: rclone-config
```

NASの同期の機能やスナップショットで写すこともできます。

### 戻す

戻す先は、空のDBにします。表のあるDBには戻しません。
手順は次のとおりです。

1. 全ロールを止めます。
2. 空のDBを作ります。今のDBに戻すなら、DBを消して作り直します。
3. データディレクトリをマウントした環境で、秘密鍵のファイルを渡して`kotatsu admin restore`を実行します。
4. 各ロールを起動します。nekoが番組表を取り直します。全員がログインし直してください。

```bash
kotatsu admin restore --input kotatsu-20261117.age --identity kotatsu-backup-key.txt
```

Docker Composeの例では、次のように戻します。
バックアップと秘密鍵のファイルを、コンテナにマウントした`./data`に置いてから実行し、戻し終えたら秘密鍵のファイルを消します。
`--no-deps`を付けないと、先に`migrate`のサービスがスキーマを作り、DBが空でなくなります。

```bash
docker compose stop kotatsu
docker compose exec postgres dropdb -U kotatsu kotatsu
docker compose exec postgres createdb -U kotatsu kotatsu
docker compose run --rm --no-deps kotatsu admin restore --input /data/kotatsu-20261117.age --identity /data/kotatsu-backup-key.txt
rm data/kotatsu-backup-key.txt
docker compose up -d
```

バックアップのアプリの秘密キーが、設定の`auth.secret_key`と違えば、何も変えずに止まります。
別のマシンに戻すときなど、秘密キーを設定し直すなら、`--secret-key-out`で秘密キーをファイルに書き出させ、その値を`auth.secret_key`に設定してから起動します。

```bash
kotatsu admin restore --input kotatsu-20261117.age --identity kotatsu-backup-key.txt --secret-key-out secret_key.txt
```

`kotatsu admin restore`は、バックアップを書き出したときのスキーマの版までマイグレーションを当ててから流し込み、最後に最新の版まで当てます。
古い版のKotatsuで書き出したバックアップも、新しい版のKotatsuで戻せます。
新しい版で書き出したバックアップは、古い版のKotatsuでは戻せません。

DBを古い時点に戻すと、プールのファイルと食い違います。
戻した後に録ったファイルは「DBにないファイル」として、戻した後に消した録画は「ファイルが見つからない録画」として、保存先の画面で探せます。
DBにないファイルは、`kotatsu import files`で取り込み直せます。

### ほかの手段

CloudNativePGのバックアップや`pg_dump`でDBを残すこともできます。
その場合は、アプリの秘密キーとキャプチャの画像を、別に残してください。
アプリの秘密キーをなくすと、DBを戻しても、暗号化して保存した値を読めません。
その場合は、全員のTOTPとWeb Pushの設定し直しと、Webhookの作り直しが必要です。

### 生成物を作り直す

生成物は、データディレクトリ（`storage.data_dir`）の、録画のセグメントのIDの名前のディレクトリに置きます。
シーク用の索引、サムネイル、シークバー用のプレビュー画像が入っています。
なくしても、録画の詳細の解析し直しと、サムネイルの作り直しで作り直せます。
多くの録画の生成物をなくしたときは、プールとデータディレクトリをマウントしたロールで、`kotatsu admin reanalyze`を実行します。
索引か代表画像のない録画を探し、解析のジョブをまとめて入れます。解析が終われば、画像も作り直します。
新しく録った録画のジョブより後に回すので、録画の処理は遅れません。

```bash
kubectl exec deploy/kotatsu-all -- kotatsu admin reanalyze --dry-run
kubectl exec deploy/kotatsu-all -- kotatsu admin reanalyze
```

ロールを分けているときは、`deploy/kotatsu-nekonote`のように、プールをマウントしたロールで実行してください。

## 更新

更新する前に、[破壊的変更](breaking-changes.md)で、今の版より後の版の節を読み、書いてある対処を済ませてください。

Kotatsuを更新するときは、新しい版のバイナリで`kotatsu migrate`を実行してから、各ロールを起動し直します。
各ロールは起動時にDBのスキーマのバージョンを確かめ、対応していないバージョンなら起動しません。

Docker Composeの例では、`migrate`のサービスが、起動の前にスキーマを更新します。

```bash
docker compose pull
docker compose up -d
```

Helm Chartでは、`helm upgrade`の前に、フックのJobがスキーマを更新します。

```bash
helm repo update
helm upgrade kotatsu kotatsu/kotatsu --version <バージョン> -f values.yaml
```

マイグレーションは、更新の間に新旧のバイナリが同時に動いても壊れない作りです。
そのため、ローリング更新をそのまま使えます。

### 録画を止めずに更新する

録画を書き込むmikanが1つだけのときは、更新の間、録画が途切れます。
新しいプロセスが起動すると、新しいセグメントで録画を再開します。
全ロールを1つのプロセスで動かす`all`の構成も、mikanは1つです。

更新の間も録画を途切れさせたくないときは、mikanを2つ以上動かしてください。
終了のシグナル（SIGTERM）を受けたmikanは、担当している録画を別のmikanに引き継ぎます。
別のmikanがストリームを開いたことを確かめてから、自分のストリームを閉じます。
チューナーサーバーが同じチャンネルを相乗りさせるので、引き継ぎの間もチューナーは増えません。

Helm Chartで録画を止めずに更新するには、`mode: split`にします。
`split`の既定値では、mikanのレプリカは2つです。
mikanには、同時に止まるのを1つまでに抑えるPodDisruptionBudgetを付けています。
`terminationGracePeriodSeconds`は、引き継ぎを待てるよう、既定で300秒にしています。

Docker Composeの例は、mikanが1つの`all`の構成です。

### ロールを分けるときの数

| ロール | 数 |
| --- | --- |
| tenban | 1つ以上。増やすと、APIと配信の負荷を分けられる |
| neko | 1つ以上。複数動かしても、処理するのは1つだけで、残りは待機する |
| mikan | 1つ以上。録画を止めずに更新するには2つ以上 |
| nekonote | 1つ以上。増やすと、後処理を並行して進められる |
| kyusu | 0以上。ライブ視聴と録画の再生でサーバー側のトランスコードを使うときに動かす |
| nabe | 0から2。EDCBを使うときに、同じ局のライブ視聴と録画でチューナーを分け合うために動かす。2つ動かすと、1つが止まっても、もう1つが同じチューナーのTSを配り続ける。Helmのチャートでは`roles.nabe.replicas`を2にする |

ロールを分けるときは、すべてのプロセスに同じDBの接続先と、同じアプリの秘密キーを渡してください。

## 監視

| エンドポイント | 用途 | 成功の条件 |
| --- | --- | --- |
| `/healthz` | liveness | プロセスが動いている |
| `/readyz` | readiness | DBに接続できる |
| `/metrics` | Prometheusのメトリクス | `metrics.listen`のアドレスで公開する |
| `/debug/pprof/` | Goのpprofのプロファイル | `metrics.pprof`を`true`にしたときだけ、`metrics.listen`のアドレスで公開する |

`/healthz`と`/readyz`は、認証なしで使えます。
`/readyz`は、チューナーサーバーの状態を判定に含めません。

ログは、JSON形式で標準出力に書きます。
各プロセスは、最後の8000行をメモリにも持ち、管理者は`/admin/logs`の画面で読めます。
OpenTelemetryのトレースを送るときは、`telemetry.otlp_endpoint`を設定してください。

## 不具合の報告

1. 管理者で`/admin/logs`を開きます。直近のログと、チューナーサーバーのログが並びます。「警告とエラーだけ」と検索で、起きたときの行を確かめられます。
2. 「Issue にする」を押すと、秘密の値、IPアドレス、ホスト名、メールアドレス、ホームフォルダのユーザー名などを伏せた報告が、確認の窓に出ます。どの欄も書き換えたり消したりできます。
3. 「GitHub で開く」を押すと、このブラウザでGitHubの不具合の報告のフォームが開き、版、環境、ログが入ります。起きたことと再現の手順を書いて、送ってください。KotatsuのサーバーからGitHubには何も送りません。

URLの長さには上限があるので、フォームに入るのは、警告とエラーを先に、新しい順に選んだ行だけです。全部のログが要るときは、確認の窓の「コピー」か「ダウンロード」で全部入りの報告を取り、Issueに貼るか添付してください。伏せる規則は形で当てるので、貼る前にもう一度中身を確かめてください。

ログはプロセスごとに持つので、ロールを分けた構成では、画面に出るのは、答えたtenbanのログだけです。ほかのロールのログは、次のように集めて、報告に足してください。伏せていないので、貼る前に秘密の値やホスト名を消してください。

```sh
kubectl logs -l app.kubernetes.io/component=neko --tail=1000
kubectl logs -l app.kubernetes.io/component=mikan --tail=1000
kubectl logs -l app.kubernetes.io/component=nekonote --tail=1000
kubectl logs -l app.kubernetes.io/component=kyusu --tail=1000
```

## アカウントの復旧

ユーザーがパスワードを忘れたときは、`kotatsu user set-password`で設定し直せます。
新しいパスワードは、標準入力から読みます。

```bash
kotatsu user set-password <ユーザー名>
```

`kotatsu admin setup-token`でセットアップトークンを発行できるのは、管理者がまだいないときだけです。
