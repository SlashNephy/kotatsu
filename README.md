<p align="center">
  <img src="frontend/public/icons/icon.svg" alt="Kotatsu のアイコン" width="128" height="128">
</p>

# Kotatsu

Kotatsuは日本のテレビ放送を予約、録画、視聴するためのWebアプリケーションです。
[EPGStation](https://github.com/l3tnun/EPGStation)の後継として、[Mirakurun](https://github.com/Chinachu/Mirakurun)、[mirakc](https://github.com/mirakc/mirakc)、Mahironの上で動きます。

[デモ](https://kotatsu-demo.pages.dev/)では、架空の局と番組で画面を触れます。

<table>
  <tr>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/guide-dark.png">
        <img src="docs/screenshots/guide-light.png" alt="番組表">
      </picture>
    </td>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/rule-dark.png">
        <img src="docs/screenshots/rule-light.png" alt="ルールの編集">
      </picture>
    </td>
  </tr>
  <tr>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/recordings-dark.png">
        <img src="docs/screenshots/recordings-light.png" alt="録画の一覧">
      </picture>
    </td>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/recording-dark.png">
        <img src="docs/screenshots/recording-light.png" alt="録画の詳細">
      </picture>
    </td>
  </tr>
</table>

## 特徴

- 番組表、ルール予約、録画、録画の再生、ライブ視聴を、1つのアプリケーションで扱います。
- 録画は.m2tsのまま保存し、ブラウザの中でH.264に変換して再生します。変換できない端末にはサーバー側でトランスコードしたHLSを用意できます。
- 再生中の映像に、ニコニコ実況のコメントとARIB字幕を重ねて表示できます。ライブ視聴では、データ放送も表示できます。
- 1台のマシンからKubernetesのマルチレプリカ構成まで、同じバイナリで動きます。
- ログインは必須で、パスキー、OIDC、パスワードと認証アプリ（TOTP）に対応します。
- 録画の開始や失敗などを、Web PushとWebhook（Discord、Slack、汎用のJSON）で知らせます。
- EPGStationから、ルール、予約、録画を取り込めます。

## 使い方

PostgreSQLと、Mirakurun、mirakc、Mahironのいずれかのチューナーサーバーが必要です。
コンテナイメージは`ghcr.io/slashnephy/kotatsu`で配布しています。

- [セットアップ](docs/setup.md)：Docker ComposeとHelmでの起動、EPGStationからの移行
- [設定](docs/configuration.md)：設定項目の一覧
- [運用](docs/operations.md)：バックアップ、更新、監視

## 開発

手元で動かす手順とよく使うコマンドは、[開発](docs/development.md)にまとめました。
変更を送るときの約束は[貢献のしかた](.github/CONTRIBUTING.md)にあります。
設計は[設計書](docs/spec.v1.md)にまとめています。

## ライセンス

Kotatsuは[Apache License 2.0](LICENSE)で公開しています。
同梱するソフトウェアとデータのライセンスは、[設計書の付録B](docs/spec.v1.md#付録-b-ライセンスの境界)にまとめました。
