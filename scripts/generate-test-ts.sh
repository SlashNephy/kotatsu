#!/usr/bin/env bash
# テスト用の TS (testdata/ts/test.ts、test-h264.ts、test-hevc.ts、test-hevc-main10.ts) を作る。Docker が要る。
#
# FFmpeg と TSDuck をコンテナに閉じ込め、手元の環境に依らず同じ手順で作る。
set -euo pipefail

repo_root="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
image="kotatsu-testts"

docker build --quiet --tag "${image}" "${repo_root}/scripts/testts" >/dev/null
mkdir -p "${repo_root}/testdata/ts"
docker run --rm --user "$(id -u):$(id -g)" --volume "${repo_root}/testdata/ts:/out" "${image}" /out/test.ts
docker run --rm --user "$(id -u):$(id -g)" --volume "${repo_root}/testdata/ts:/out" --entrypoint /work/generate-codecs.sh "${image}" /out

ls -l "${repo_root}/testdata/ts/"
