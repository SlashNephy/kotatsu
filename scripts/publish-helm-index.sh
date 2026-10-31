#!/usr/bin/env bash
# Helm リポジトリの index.yaml に、リリースに添付した Chart を足し、gh-pages ブランチに書く (設計書 11.1)。
# コミットは git push ではなく GitHub の API で作る。GitHub App のトークンで動かすと、App の名前のコミットになり、
# GitHub が署名する (#1060)。
#
# 使い方: publish-helm-index.sh <Chart を置いたディレクトリ> <タグ>
#   環境変数 GH_TOKEN (contents: write のトークン) と REPOSITORY (owner/name) を渡す。
#   BRANCH で書くブランチを変えられる (既定は gh-pages)。
set -euo pipefail

chart_dir="$1"
ref_name="$2"
branch="${BRANCH:-gh-pages}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# get は、API の GET の結果を出す。見つからなければ何も出さずに成功し、ほかの失敗はそのまま落とす。
get() {
  local out
  if out="$(gh api "$@" 2>"$work/err")"; then
    printf '%s' "$out"
  elif ! grep -q 'HTTP 404' "$work/err"; then
    cat "$work/err" >&2
    return 1
  fi
}

# 最初のリリースでは gh-pages ブランチがないので、親のないコミットから始める。
parent="$(get "repos/$REPOSITORY/git/ref/heads/$branch" --jq .object.sha)"
merge=()
if [[ -n "$parent" ]]; then
  get -H 'Accept: application/vnd.github.raw' "repos/$REPOSITORY/contents/index.yaml?ref=$branch" >"$work/index.yaml"
  if [[ -s "$work/index.yaml" ]]; then
    merge=(--merge "$work/index.yaml")
  fi
fi
# 生成した index.yaml は apiVersion: で始まるので、Jekyll はそのまま配る。
# 先頭に --- を付けると front matter として読まれ、配るファイルから消えるので付けない。
helm repo index "$chart_dir" --url "https://github.com/${REPOSITORY}/releases/download/${ref_name}" "${merge[@]}"

blob="$(base64 -w0 "$chart_dir/index.yaml" | jq -Rn '{content: input, encoding: "base64"}' |
  gh api "repos/$REPOSITORY/git/blobs" --input - --jq .sha)"
if [[ -n "$parent" ]]; then
  base_tree="$(gh api "repos/$REPOSITORY/git/commits/$parent" --jq .tree.sha)"
  tree="$(jq -n --arg base "$base_tree" --arg blob "$blob" \
    '{base_tree: $base, tree: [{path: "index.yaml", mode: "100644", type: "blob", sha: $blob}]}' |
    gh api "repos/$REPOSITORY/git/trees" --input - --jq .sha)"
  parents="$(jq -n --arg parent "$parent" '[$parent]')"
else
  tree="$(jq -n --arg blob "$blob" '{tree: [{path: "index.yaml", mode: "100644", type: "blob", sha: $blob}]}' |
    gh api "repos/$REPOSITORY/git/trees" --input - --jq .sha)"
  parents='[]'
fi
# 作者を渡さないと、トークンの持ち主 (App) の名前になり、GitHub が署名する。
commit="$(jq -n --arg message "chore: publish the kotatsu chart ${ref_name#v}" --arg tree "$tree" --argjson parents "$parents" \
  '{message: $message, tree: $tree, parents: $parents}' |
  gh api "repos/$REPOSITORY/git/commits" --input - --jq .sha)"
if [[ -n "$parent" ]]; then
  # 早送りでなければ (ほかのリリースが先に書いたら) 落とす。
  gh api -X PATCH "repos/$REPOSITORY/git/refs/heads/$branch" -f sha="$commit" -F force=false --silent
else
  gh api "repos/$REPOSITORY/git/refs" -f ref="refs/heads/$branch" -f sha="$commit" --silent
fi
echo "published $commit to $branch"
