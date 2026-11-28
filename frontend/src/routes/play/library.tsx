import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Captions } from 'lucide-react';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useMutation, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../auth/returnTo';
import { ErrorBoundary, PageLoader, ProgramTitle, formatBytes } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { broadcastTime, formatDuration } from '../../yunomi/position';
import { toVideoFormat } from '../../yunomi/route';
import { loadCmAutoSkip } from '../../yunomi/cmAutoSkip';
import { cmMergeGap, cmSectionAt, mergeCmSections } from '../../yunomi/cmSkip';
import { SaveOfflineButton } from '../../offline/SaveOfflineButton';
import {
  AutoCaptionsStatus,
  autoCaptionsActive,
  captionModelsAnchor,
} from '../../recordings/AutoCaptions';
import type { libraryPlayGenerateCaptionsMutation } from './__generated__/libraryPlayGenerateCaptionsMutation.graphql';
import { browserStorage } from '../../yunomi/volume';
import { playableStart } from '../../yunomi/reconnect';
import { parseStartSeconds } from '../../yunomi/shareUrl';
import { useLibraryPositionSaver } from '../../yunomi/useWatchPositionSaver';
import { useHostedWatch, useWatchMini } from '../../yunomi/WatchHost';
import {
  clampSeek,
  fileDuration,
  libraryResumePosition,
  parentPath,
  seekLimit,
} from '../../library/library';
import type { libraryPlayFileQuery } from './__generated__/libraryPlayFileQuery.graphql';
import { VodPlayer } from '../../yunomi/VodPlayer';

// noGaps は、ライブラリのファイルの途切れた区間。ファイル 1 本を流すので、録画のセグメントの間のような区間はない。
const noGaps: readonly { from: string; to: string }[] = [];

// PlaySearch は、再生の画面の URL のクエリ。library と path は再生するファイル、t は再生を始める位置 (ファイルの始まりからの秒)。
type PlaySearch = { library: string; path: string; t?: number };

export const Route = createFileRoute('/play/library')({
  // 右のカラムを置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => {
    const t = parseStartSeconds(search.t);

    return {
      library: typeof search.library === 'string' ? search.library : '',
      path: typeof search.path === 'string' ? search.path : '',
      ...(t === undefined ? {} : { t }),
    };
  },
  component: PlayPage,
});

// PlayPage は、ライブラリのファイル (録画として登録していない TS) を再生する画面 (設計書 5.8、#1734)。
// 録画と同じく、ブラウザの中で変換するか、kyusu の仮想 HLS で再生する。画面は、アプリのルート要素に描く (#1602)。
function PlayPage() {
  const { library, path, t: start } = Route.useSearch();

  return useHostedWatch(<PlayWatch library={library} path={path} start={start} />);
}

