#!/usr/bin/env bash
# PR で変わったファイルから、CI のどのジョブを走らせるかを決める (#700、#1939)。
# 関係のないジョブを飛ばし、同時に走る CI のジョブを減らして、ランナーの空きを待つ時間を短くする。
#
# 使い方: git diff --name-only <base> <head> | ci-changes.sh
#   変わったファイルを標準入力から 1 行ずつ読み、ジョブごとに走らせるかどうかを name=true|false で出す。
#   codeql は、解析する言語の matrix (JSON の配列) を出す。
#   迷うファイルは走らせる側に倒す。どのジョブにも関係しないと分かっているものだけを、ジョブごとに除く。
#   引数に --all を渡すと、標準入力を読まずに、すべてを走らせる (main への push と手で動かしたとき)。
set -euo pipefail

go=false generate=false web=false deploy=false mpeg2toh264=false ffmpeg=false
image=false pnpm_audit=false kube_linter=false
codeql_actions=false codeql_go=false codeql_js=false codeql_python=false codeql_cpp=false

all() {
  go=true generate=true web=true deploy=true mpeg2toh264=true ffmpeg=true
  image=true pnpm_audit=true kube_linter=true
  codeql_actions=true codeql_go=true codeql_js=true codeql_python=true codeql_cpp=true
}

if [[ "${1:-}" == --all ]]; then
  all
else
  while IFS= read -r file || [[ -n "$file" ]]; do
    [[ -z "$file" ]] && continue

    case "$file" in
      # ツールのバージョンと CI 自体の変更は、すべてのジョブに関わる。
      mise.toml | mise.lock | .github/workflows/ci.yml | .github/workflows/image.yml | .github/actions/* | scripts/ci-changes.sh)
        all
        continue
        ;;
      # Trivy と kube-linter のバージョンは、イメージとマニフェストの検査に関わる。
      mise.scan.toml | mise.scan.lock)
        image=true kube_linter=true
        continue
        ;;
      # 文書と、開発の道具の設定は、どのジョブにも関わらない。
      # 定期の検査 (.github/workflows/scan.yml) だけが使うものも、CI のジョブには関わらない。
      docs/* | *.md | LICENSE | renovate.json5 | .claude/* | .specstory/* | .github/scan/*)
        continue
        ;;
    esac

    # Go のテストと lint。画面 (frontend/) は関わらない。
    case "$file" in
      frontend/* | deploy/* | .github/*) ;;
      *) go=true ;;
    esac

    # 生成物の確認。Go の生成に加え、zxcvbn のバンドルを画面の依存で作るので、その依存にも関わる。
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

    # Helm Chart から生成したマニフェストと、その設定の検査 (kube-linter)。
    case "$file" in
      deploy/*) deploy=true kube_linter=true ;;
    esac

    # mpeg2toh264 のプレイヤーのビルド。
    case "$file" in
      frontend/vendor/mpeg2toh264/* | frontend/public/mpeg2toh264/* | frontend/scripts/build-mpeg2toh264.sh | frontend/mise.mpeg2toh264.toml | frontend/mise.mpeg2toh264.lock) mpeg2toh264=true ;;
    esac

    # ブラウザ内でデコードする経路の WASM の FFmpeg のビルド。
    case "$file" in
      frontend/vendor/ffmpeg/* | frontend/public/ffmpeg/* | frontend/scripts/build-ffmpeg.sh) ffmpeg=true ;;
    esac

    # 画面の依存の脆弱性 (pnpm audit)。依存を変えたときだけ見る。新しく公表された勧告は、定期の検査が見つける。
    case "$file" in
      frontend/package.json | frontend/pnpm-lock.yaml | frontend/pnpm-workspace.yaml) pnpm_audit=true ;;
    esac

    # CodeQL で解析する言語。
    case "$file" in
      .github/*) codeql_actions=true ;;
      backend/* | go.work | go.work.sum) codeql_go=true ;;
    esac
    case "$file" in
      *.py) codeql_python=true ;;
      *.c | *.h | *.cc | *.cpp) codeql_cpp=true ;;
      frontend/*) codeql_js=true ;;
    esac
  done
fi

# イメージは、Go のバイナリと画面を埋め込むので、どちらかが変わったら作る。
if [[ "$go" == true || "$web" == true ]]; then
  image=true
fi

# 言語ごとの matrix。build-mode は、CodeQL の default setup で動かしていたときと揃える (設計書 11.1)。
codeql=()
[[ "$codeql_actions" == true ]] && codeql+=('{"language":"actions","build-mode":"none"}')
[[ "$codeql_cpp" == true ]] && codeql+=('{"language":"c-cpp","build-mode":"none"}')
[[ "$codeql_go" == true ]] && codeql+=('{"language":"go","build-mode":"manual"}')
[[ "$codeql_js" == true ]] && codeql+=('{"language":"javascript-typescript","build-mode":"none"}')
[[ "$codeql_python" == true ]] && codeql+=('{"language":"python","build-mode":"none"}')

echo "go=$go"
echo "generate=$generate"
echo "web=$web"
echo "deploy=$deploy"
echo "mpeg2toh264=$mpeg2toh264"
echo "ffmpeg=$ffmpeg"
echo "image=$image"
echo "pnpm-audit=$pnpm_audit"
echo "kube-linter=$kube_linter"
echo "codeql=[$(IFS=,; echo "${codeql[*]}")]"
