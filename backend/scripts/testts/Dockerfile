# テスト用の TS を作るためのイメージ。scripts/generate-test-ts.sh から使う。
FROM debian:13-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f

ARG TARGETARCH
# renovate: datasource=github-releases depName=tsduck/tsduck extractVersion=^v(?<version>.+)$ versioning=loose
ARG TSDUCK_VERSION=3.45-4798
# TSDuck の公式のパッケージを、チェックサムを確かめてから入れる。バージョンを上げたら両方とも更新する。
ARG TSDUCK_SHA256_AMD64=b9960bb4347384342e583f3df7301c1c23489956ad0acf3e8f05703ff36c2259
ARG TSDUCK_SHA256_ARM64=f529beac3a5b7962d3d1c9be083e013212de03e4eba55d6d5288c64126f166fc

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
  && case "${TARGETARCH}" in \
       amd64) sha256="${TSDUCK_SHA256_AMD64}" ;; \
       arm64) sha256="${TSDUCK_SHA256_ARM64}" ;; \
       *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
     esac \
  && curl -fsSL -o /tmp/tsduck.deb "https://github.com/tsduck/tsduck/releases/download/v${TSDUCK_VERSION}/tsduck_${TSDUCK_VERSION}.debian13_${TARGETARCH}.deb" \
  && echo "${sha256}  /tmp/tsduck.deb" | sha256sum -c - \
  && apt-get install -y --no-install-recommends /tmp/tsduck.deb \
  && rm -rf /tmp/tsduck.deb /var/lib/apt/lists/*

COPY generate.sh generate-codecs.sh /work/
COPY tables/ /work/tables/
WORKDIR /work
ENTRYPOINT ["/work/generate.sh"]
