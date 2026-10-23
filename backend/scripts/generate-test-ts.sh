#!/usr/bin/env bash
# テスト用の TS (backend/testdata/ts/test.ts、test-h264.ts、test-hevc.ts、test-hevc-main10.ts) を作る。Docker が要る。
#
# FFmpeg と TSDuck をコンテナに閉じ込め、手元の環境に依らず同じ手順で作る。
set -euo pipefail

backend="$(cd "$(dirname "$0")/.." && pwd)"
image="kotatsu-testts"

docker build --quiet --tag "${image}" "${backend}/scripts/testts" >/dev/null
mkdir -p "${backend}/testdata/ts"
docker run --rm --user "$(id -u):$(id -g)" --volume "${backend}/testdata/ts:/out" "${image}" /out/test.ts
docker run --rm --user "$(id -u):$(id -g)" --volume "${backend}/testdata/ts:/out" --entrypoint /work/generate-codecs.sh "${image}" /out

ls -l "${backend}/testdata/ts/"
