# syntax=docker/dockerfile:1
# Kotatsu のコンテナイメージ (設計書 2.4、11.1、付録 B)。
#
#   - static: distroless の static イメージ。tenban、neko、mikan 向け。
#   - ffmpeg: Debian 13 の slim に FFmpeg と tsreadex と whisper.cpp を入れたイメージ。kyusu、nekonote、all 向け。
#
# docker buildx build --platform linux/amd64,linux/arm64 --target static . のようにビルドする。
#
# ソースからビルドするものは、ソースを取る段 (source-*) と、ビルドする段 (build-*) に分ける。C と C++ のビルドは QEMU では
# 遅いので、ビルドするマシンのアーキテクチャ ($BUILDPLATFORM) で、対象のアーキテクチャへクロスコンパイルする。

# renovate: datasource=github-tags depName=xtne6f/tsreadex versioning=loose
ARG TSREADEX_VERSION=master-260428
ARG TSREADEX_REPOSITORY=https://github.com/xtne6f/tsreadex.git

# renovate: datasource=github-releases depName=ggml-org/whisper.cpp
ARG WHISPER_CPP_VERSION=v1.9.5
ARG WHISPER_CPP_REPOSITORY=https://github.com/ggml-org/whisper.cpp.git

# FFmpeg は、リリースの tarball からビルドする (#2001)。tarball は、リリースの署名鍵 (https://ffmpeg.org/download.html#releases) の
# 署名を確かめてから使う。
# renovate: datasource=github-tags depName=FFmpeg/FFmpeg versioning=loose extractVersion=^n(?<version>\d+\.\d+(?:\.\d+)?)$
ARG FFMPEG_VERSION=9.1
ARG FFMPEG_SIGNING_FINGERPRINT=FCF986EA15E6E293A5644F10B4322F04D67658D8
ARG FFMPEG_DEPENDENCIES="libx264-dev libx265-dev libnuma-dev libva-dev libdrm-dev"
ARG FFMPEG_DEPENDENCIES_AMD64="libvpl-dev"

# NVENC と NVDEC を、実行時に読み込むためのヘッダー。版で、動く NVIDIA のドライバの下限が決まる (12.1 は Linux で 530.41.03 以降)。
# 下限を上げないよう、Renovate では 12.1 の中でだけ上げる (renovate.json5)。
# renovate: datasource=github-tags depName=FFmpeg/nv-codec-headers versioning=loose extractVersion=^n(?<version>\d+\.\d+\.\d+\.\d+)$
ARG NV_CODEC_HEADERS_VERSION=12.1.14.1
ARG NV_CODEC_HEADERS_REPOSITORY=https://github.com/FFmpeg/nv-codec-headers.git

# ffmpeg のイメージに入れる Debian のパッケージ。FFmpeg がリンクする x264、x265、libva、libdrm と、VAAPI と QSV のドライバ。
# VAAPI と QSV のドライバのうち、Intel のものは amd64 にしかない。Intel の VAAPI のドライバだけは、non-free の
# intel-media-va-driver-non-free を入れる (#509)。main の intel-media-va-driver は、Gen11 以前の GPU で、縮小 (VPP) と
# HEVC のエンコードができないため。ライセンスは Expat で再配布できるが、GPU のバイナリにソースがないので non-free にある (付録 B)。
ARG RUNTIME_PACKAGES="ca-certificates libx264-164 libx265-215 libva2 libva-drm2 libdrm2 mesa-va-drivers"
ARG RUNTIME_PACKAGES_AMD64="intel-media-va-driver-non-free libvpl2 libmfx-gen1.2"

# Debian 13 の slim。対象のアーキテクチャで動かす段 (debian) と、ビルドするマシンで動かす段 (debian-build) の土台にする。
# FROM --platform は、段の名前に付けても効かないので、イメージを 2 回書く。
ARG DEBIAN_IMAGE=docker.io/library/debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

FROM ${DEBIAN_IMAGE} AS debian
SHELL ["/usr/bin/bash", "-euxo", "pipefail", "-c"]

FROM --platform=$BUILDPLATFORM ${DEBIAN_IMAGE} AS debian-build
SHELL ["/usr/bin/bash", "-euxo", "pipefail", "-c"]

