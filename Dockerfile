# syntax=docker/dockerfile:1
# Kotatsu のコンテナイメージ (設計書 2.4、11.1、付録 B)。
#
#   - static: distroless の static イメージ。tenban、neko、mikan 向け。
#   - ffmpeg: Debian 13 の slim に FFmpeg と tsreadex を入れたイメージ。kyusu、nekonote、all 向け。
#
# docker buildx build --platform linux/amd64,linux/arm64 --target static . のようにビルドする。
# Docker Hub の取得の回数の上限に当たらないよう、Docker Hub の公式イメージは Google のミラーから取得する。

# renovate: datasource=github-tags depName=xtne6f/tsreadex versioning=loose
ARG TSREADEX_VERSION=master-260428

# SPA は、どのアーキテクチャでも同じものになるので、ビルドするマシンのアーキテクチャで 1 回だけビルドする。
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/node:24.21.0-trixie-slim@sha256:173f125896c3b47ddf056734c7ea789d04595a6a08769a8f78e0df642781fb66 AS web
WORKDIR /src/web
# pnpm の版は package.json の packageManager に従う。
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
# ワークスペースのパッケージ (ビルドした mpeg2toh264) も、依存を入れる前に置く。
COPY web/vendor/ ./vendor/
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm exec vp build

# Go のバイナリは、ビルドするマシンのアーキテクチャのまま、対象のアーキテクチャへクロスコンパイルする。
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.27-trixie@sha256:2f84bc93ecfb2689f782b153fdcd368b5a7ab96c1386c65cdaccf35e726d6a44 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist/app ./web/dist/app
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/kotatsu ./cmd/kotatsu

# tsreadex は、配布されているバイナリがないので、対象のアーキテクチャでソースからビルドする。
FROM mirror.gcr.io/library/debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS tools
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates g++ git make \
    && rm -rf /var/lib/apt/lists/*
ARG TSREADEX_VERSION
WORKDIR /build
RUN git clone --depth 1 --branch "$TSREADEX_VERSION" https://github.com/xtne6f/tsreadex.git \
    && make -C tsreadex
# ライセンス文書と、ビルドに使ったリポジトリとコミットを残す (付録 B)。
RUN mkdir -p /out/doc \
    && for name in tsreadex; do \
        cp "$name/$name" /out/ \
        && cp "$name/License.txt" "/out/doc/$name.License.txt" \
        && echo "$name https://github.com/xtne6f/$name $(git -C "$name" rev-parse HEAD)" >> /out/doc/SOURCES; \
    done

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3 AS static
COPY --from=build /out/kotatsu /usr/local/bin/kotatsu
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kotatsu"]

FROM mirror.gcr.io/library/debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS ffmpeg
ARG TARGETARCH
# FFmpeg は Debian の ffmpeg パッケージ (GPL-2.0-or-later) を使い、non-free のパッケージは入れない (#77)。
# VAAPI と QSV のドライバは main コンポーネントのものだけを入れる。Intel のものは amd64 にしかない。
RUN apt-get update \
    && drivers="mesa-va-drivers" \
    && if [ "$TARGETARCH" = "amd64" ]; then drivers="$drivers intel-media-va-driver libvpl2 libmfx-gen1.2"; fi \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg $drivers \
    && rm -rf /var/lib/apt/lists/*
COPY --from=tools /out/tsreadex /usr/local/bin/
COPY --from=tools /out/doc/ /usr/share/doc/kotatsu/third-party/
COPY build/container/write-debian-sources.sh /tmp/
# 入れた Debian のパッケージとソースパッケージの一覧を、ソースの入手先とともに書き出す (付録 B)。
RUN sh /tmp/write-debian-sources.sh /usr/share/doc/kotatsu/third-party/debian-packages.tsv && rm /tmp/write-debian-sources.sh
# distroless の nonroot と同じ UID にそろえる (設計書 2.4)。
RUN groupadd --gid 65532 nonroot \
    && useradd --uid 65532 --gid 65532 --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin nonroot
COPY --from=build /out/kotatsu /usr/local/bin/kotatsu
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kotatsu"]
