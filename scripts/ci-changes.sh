#!/usr/bin/env bash
# PR で変わったファイルから、CI のどのジョブを走らせるかを決める (#700)。
# 関係のないジョブを飛ばし、同時に走る CI のジョブを減らして、ランナーの空きを待つ時間を短くする。
#
# 使い方: git diff --name-only <base> <head> | ci-changes.sh
#   変わったファイルを標準入力から 1 行ずつ読み、ジョブごとに走らせるかどうかを name=true|false で出す。
#   迷うファイルは走らせる側に倒す。どのジョブにも関係しないと分かっているものだけを、ジョブごとに除く。
set -euo pipefail

go=false generate=false web=false deploy=false mpeg2toh264=false
while IFS= read -r file || [[ -n "$file" ]]; do
  [[ -z "$file" ]] && continue

  case "$file" in
    # ツールの版と CI 自体の変更は、すべてのジョブに関わる。
    mise.toml | mise.lock | .github/workflows/ci.yml | scripts/ci-changes.sh)
      go=true generate=true web=true deploy=true mpeg2toh264=true
      continue
      ;;
    # 文書と、開発の道具の設定は、どのジョブにも関わらない。
    docs/* | *.md | LICENSE | renovate.json5 | .claude/* | .specstory/*)
      continue
      ;;
  esac

  # Go のテストと lint。画面 (frontend/) は関わらない。
  case "$file" in
    frontend/* | deploy/* | .github/*) ;;
    *) go=true ;;
  esac

  # 生成物の確かめ。Go の生成に加え、zxcvbn のバンドルを画面の依存で作るので、その依存にも関わる。
  case "$file" in
    frontend/package.json | frontend/pnpm-lock.yaml | frontend/pnpm-workspace.yaml | frontend/patches/* | frontend/zxcvbn/* | frontend/schema.graphql | frontend/public/icons/* | frontend/mise.toml) generate=true ;;
    frontend/* | deploy/* | .github/*) ;;
    *) generate=true ;;
  esac

  # 画面。Go の変更は、GraphQL のスキーマが変われば生成した frontend/schema.graphql も変わるので、それで拾う。
  case "$file" in
    frontend/*) web=true ;;
    backend/* | go.work | go.work.sum | deploy/* | .github/* | \
      scripts/next-version.sh | mise.dev.toml | mise.dev.lock | compose.yaml | docker/* | \
      Dockerfile | .dockerignore | .goreleaser.yaml) ;;
    *) web=true ;;
  esac

  # Helm Chart から生成したマニフェスト。
  case "$file" in
    deploy/*) deploy=true ;;
  esac

  # mpeg2toh264 のプレイヤーのビルド。
  case "$file" in
    frontend/vendor/mpeg2toh264/* | frontend/public/mpeg2toh264/* | frontend/scripts/build-mpeg2toh264.sh | frontend/mise.mpeg2toh264.toml | frontend/mise.mpeg2toh264.lock) mpeg2toh264=true ;;
  esac
done

echo "go=$go"
echo "generate=$generate"
echo "web=$web"
echo "deploy=$deploy"
echo "mpeg2toh264=$mpeg2toh264"