# クロスコンパイルに使う段。対象のアーキテクチャの GNU のツールチェーンを入れ、その三つ組を /triple に書いておく。
FROM debian-build AS cross
ARG TARGETARCH
RUN <<EOF
  case "$TARGETARCH" in
    amd64) echo x86_64-linux-gnu > /triple ;;
    arm64) echo aarch64-linux-gnu > /triple ;;
    *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;;
  esac
  apt-get update
  apt-get install -y --no-install-recommends binutils ca-certificates cmake git make "g++-$(tr _ - < /triple)"
  rm -rf /var/lib/apt/lists/*
EOF
WORKDIR /build

# SPA は、どのアーキテクチャでも同じものになるので、ビルドするマシンのアーキテクチャで 1 回だけビルドする。
FROM --platform=$BUILDPLATFORM docker.io/library/node:26.13.1-trixie-slim@sha256:e6bedef217677b276c591795e169840edd9096dbf13f29df8ef8d1856b8820bc AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
# pnpm の版は package.json の packageManager に従う。Node.js 25 から corepack が同梱されないので、npm で入れる。
RUN npm install -g "$(node -p 'require("./package.json").packageManager')"
# ワークスペースのパッケージ (ビルドした mpeg2toh264) も、依存を入れる前に置く。
COPY frontend/vendor/ ./vendor/
# 依存に当てるパッチ (pnpm-workspace.yaml の patchedDependencies) も、依存を入れる前に置く。
COPY frontend/patches/ ./patches/
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY frontend/ ./
# 字幕を描くフォントは npm にないので、frontend/vendor/caption-font/SOURCE で固定したリリースから取得する。
RUN node scripts/fetch-caption-font.mjs
# 字幕のフォントを、基本の字と JIS X 0213 で足された漢字の 2 つに分け、WOFF2 にする (#1520)。
RUN node scripts/split-caption-font.mjs
# 字幕の外字の既定の表は、frontend/vendor/libaribcaption/SOURCE で固定した commit の定義から作る。
RUN node scripts/fetch-libaribcaption-drcs.mjs
RUN pnpm exec vp build

# Go のバイナリは、ビルドするマシンのアーキテクチャのまま、対象のアーキテクチャへクロスコンパイルする。
# Go の脆弱性の影響を受ける版かどうかがわかるよう、タグはパッチバージョンまで書く (mise.toml の go と合わせる)。
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d AS build
WORKDIR /src
# go.work に足したモジュールの go.mod と go.sum も、ここで写す。
COPY go.work go.work.sum ./
COPY backend/go.mod backend/go.sum ./backend/
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/backend/internal/web/webui/dist/app ./backend/internal/web/webui/dist/app
ARG TARGETOS
ARG TARGETARCH
# VERSION は、版とコミットを 1.2.3+<コミット> の形で渡す (internal/lib/buildinfo)。.git はイメージに入れないので、
# 渡さなければ dev になる。
ARG VERSION=""
# 本体に組み込む Go のモジュールのライセンスの一覧を、ビルドの前に書き出して埋め込む (#1604)。
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go -C backend run ./internal/web/licenses/printlicenses -o internal/web/licenses/dist/go.json
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X github.com/SlashNephy/kotatsu/internal/lib/buildinfo.version=${VERSION}" -o /out/kotatsu ./backend/cmd/kotatsu

# tsreadex は、配布されているバイナリがないので、ソースからビルドする。
FROM debian-build AS source-tsreadex
RUN <<EOF
  apt-get update
  apt-get install -y --no-install-recommends ca-certificates git
  rm -rf /var/lib/apt/lists/*
EOF
ARG TSREADEX_REPOSITORY
ARG TSREADEX_VERSION
RUN git clone --depth 1 --branch "$TSREADEX_VERSION" "$TSREADEX_REPOSITORY" /src

FROM cross AS build-tsreadex
COPY --from=source-tsreadex /src/ ./
ARG TSREADEX_REPOSITORY
# ライセンス文書と、ビルドに使ったリポジトリとコミットを残す (付録 B)。
RUN <<EOF
  triple="$(cat /triple)"
  make CXX="$triple-g++"
  install -D -m 755 tsreadex /out/bin/tsreadex
  install -D -m 644 License.txt /out/doc/tsreadex.License.txt
  echo "tsreadex ${TSREADEX_REPOSITORY%.git} $(git rev-parse HEAD)" > /out/doc/SOURCES
EOF

# 字幕の自動生成 (設計書 6.1、#1682) に使う whisper.cpp の whisper-cli と whisper-vad-speech-segments は、配布されている Linux の
# バイナリがないので、ソースからビルドする。共有ライブラリを持ち込まないよう、静的にリンクする。
FROM debian-build AS source-whisper-cpp
RUN <<EOF
  apt-get update
  apt-get install -y --no-install-recommends ca-certificates git
  rm -rf /var/lib/apt/lists/*
EOF
ARG WHISPER_CPP_REPOSITORY
ARG WHISPER_CPP_VERSION
RUN git clone --depth 1 --branch "$WHISPER_CPP_VERSION" "$WHISPER_CPP_REPOSITORY" /src

# 命令セットは、ビルドするマシンに合わせず (GGML_NATIVE=OFF)、amd64 は AVX2 と FMA (x86-64-v3)、arm64 は ARMv8.0 を前提にする。
FROM cross AS build-whisper-cpp
COPY --from=source-whisper-cpp /src/ ./
ARG TARGETARCH
ARG WHISPER_CPP_REPOSITORY
RUN <<EOF
  triple="$(cat /triple)"
  case "$TARGETARCH" in
    amd64) arch=(-DCMAKE_SYSTEM_PROCESSOR=x86_64 -DGGML_AVX=ON -DGGML_AVX2=ON -DGGML_FMA=ON -DGGML_F16C=ON -DGGML_BMI2=ON) ;;
    arm64) arch=(-DCMAKE_SYSTEM_PROCESSOR=aarch64) ;;
  esac
  cmake -S . -B build -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_SYSTEM_NAME=Linux -DCMAKE_C_COMPILER="$triple-gcc" -DCMAKE_CXX_COMPILER="$triple-g++" "${arch[@]}" \
    -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF -DGGML_OPENMP=OFF -DCMAKE_EXE_LINKER_FLAGS=-static
  cmake --build build -j "$(nproc)" --target whisper-cli whisper-vad-speech-segments
  for name in whisper-cli whisper-vad-speech-segments; do
    install -D -m 755 "build/bin/$name" "/out/bin/$name"
    "$triple-strip" "/out/bin/$name"
  done
  # ライセンス文書と、ビルドに使ったリポジトリとコミットを残す (付録 B)。
  install -D -m 644 LICENSE /out/doc/whisper.cpp.License.txt
  echo "whisper.cpp ${WHISPER_CPP_REPOSITORY%.git} $(git rev-parse HEAD)" > /out/doc/SOURCES
EOF

FROM debian-build AS source-nv-codec-headers
RUN <<EOF
  apt-get update
  apt-get install -y --no-install-recommends ca-certificates git
  rm -rf /var/lib/apt/lists/*
EOF
ARG NV_CODEC_HEADERS_REPOSITORY
ARG NV_CODEC_HEADERS_VERSION
RUN git clone --depth 1 --branch "n$NV_CODEC_HEADERS_VERSION" "$NV_CODEC_HEADERS_REPOSITORY" /src

FROM debian-build AS source-ffmpeg
RUN <<EOF
  apt-get update
  apt-get install -y --no-install-recommends ca-certificates curl gpg gpg-agent xz-utils
  rm -rf /var/lib/apt/lists/*
EOF
ARG FFMPEG_VERSION
ARG FFMPEG_SIGNING_FINGERPRINT
WORKDIR /download
RUN <<EOF
  tarball="ffmpeg-$FFMPEG_VERSION.tar.xz"
  curl -fsSL -o "$tarball" "https://ffmpeg.org/releases/$tarball"
  curl -fsSL -o "$tarball.asc" "https://ffmpeg.org/releases/$tarball.asc"
  export GNUPGHOME="$(mktemp -d)"
  curl -fsSL https://ffmpeg.org/ffmpeg-devel.asc | gpg --quiet --import
  if ! gpg --status-fd 1 --verify "$tarball.asc" "$tarball" 2>/dev/null | grep -q "^\[GNUPG:\] VALIDSIG $FFMPEG_SIGNING_FINGERPRINT "; then
    echo "the signature of $tarball is not by $FFMPEG_SIGNING_FINGERPRINT" >&2
    exit 1
  fi
  mkdir /src
  tar -C /src --strip-components 1 -xf "$tarball"
  echo "FFmpeg https://ffmpeg.org/releases/$tarball sha256:$(sha256sum "$tarball" | cut -d ' ' -f 1)" > /src/SOURCES
EOF

# FFmpeg は、使う部品 (docker/ffmpeg/components) だけを組み込む。--enable-gpl で構成し、--enable-nonfree は付けない (付録 B)。
# libav* は静的に、x264 などの Debian のライブラリは動的にリンクする。対象のアーキテクチャのライブラリは、Debian の multiarch で入れる。
FROM cross AS build-ffmpeg
ARG TARGETARCH
ARG FFMPEG_DEPENDENCIES
ARG FFMPEG_DEPENDENCIES_AMD64
RUN <<EOF
  dependencies="$FFMPEG_DEPENDENCIES"
  if [ "$TARGETARCH" = amd64 ]; then
    dependencies="$dependencies $FFMPEG_DEPENDENCIES_AMD64"
  fi
  dpkg --add-architecture "$TARGETARCH"
  apt-get update
  # configure は、ビルドするマシン向けのコンパイラ (gcc と libc6-dev) でも、小さなプログラムを作る。
  apt-get install -y --no-install-recommends gcc libc6-dev nasm pkgconf $(printf "%s:$TARGETARCH " $dependencies)
  rm -rf /var/lib/apt/lists/*
EOF
COPY --from=source-nv-codec-headers /src/ /build/nv-codec-headers/
RUN make -C nv-codec-headers install PREFIX=/opt/ffnvcodec
COPY --from=source-ffmpeg /src/ /build/ffmpeg/
COPY docker/ffmpeg/components /build/components
ARG NV_CODEC_HEADERS_REPOSITORY
WORKDIR /build/ffmpeg
RUN <<EOF
  triple="$(cat /triple)"
  enable=()
  while read -r kind name only; do
    case "$kind" in '' | '#'*) continue ;; esac
    if [ -z "${only:-}" ] || [ "$only" = "$TARGETARCH" ]; then
      enable+=("--enable-$kind=$name")
    fi
  done < /build/components
  libraries=(--enable-libx264 --enable-libx265 --enable-vaapi --enable-libdrm --enable-ffnvcodec --enable-cuda --enable-nvdec --enable-nvenc)
  if [ "$TARGETARCH" = amd64 ]; then
    libraries+=(--enable-libvpl)
  fi
  export PKG_CONFIG_LIBDIR="/usr/lib/$triple/pkgconfig:/usr/share/pkgconfig:/opt/ffnvcodec/lib/pkgconfig"
  ./configure \
    --prefix=/usr/local \
    --enable-cross-compile --target-os=linux --arch="${triple%%-*}" --cross-prefix="$triple-" --pkg-config=pkg-config \
    --enable-gpl \
    --disable-autodetect --disable-everything --disable-doc --disable-ffplay --disable-debug \
    "${libraries[@]}" "${enable[@]}" \
    || { tail -n 50 ffbuild/config.log; exit 1; }
  make -j "$(nproc)"
  make install
  # ライセンス文書と、ソースの入手先を残す (付録 B)。
  install -D -m 644 COPYING.GPLv2 /out/doc/FFmpeg.COPYING.GPLv2.txt
  install -D -m 644 LICENSE.md /out/doc/FFmpeg.LICENSE.md
  cp SOURCES /out/doc/SOURCES
  # nv-codec-headers は、ヘッダーの頭にライセンスを書いている。
  for header in ../nv-codec-headers/include/ffnvcodec/*.h; do
    echo "== $(basename "$header")"
    sed -n '1,/\*\//p' "$header"
  done > /out/doc/nv-codec-headers.License.txt
  echo "nv-codec-headers ${NV_CODEC_HEADERS_REPOSITORY%.git} $(git -C ../nv-codec-headers rev-parse HEAD)" >> /out/doc/SOURCES
EOF

FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5 AS static
COPY --from=build /out/kotatsu /usr/local/bin/kotatsu
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kotatsu"]

# FFmpeg と tsreadex と whisper.cpp を入れた実行環境。ffmpeg と、開発用の dev の土台にする。
FROM debian AS runtime
ARG TARGETARCH
ARG RUNTIME_PACKAGES
ARG RUNTIME_PACKAGES_AMD64
# ベースイメージは、Debian のセキュリティの更新 (trixie-security) をすぐには取り込まないので、入っているパッケージも上げる。
RUN <<EOF
  packages="$RUNTIME_PACKAGES"
  if [ "$TARGETARCH" = amd64 ]; then
    sed -i 's/^Components: main$/Components: main non-free/' /etc/apt/sources.list.d/debian.sources
    packages="$packages $RUNTIME_PACKAGES_AMD64"
  fi
  apt-get update
  apt-get upgrade -y
  apt-get install -y --no-install-recommends $packages
  rm -rf /var/lib/apt/lists/*
EOF
COPY --from=build-ffmpeg /usr/local/bin/ffmpeg /usr/local/bin/ffprobe /usr/local/bin/
# 使う部品が、すべて組み込めたかを確かめる (#2001)。
RUN --mount=source=docker/ffmpeg,target=/tmp/ffmpeg /tmp/ffmpeg/check-components.sh /tmp/ffmpeg/components "$TARGETARCH"
COPY --from=build-tsreadex /out/bin/ /usr/local/bin/
# whisper.cpp のモデルは大きく、使う人も選ぶので、イメージに入れない。管理者が画面からダウンロードする (#1682)。
COPY --from=build-whisper-cpp /out/bin/ /usr/local/bin/
COPY --from=build-ffmpeg /out/doc/FFmpeg.COPYING.GPLv2.txt /out/doc/FFmpeg.LICENSE.md /out/doc/nv-codec-headers.License.txt /usr/share/doc/kotatsu/third-party/
COPY --from=build-tsreadex /out/doc/tsreadex.License.txt /usr/share/doc/kotatsu/third-party/
COPY --from=build-whisper-cpp /out/doc/whisper.cpp.License.txt /usr/share/doc/kotatsu/third-party/
# ビルドに使ったリポジトリとコミットの一覧 (付録 B) は、それぞれの段で書いたものをつなぐ。
RUN --mount=from=build-ffmpeg,source=/out/doc/SOURCES,target=/tmp/sources/ffmpeg \
    --mount=from=build-tsreadex,source=/out/doc/SOURCES,target=/tmp/sources/tsreadex \
    --mount=from=build-whisper-cpp,source=/out/doc/SOURCES,target=/tmp/sources/whisper.cpp \
    cat /tmp/sources/ffmpeg /tmp/sources/tsreadex /tmp/sources/whisper.cpp > /usr/share/doc/kotatsu/third-party/SOURCES
# 入れた Debian のパッケージとソースパッケージの一覧を、ソースの入手先とともに書き出す (付録 B)。
RUN --mount=source=docker/debian/write-debian-sources.sh,target=/tmp/write-debian-sources.sh \
    sh /tmp/write-debian-sources.sh /usr/share/doc/kotatsu/third-party/debian-packages.tsv

# 開発用のイメージ (compose.yaml、docs/development.md)。ソースはマウントして使い、Go と Node.js は mise で入れる。
# ソースの変更を watchexec で見て、ビルドし直す。生成したファイルが手元のユーザーの持ち物になるよう、UID と GID をそろえる。
FROM runtime AS dev
# Node.js は libatomic を使う。runtime の段には入らないので、ここで入れる。
RUN <<EOF
  apt-get update
  apt-get install -y --no-install-recommends git libatomic1
  rm -rf /var/lib/apt/lists/*
EOF
COPY --from=docker.io/jdxcode/mise:2026.11.9@sha256:9a182adba2584a6f5b29ddc1f2c7651c93694a5f829d74452b90214616baacf4 /usr/local/bin/mise /usr/local/bin/mise
ARG UID=1000
ARG GID=1000
# 名前付きボリュームは、最初にマウントしたときに、イメージのディレクトリの持ち主を引き継ぐ。
# ソースのマウントの中にある frontend/node_modules も、先に作っておく。
RUN <<EOF
  groupadd -o -g "${GID}" kotatsu
  useradd -o -m -u "${UID}" -g "${GID}" -d /home/kotatsu kotatsu
  mkdir -p /mise /go/pkg/mod /go/cache /src/frontend/node_modules
  chown -R "${UID}:${GID}" /mise /go /src /home/kotatsu
EOF
USER kotatsu
ENV MISE_DATA_DIR=/mise \
    MISE_CACHE_DIR=/mise/cache \
    MISE_ENV=dev \
    MISE_TRUSTED_CONFIG_PATHS=/src \
    PATH=/mise/shims:$PATH \
    GOMODCACHE=/go/pkg/mod \
    GOCACHE=/go/cache
# 足りないツールをその場で入れて mise.lock を書き換えないよう、イメージに入れたツールだけを使う。
ENV MISE_AUTO_INSTALL=false
# pnpm のストアは、ハードリンクが効くよう、frontend/node_modules と同じボリュームに置く。
# 置き場所を決めないと、マウントしたソースの直下に作ってしまう。
ENV pnpm_config_store_dir=/src/frontend/node_modules/.pnpm-store
WORKDIR /src
# 開発に使うツールだけを、イメージに入れておく。
COPY --chown=${UID}:${GID} mise.toml mise.lock mise.dev.toml mise.dev.lock ./
COPY --chown=${UID}:${GID} backend/mise.toml backend/mise.lock backend/mise.dev.toml ./backend/
# mise install は、そのディレクトリの config root のツールだけを入れるので、backend/ のツールは backend/ で入れる。
RUN mise install go node pnpm watchexec && mise -C backend install golangci-lint sqlc
ENTRYPOINT ["mise"]

FROM runtime AS ffmpeg
# distroless の nonroot と同じ UID にそろえる (設計書 2.4)。
RUN <<EOF
  groupadd --gid 65532 nonroot
  useradd --uid 65532 --gid 65532 --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin nonroot
EOF
COPY --from=build /out/kotatsu /usr/local/bin/kotatsu
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kotatsu"]
