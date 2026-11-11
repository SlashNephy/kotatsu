# 破壊的変更

前の版から更新したときに、そのままでは同じように動かなくなる変更を、版ごとにまとめます。
更新する前に、今の版より後の版の節をすべて読み、書いてある対処を済ませてください。
更新の手順は[運用](operations.md#更新)にあります。

## v2026.11.4

v2026.11.3からの変更です。

### 実況のコメント

- 実況は既定では使わなくなりました。外部のサービスと通信するので、管理者とユーザーの両方が有効にしたときだけ使います。
- 更新すると、全体の設定で実況が無効になり、ライブ視聴と録画の再生で実況のコメントが出なくなります。
- 引き続き使うには、管理者が連携の画面（`/admin/integrations`）で、コメントのソースのNX-Jikkyoを有効にしてください。
- プレイヤーの設定を一度も変えていないユーザーは、ユーザーの設定でも無効になります。各ユーザーがプレイヤーの設定の画面（`/settings/player`）か、プレイヤーの実況のボタンで有効にしてください。プレイヤーの設定を変えたことがあるユーザーは、実況を有効にしていれば、その設定を引き継ぎます。

### ライブの実況の中継

- ライブ視聴の実況のコメントは、ブラウザがNX-Jikkyoへ直接つながず、tenbanが中継するようになりました。ブラウザは同じオリジンのWebSocket（`/media/jikkyo/{channel}/ws`）につなぎます。
- リバースプロキシを通すときは、WebSocketの接続を通すようにしてください。通さないと、ライブ視聴で実況のコメントが流れません。nginxでは次のヘッダーを設定します。

  ```nginx
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection $connection_upgrade;
  ```

  `$connection_upgrade`は、`http`のブロックで定義します。

  ```nginx
  map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
  }
  ```

- tenbanからNX-Jikkyo（`nx-jikkyo.tsukumijima.net`）へ通信できるようにしてください。外部への通信を制限している環境では、tenbanからの通信を許可します。

### 録画の再生位置の保持期間

- 録画の再生位置に、保持期間を設けました。最後に更新してから365日を過ぎた再生位置は、nekonoteが消します。
- 更新してnekonoteを起動すると、すぐに1回目を実行します。365日より前に更新したきりの再生位置は、そのときに消えます。
- 消したくないときは、`kotatsu migrate`でスキーマを更新した後、nekonoteを起動する前に、保持期間を0（無期限）にしてください。保持期間はDBの設定です。保持期間の画面（`/admin/retention`）で変えるには、tenbanだけを先に起動します。`all`の構成のように、tenbanとnekonoteを分けて起動できないときは、DBで次のSQLを実行します。

  ```sql
  UPDATE system_settings SET watch_progress_retention_days = 0;
  ```

### 監査ログのIPアドレスとUser-Agentの保持期間

- 監査ログのIPアドレスとUser-Agentに、保持期間（`audit.client_info_retention_days`、`KOTATSU_AUDIT_CLIENT_INFO_RETENTION_DAYS`）を設けました。既定は90日です。過ぎた記録はIPアドレスとUser-Agentだけを空にし、操作の記録は残します。
- 更新してnekonoteを起動すると、すぐに1回目を実行します。90日より前の記録のIPアドレスとUser-Agentは、そのときに空になります。
- もっと長く残すには、新しい版を起動する前に、この設定を大きくしてください。上限はなく、`audit.retention_days`以上にすれば、監査ログを消すまで残ります。古い版は、設定ファイルに知らないキーがあると起動しません。新しい版に切り替えるまでは、設定ファイルではなく環境変数で渡してください。

### `X-Forwarded-Host`と`server.allowed_hosts`

- `X-Forwarded-Host`に値が複数あるとき、すべての値を`server.allowed_hosts`で確かめるようになりました。これまでは、最左の値だけを確かめていました。
- 信頼済みのプロキシがヘッダーを上書きせずに追記する構成（Apacheのmod_proxyの既定など）で、`server.allowed_hosts`を設定しているときは、どの段のプロキシが追記するホスト名も`server.allowed_hosts`に書いてください。書かないと、リクエストを拒みます。

### OTLPで送るトレース

- OTLPで送るトレースから、視聴者のIPアドレスとUser-Agentの属性（`client.address`、`network.peer.address`、`user_agent.original`など）を除くようになりました。
- `url.path`は、先頭の2つのセグメントだけを残し、後ろのセグメントと数字を含むセグメントを`:id`にまとめます。
- これらの属性で絞り込むダッシュボードやアラートは、作り直してください。