function PlayWatch({
  library,
  path,
  start,
}: {
  library: string;
  path: string;
  start: number | undefined;
}) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();
  const parent = parentPath(path);

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor
          size="sm"
          renderRoot={(props) => (
            <Link
              {...props}
              to="/library"
              search={{ library, ...(parent === '' ? {} : { path: parent }) }}
            />
          )}
        >
          {t('library.backToFolder')}
        </Anchor>
      )}
      {/* 同じルートのまま別のファイルへ移ったときは、前のファイルのエラーを残さず読み直す (#1453)。 */}
      <ErrorBoundary
        resetKey={`${library}:${path}`}
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor
                component={Link}
                to="/login"
                onClick={() => rememberReturnTo(window.location.pathname + window.location.search)}
              >
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">
              {graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('library.fileNotFound')
                : graphQLErrorDetails(error).code === 'BAD_USER_INPUT'
                  ? t('library.unsupported')
                  : graphQLErrorDetails(error).code === 'CONFLICT'
                    ? t('library.busy')
                    : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Player key={`${library}:${path}`} library={library} path={path} start={start} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// playQuery は、再生するファイル。シークのインデックスを作っている間は、進み具合を知るために読み直す。
const playQuery = graphql`
  query libraryPlayFileQuery($library: String!, $path: String!) {
    libraryFile(library: $library, path: $path) {
      id
      fileId
      name
      size
      modifiedAt
      videoFormat
      watchPositionMs
      streamUrl
      hlsUrl
      library {
        name
        kind
      }
      jikkyoCommentsUrl
      thumbnailUrl
      previewUrl
      cmSections {
        startSeconds
        endSeconds
      }
      autoCaptions {
        startSeconds
        endSeconds
        text
      }
      autoCaptionsStatus {
        state
        generatedAt
        model
        count
        progress {
          ratio
          remainingSeconds
        }
      }
      broadcast {
        serviceName
        jikkyoChannelId
        recordedFrom
        recordedUntil
        service {
          name
          logoUrl
        }
        program {
          name
          description
          extended {
            heading
            text
          }
          startAt
          endAt
        }
      }
      audios {
        componentType
        languages
        samplingRate
      }
      quality {
        drops
        errors
        scrambled
      }
      index {
        complete
        writing
        seekableSeconds
        durationSeconds
      }
    }
    viewer {
      role
    }
    transcoding {
      available
      presets {
        name
        codec
        passthrough
        videoBitrate
      }
    }
  }
`;

// refreshMs は、シークのインデックスを作っている間に、ファイルを読み直す間隔。
const refreshMs = 5_000;

// probeRefreshMs は、インデックスを作り始めてから、放送の情報 (実況のチャンネルと時間帯) が分かるまで、ファイルを読み直す間隔。
// 放送の情報は、ファイルの先頭と終わりだけを読んで、インデックスを作り終えるよりずっと早く分かるので、過去ログを早く読み始めるために縮める (#1910)。
const probeRefreshMs = 1_000;

// imagesRefreshMs は、インデックスを作り終えた後、シークバーの画像 (nekonote のジョブで作る) が届くまで、ファイルを読み直す間隔。
const imagesRefreshMs = 30_000;

function Player({
  library,
  path,
  start,
}: {
  library: string;
  path: string;
  start: number | undefined;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const variables = { library, path };
  const {
    libraryFile: file,
    transcoding,
    viewer,
  } = useLazyLoadQuery<libraryPlayFileQuery>(playQuery, variables);
  const autoCaptions = file.autoCaptions.map((caption) => ({
    from: caption.startSeconds,
    to: caption.endSeconds,
    text: caption.text,
  }));
  const [generateCaptions, generatingCaptions] = useMutation<libraryPlayGenerateCaptionsMutation>(
    graphql`
      mutation libraryPlayGenerateCaptionsMutation($id: ID!) {
        generateLibraryCaptions(id: $id) {
          id
          autoCaptionsStatus {
            state
            generatedAt
            model
            count
            progress {
              ratio
              remainingSeconds
            }
          }
        }
      }
    `,
  );
  const [captionsError, setCaptionsError] = useState(false);
  // シークのインデックスを作り終えるまで、読み直して、シークできる所と長さを伸ばす (設計書 5.8)。
  const environment = useRelayEnvironment();
  // 書いている途中のファイルは、インデックスを作らないので、読み直さない (設計書 5.8)。
  const indexing = !file.index.complete && !file.index.writing;
  // インデックスを作り終えた後も、シークバーの画像と CM の区間が届くまで、間を空けて読み直す。
  // CM の区間は、映像と音声を取り出すサービスが TS から分かったファイルだけで見分ける。
  // 頼まれた字幕の自動生成も、待っているか生成している間は読み直す。
  const waitingJobs =
    (file.index.complete &&
      (file.thumbnailUrl == null || (file.broadcast != null && file.cmSections == null))) ||
    autoCaptionsActive(file.autoCaptionsStatus.state);
  const interval = indexing
    ? file.broadcast == null
      ? probeRefreshMs
      : refreshMs
    : waitingJobs
      ? imagesRefreshMs
      : null;
  useEffect(() => {
    if (interval === null) {
      return;
    }
    const timer = setInterval(() => {
      fetchQuery<libraryPlayFileQuery>(environment, playQuery, variables, {
        fetchPolicy: 'network-only',
      })
        .toPromise()
        .catch(() => {});
    }, interval);

    return () => clearInterval(timer);
    // variables は描画ごとに作り直すが、内容は library と path だけで決まる。
  }, [environment, interval, library, path]);
  // duration は、シークバーの長さ (秒)。インデックスを作っている途中は見込みの長さ。limit は、シークできる所。
  const duration = fileDuration(file.index);
  const limit = seekLimit(file.index);
  // cmPositions は、CM とみなした区間 (設計書 5.8)。再生の 0 秒からの秒で受け取る。まだ見分けていなければ空。
  const cmPositions = (file.cmSections ?? []).map((section) => ({
    from: section.startSeconds,
    to: section.endSeconds,
  }));
  // cmSpans は、自動で飛ばすときに使う、続いた区間をまとめた CM の区間 (#1512)。
  const cmSpans = mergeCmSections(cmPositions, cmMergeGap);
  // opening は、再生を始める位置 (秒)。URL で位置を渡されたらそこから、なければ前回の続きから (設計書 7.5)。
  // CM を自動で飛ばす設定で、URL で位置を渡されずに開いた位置が CM の中なら、録画と同じく、最初から CM の終わりを求める (#1512)。
  // Int64 は Relay の型では any になるので、数に直す。
  const [opening] = useState(() => {
    const savedMs = file.watchPositionMs == null ? null : Number(file.watchPositionMs);
    let at =
      start === undefined
        ? libraryResumePosition(savedMs, file.index)
        : clampSeek(start, file.index);
    const span =
      start === undefined && loadCmAutoSkip(browserStorage()) ? cmSectionAt(cmSpans, at) : null;
    if (span !== null) {
      at = clampSeek(span.to, file.index);
    }

    // 終わりの直前から読むと、映像のコマが届かずに失敗するので、映像が入るだけ手前から始める (#1576)。
    return {
      at: file.index.complete ? playableStart(at, duration) : at,
      skipped: span !== null,
      resumed: savedMs !== null,
    };
  });
  const initialPosition = opening.at;
  // broadcast は、TS の中の SDT、EIT[p/f]、TOT から読んだ放送の情報 (設計書 5.8)。再生の 0 秒を、録り始め (recordedFrom) の放送時刻とみなす。
  // ファイルは 1 本なので、途切れた区間は数えない。
  const broadcast = file.broadcast ?? null;
  const recordedFrom = broadcast?.recordedFrom ?? null;
  const clockAt = (seconds: number): number | null =>
    recordedFrom == null ? null : broadcastTime(recordedFrom, noGaps, seconds);
  const serviceName = broadcast?.service?.name ?? (broadcast?.serviceName || file.library.name);
  const program = broadcast?.program ?? null;
  // channelId は、実況のチャンネルの ID。Kotatsu にない局でも、ネットワークとサービス ID で検索する (設計書 7.4)。
  const channelId = broadcast?.jikkyoChannelId ?? null;
  // spanKnown は、インデックスを作り終える前に、放送の情報から過去ログの時間帯が分かるかどうか (#1910)。
  // 終わりの TOT が始まりの TOT より後なら分かる。短いファイルでは TOT を 1 度しか読めないので、インデックスから長さが分かるまで待つ。
  const recordedUntil = broadcast?.recordedUntil ?? null;
  const spanKnown =
    recordedFrom != null &&
    recordedUntil != null &&
    new Date(recordedUntil).getTime() > new Date(recordedFrom).getTime();
  const backToFolder = () =>
    void navigate({
      to: '/library',
      search: { library, ...(parentPath(path) === '' ? {} : { path: parentPath(path) }) },
    });
  // info は、見出しの下に出す、ファイルの場所と大きさ、端末への保存、字幕の自動生成、インデックスの様子。
  const info = (
    <>
      <Text size="sm" c="dimmed">
        {/* Google Drive のパスは ID をつないだものなので、名前を出す (#1902)。 */}
        {file.library.kind === 'GOOGLE_DRIVE' ? file.name : path} · {formatBytes(Number(file.size))}
      </Text>
      {/* 端末に保存 (設計書 7.8)。録画と同じく、配信の TS をそのまま書く。端末での再生は、録り始めと録り終わりの放送時刻で
          時計と長さを出すので、TOT から読めたファイルだけで出す。書いている途中か、インデックスを作り終えていないファイルは、長さが決まらないので出さない。 */}
      {file.index.complete &&
        !file.index.writing &&
        broadcast?.recordedFrom != null &&
        broadcast.recordedUntil != null && (
          <SaveOfflineButton
            videoFormat={file.videoFormat}
            source={{
              // 端末に保存するときの名前。GraphQL の ID はパスを含んで長いので、パスを含まないファイルの ID を使う。
              id: `library-${file.fileId}`,
              name: program?.name ?? file.name,
              serviceName,
              startAt: program?.startAt ?? broadcast.recordedFrom,
              endAt: program?.endAt ?? broadcast.recordedUntil,
              recordedFrom: broadcast.recordedFrom,
              recordedUntil: broadcast.recordedUntil,
              gaps: noGaps,
              streamUrl: file.streamUrl,
              // 実況のチャンネルのない局では、過去ログを読まない (設計書 7.4)。
              jikkyoCommentsUrl: channelId === null ? null : file.jikkyoCommentsUrl,
              // 配信は Content-Length を返さないので、ファイルの大きさで空きを見積もる。
              files: [{ kind: 'SEGMENT', size: file.size }],
            }}
          />
        )}
      {/* 字幕の自動生成 (設計書 5.8)。録画と同じく、頼まれたときに生成する。音声を取り出すサービスが分からないファイルには作れない。 */}
      {broadcast !== null && (
        <Stack gap={4}>
          <Group gap="xs">
            <Text size="sm" fw={600}>
              {t('recordings.autoCaptions.title')}
            </Text>
            {viewer.role !== 'VIEWER' && (
              <Button
                size="compact-xs"
                variant="subtle"
                leftSection={<Captions size={14} aria-hidden />}
                loading={generatingCaptions}
                onClick={() =>
                  generateCaptions({
                    variables: { id: file.id },
                    onCompleted: (_response, errors) => setCaptionsError((errors ?? []).length > 0),
                    onError: () => setCaptionsError(true),
                  })
                }
              >
                {t('recordings.generateCaptions')}
              </Button>
            )}
          </Group>
          <AutoCaptionsStatus
            status={file.autoCaptionsStatus}
            settingsLink={
              // モデルを置けるのは管理者だけなので、録画の詳細と同じく、管理者にだけ、置く画面へのリンクを出す。
              viewer.role === 'ADMIN' && (
                <Anchor component={Link} to="/admin/recording" hash={captionModelsAnchor} size="sm">
                  {t('recordings.autoCaptions.notConfiguredLink')}
                </Anchor>
              )
            }
          />
          {captionsError && (
            <Text size="sm" c="red" role="alert">
              {t('recordings.actionError')}
            </Text>
          )}
        </Stack>
      )}
      {indexing && (
        <Text size="sm" c="dimmed" role="status">
          {t('library.indexingStatus', { seconds: formatDuration(limit) })}
        </Text>
      )}
      {file.index.writing && (
        <Text size="sm" c="dimmed" role="status">
          {t('library.writing')}
        </Text>
      )}
    </>
  );

  return (
    <VodPlayer
      source={{
        kind: 'RECORDING',
        target: file.id,
        program: { id: file.id, name: program?.name ?? file.name },
        title: program === null ? file.name : <ProgramTitle name={program.name} />,
        serviceName,
        serviceLogoUrl: broadcast?.service?.logoUrl ?? null,
        time:
          program?.startAt != null && program.endAt != null
            ? { startAt: program.startAt, endAt: program.endAt }
            : null,
        description:
          program === null ? null : { text: program.description, extended: program.extended },
        artwork: null,
        previewUrl: file.previewUrl ?? null,
        // 録画と同じく、ブラウザの中で変換するか、kyusu の仮想 HLS で再生する (設計書 7.1)。
        transcoding,
        videoFormat: toVideoFormat(file.videoFormat),
        // 終わりが伸びないので、デコードが追いつかずにほかの画面で下げた段階の上限は引き継がない (#1513)。録画を終えた録画と同じ。
        inheritCap: false,
        streamUrl: file.streamUrl,
        // kyusu は、インデックスの I フレームが 1 つもない間は、プレイリストを作れずに 503 を返す。インデックスができ始めるまで読まない。
        hlsUrl: indexing && file.index.durationSeconds == null ? null : file.hlsUrl,
        audios: file.audios.length === 0 ? null : file.audios,
        duration,
        seekLimit: limit,
        opening,
        initialSeek: start === undefined ? null : initialPosition,
        initial: initialPosition,
        // シークできる所 (インデックスのできた所) より先へは移らない。インデックスを作っている途中は、長さが見込みなので、
        // 途中で切れたかを見分けない。
        clampSeek: (seconds) => clampSeek(seconds, file.index),
        endKnown: !indexing,
        growing: false,
        clockAt,
        // bands は、インデックスをまだ作っている所。シークできないので帯で示す。
        bands:
          indexing && duration > limit
            ? [{ from: limit, to: duration, label: t('library.indexing') }]
            : [],
        // cmSections は、CM とみなした区間 (設計書 5.8)。まだ見分けていなければ空。
        cmSections: cmPositions,
        jikkyo:
          channelId === null
            ? null
            : {
                // 過去ログは、時間帯が分かったら、インデックスを作り終える前でも読む。作っている途中は、終わりがインデックスの長さまで
                // 伸びうるので、作り終えたら再取得して、足りなかったコメントを足す (設計書 5.8)。
                url: file.index.complete || spanKnown ? file.jikkyoCommentsUrl : null,
                live: null,
                others: null,
                nicolog: null,
                growing: !file.index.complete,
                recordedFrom,
                gaps: noGaps,
              },
        autoCaptions,
        // ライブラリのファイルは、ほかの端末から操作させない。
        remote: false,
        topBitrate: null,
        streamInfo: {
          average: null,
          transport:
            file.quality == null
              ? null
              : {
                  dropped: Number(file.quality.drops),
                  errors: Number(file.quality.errors),
                  scrambled: Number(file.quality.scrambled),
                },
        },
        recordingId: null,
        // スリープタイマーと「再生をやめる」のキーは、プレイヤーを閉じて、ファイルのあったフォルダへ戻る (#1426)。
        stop: backToFolder,
        info,
      }}
    >
      {/* 再生位置を、録画と同じく、ユーザーごとに保存する。 */}
      {(api) => (
        <LibraryPositionSaver
          fileId={file.id}
          currentPosition={api.currentPosition}
          playing={!api.paused}
        />
      )}
    </VodPlayer>
  );
}

// LibraryPositionSaver は、ライブラリのファイルの再生位置を、ユーザーごとに保存する。
function LibraryPositionSaver({
  fileId,
  currentPosition,
  playing,
}: {
  fileId: string;
  currentPosition: () => number;
  playing: boolean;
}) {
  useLibraryPositionSaver(fileId, currentPosition, playing);

  return null;
}
