import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Captions, MessageSquareText, Share2 } from 'lucide-react';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useMutation, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../auth/returnTo';
import { ErrorBoundary, PageLoader, ProgramTitle, formatBytes } from '../../futon';
import { useRecordedJikkyo } from '../../jikkyo/useRecordedJikkyo';
import { CommentList } from '../../jikkyo/CommentList';
import type { JikkyoComment } from '../../jikkyo/comments';
import { countUntil, listedUntil } from '../../jikkyo/commentList';
import { CommentOffsetSetting } from '../../jikkyo/CommentOffsetSetting';
import { findPartMarkers } from '../../jikkyo/partMarkers';
import { formatCommentOffset, useCommentOffsetCommands } from '../../jikkyo/commentOffset';
import { isSurroundAudio, useVirtualSurround } from '../../yunomi/useVirtualSurround';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { WatchPanel } from '../../yunomi/WatchPanel';
import {
  WatchDescription,
  WatchHeading,
  WatchLayout,
  watchCommentsHeight,
} from '../../yunomi/WatchLayout';
import { CaptionHistory } from '../../yunomi/CaptionHistory';
import { StreamInfo, useStreamMeter } from '../../yunomi/StreamInfo';
import { useDRCSReplacements } from '../../yunomi/useDRCSReplacements';
import {
  insertRecordedCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../yunomi/captionHistory';
import { broadcastTime, formatClock, formatDuration } from '../../yunomi/position';
import { useFastForward } from '../../yunomi/fastForward';
import {
  RateSelect,
  CropSelect,
  AudioDelaySetting,
  NormalizeVolumeToggle,
  SleepTimerSelect,
  QualitySelect,
  VirtualSurroundToggle,
} from '../../yunomi/QualitySelect';
import { hlsURL, toVideoFormat } from '../../yunomi/route';
import { HlsPlayer } from '../../yunomi/HlsPlayer';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { type PlayerSnapshot } from '../../yunomi/playerState';
import { SeekBar } from '../../yunomi/SeekBar';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import type { PlayerCommands } from '../../yunomi/commands';
import { useCapture } from '../../yunomi/PlayerTools';
import {
  CommandButton,
  PlayButton,
  PlayerControls,
  PositionStatus,
  SeekStepButtons,
  SettingsButton,
  VolumeControl,
} from '../../yunomi/PlayerControls';
import { loadCmAutoSkip } from '../../yunomi/cmAutoSkip';
import { cmMergeGap, cmSectionAt, mergeCmSections } from '../../yunomi/cmSkip';
import { SaveOfflineButton } from '../../offline/SaveOfflineButton';
import {
  CaptionTrackSelect,
  PositionedAutoCaptionOverlay,
  type CaptionTrack,
} from '../../yunomi/AutoCaptionOverlay';
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
import {
  PlayerBox,
  PlayerErrorOverlay,
  PlayerFeedback,
  PlayerFrame,
  PlayerToolButtons,
  usePlayerInputs,
  usePlayerPresence,
  usePlayerShell,
} from '../../yunomi/playerShell';
import { useVodTimeline } from '../../yunomi/useVodTimeline';
import { useCmSkip } from '../../yunomi/useCmSkip';

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
  // shell は、再生元によらないプレイヤーの外側 (#2244)。
  const shell = usePlayerShell();
  const { mediaRef, media, setMedia, visible, controls, videoState, showFlash } = shell;
  // 録画と同じく、ブラウザの中で変換するか、kyusu の仮想 HLS で再生する (設計書 7.1)。
  const { route, choices, quality, setQuality } = usePlaybackRoute(
    transcoding,
    toVideoFormat(file.videoFormat),
  );
  const hls = route.kind === 'kyusu';
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    setJikkyoEnabled,
    comments: commentSettings,
    muteUser,
    captions: captionSettings,
  } = usePlayerSettings();
  // captionTrack は、出す字幕の種類 (#1682)。自動生成した字幕があるときだけ選べ、既定は放送の字幕にする。
  const [captionTrack, setCaptionTrack] = useState<CaptionTrack>('broadcast');
  const autoCaptions = file.autoCaptions.map((caption) => ({
    from: caption.startSeconds,
    to: caption.endSeconds,
    text: caption.text,
  }));
  const hasAutoCaptions = autoCaptions.length > 0;
  const autoCaptionsShown = captions && hasAutoCaptions && captionTrack === 'auto';
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
  // timeline は、ファイルの始まりからの秒で、位置を指定して読み直す配信 (設計書 7.2)。kyusu の経路では、仮想 HLS を流す。
  // 再生できない経路 (8K や kyusu のない UHD) では、配信に HEAD を送らない。シークできる所 (インデックスのできた所) より
  // 先へは移らない。インデックスを作っている途中は、長さが見込みなので、途中で切れたかを見分けない。
  const timeline = useVodTimeline({
    streamUrl: file.streamUrl,
    media,
    mediaRef,
    convert: route.kind === 'convert',
    hls,
    initial: initialPosition,
    duration,
    clampSeek: (seconds) => clampSeek(seconds, file.index),
    endKnown: !indexing,
  });
  const {
    stream,
    hlsStream,
    position,
    positionRef,
    currentPosition,
    restartAt,
    jumpTo,
    retry,
    error,
  } = timeline;
  const [seeking, setSeeking] = useState<number | null>(null);
  // 再生位置を、録画と同じく、ユーザーごとに保存する。
  useLibraryPositionSaver(file.id, currentPosition, !videoState.paused);
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
  const jikkyo = useRecordedJikkyo(
    // 過去ログは、時間帯が分かったら、インデックスを作り終える前でも読む。作っている途中は、終わりがインデックスの長さまで伸びうるので、
    // 作り終えたら再取得して、足りなかったコメントを足す (設計書 5.8)。
    channelId !== null && jikkyoEnabled && (file.index.complete || spanKnown)
      ? file.jikkyoCommentsUrl
      : null,
    currentPosition,
    visible,
    null,
    commentSettings,
    null,
    null,
    !file.index.complete,
  );
  // listedCount は、再生位置までに流れたコメントの数。一覧は、この数が変わったときだけ作り直す (JikkyoRows)。
  const listedCount =
    jikkyo.comments === null ? 0 : countUntil(jikkyo.comments, position - jikkyo.offset);
  // partBands は、実況のコメントから見つけた番組の区切りのマーク (#1358)。コメントの位置をずらしたら、マークも同じだけ動かす。
  const partBands = findPartMarkers(jikkyo.ownComments ?? []).map((marker) => {
    const at = Math.max(marker.time + jikkyo.offset, 0);
    const part = t(`player.partMarker.${marker.part}`);

    return {
      from: at,
      to: at,
      label: marker.noCmBefore ? t('player.partMarkerNoCm', { part }) : part,
      kind: 'part' as const,
    };
  });
  const jikkyoLabel =
    jikkyo.state === null
      ? t('jikkyo.toggle')
      : t('jikkyo.toggleWith', {
          state:
            jikkyo.state.kind === 'loading'
              ? t('jikkyo.kakologLoading')
              : jikkyo.state.kind === 'error'
                ? t('jikkyo.kakologError')
                : jikkyo.state.count === 0
                  ? t('jikkyo.kakologEmpty')
                  : t('jikkyo.kakologLoaded', { count: jikkyo.state.count }),
        });

  const streamMeter = useStreamMeter(quality);
  // 仮想サラウンド (#454)。5.1ch の番組を、ブラウザの中での変換で見ているときだけ使える。
  const audios = file.audios.length === 0 ? null : file.audios;
  const surroundAvailable = route.kind === 'convert' && isSurroundAudio(audios);
  useVirtualSurround(media, surroundAvailable);
  // captionEntries は、字幕の履歴 (#450)。字幕が出たときの再生位置で並べ、行を押すとその位置へ移る。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  const drcs = useDRCSReplacements();
  // currentCaption は、今出ている字幕の文。キャプチャに書き込む (#458)。
  const currentCaption = useRef<string | null>(null);
  const onCaptionText = (text: string) => {
    const normalized = normalizeCaption(text);
    currentCaption.current = normalized === '' ? null : normalized;
    const at = currentPosition();
    setCaptionEntries((list) => insertRecordedCaption(list, at, text));
  };
  // cm は、CM とみなした区間 (設計書 5.8) の帯と、飛ばすボタンと、録画と同じく自動で飛ばす判定 (#1512)。
  const cm = useCmSkip({
    sections: cmPositions,
    media,
    mediaRef,
    framePosition: (time) => (hls ? hlsStream.framePosition(time) : stream.framePosition(time)),
    jumpTo,
    currentPosition,
    scrubbing: seeking !== null,
    showFlash,
    opening,
    initialSeek: start === undefined ? null : initialPosition,
  });
  // seek は、利用者の操作で、指定の位置に移る。skip は、今の位置から相対に移り、CM を飛んだら true を返す。
  const { seek, skip } = cm;
  const changeQuality = (next: string) => {
    restartAt(positionRef.current);
    setQuality(next);
  };
  // 音声は、経路ごとのメニューで選ぶ。音声の構成は、インデックスを作るときに TS の EIT[p/f] から読む。読めなければプレイヤーに任せる。
  const audio = usePlayerAudio(route, audios, () => restartAt(positionRef.current));
  const [rate, setRate] = useState(1);
  const fastForward = useFastForward(rate, { growing: false, position, duration });
  useEffect(() => {
    if (media !== null) {
      media.defaultPlaybackRate = rate;
      media.playbackRate = fastForward.playbackRate;
    }
  }, [rate, fastForward.playbackRate, media, stream.url, hlsStream.loads]);

  const shownFlash = fastForward.active
    ? t('player.flash.fastForward', { rate: fastForward.playbackRate })
    : shell.flash;
  // キャプチャ (#908)。放送時刻は、TS の TOT から読んだ録り始めから数える。分からなければ、撮った時刻にする。
  const capture = useCapture({
    crop: shell.crop,
    frameRef: shell.playerBoxRef,
    media: () => mediaRef.current,
    serviceName,
    programName: program?.name ?? file.name,
    captureTime: () => clockAt(currentPosition()) ?? Date.now(),
    caption: () => currentCaption.current,
  });
  const saveCapture = async () => {
    if ((await capture.capture('save')) === 'saved') {
      showFlash(t('player.flash.captured'));
    }
  };
  const backToFolder = () =>
    void navigate({
      to: '/library',
      search: { library, ...(parentPath(path) === '' ? {} : { path: parentPath(path) }) },
    });
  // commentOffsetCommands は、実況のコメントの位置 (#1357) の操作。変えたら、ずらした量を映像の上に短く出す。
  const commentOffsetCommands = useCommentOffsetCommands((ms) =>
    showFlash(t('player.flash.commentOffset', { seconds: formatCommentOffset(ms) })),
  );
  const commands: PlayerCommands = {
    ...shell.commands,
    ...shell.seekCommands({ skip, seekTo: seek, currentPosition, retry }),
    ...(channelId === null || !jikkyoEnabled ? {} : commentOffsetCommands),
    ...(channelId === null ? {} : { toggleJikkyo: () => setJikkyoEnabled(!jikkyoEnabled) }),
    capture: () => void saveCapture(),
    toggleCaptions: () => setCaptions(!captions),
    // スリープタイマーと「再生をやめる」のキーは、プレイヤーを閉じて、ファイルのあったフォルダへ戻る (#1426)。
    stop: backToFolder,
  };
  const seekLabel = (value: number) => {
    const at = clockAt(value);

    return at === null ? formatDuration(value) : `${formatDuration(value)} (${formatClock(at)})`;
  };
  const inputs = usePlayerInputs(shell, commands, {
    // CM を飛んだら、CM を飛ばした通知を出したので、シークの結果は出さない。
    onDoubleTap: (side) => {
      if (!skip(side === 'left' ? -10 : 10)) {
        showFlash(t(side === 'left' ? 'player.flash.seekBackward' : 'player.flash.seekForward'));
      }
    },
    slideSeek: {
      enabled: limit > 0,
      range: () => ({ position: currentPosition(), duration }),
      onScrub: (value) => setSeeking(value === null ? null : Math.min(value, limit)),
      onCommit: (target) => {
        setSeeking(null);
        seek(target);
      },
      onCancel: () => setSeeking(null),
    },
  });
  const { sleep, scrub } = inputs;
  const player = {
    kind: 'RECORDING',
    target: file.id,
    serviceName,
    program: { id: file.id, name: program?.name ?? file.name },
    artwork: null,
    paused: videoState.paused,
    position,
    duration,
    playbackRate: fastForward.playbackRate,
    volume: videoState.volume,
    muted: videoState.muted,
    audios: audio.remote.choices,
    audio: audio.remote.selected,
    dataBroadcast: false,
    keyGroups: [],
  } satisfies PlayerSnapshot;
  // ライブラリのファイルは、ほかの端末から操作させない。
  usePlayerPresence(player, commands, null);

  const shown = seeking ?? position;
  const shownClock = clockAt(shown);
  // bands は、CM の区間と、インデックスをまだ作っている所 (シークできないので帯で示す) と、実況のコメントから見つけた番組の区切りのマーク。
  const bands = [
    ...cm.bands,
    ...(indexing && duration > limit
      ? [{ from: limit, to: duration, label: t('library.indexing') }]
      : []),
    ...partBands,
  ];
  // skipButton は、いま再生している CM の中で出す、区間の終わりへ飛ばすボタン (#1300)。
  const skipButton = seeking === null && cm.button(position, controls.visible);

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <SeekBar
          bands={bands}
          duration={duration}
          value={shown}
          previewUrl={file.previewUrl}
          label={seekLabel}
          onChange={(value) => setSeeking(Math.min(value, limit))}
          onChangeEnd={(value) => {
            setSeeking(null);
            seek(value);
          }}
        />
      }
      start={
        <>
          <PlayButton video={videoState} commands={commands} />
          <SeekStepButtons commands={commands} onFastForward={fastForward.setHeld} />
          <VolumeControl video={videoState} commands={commands} optional />
        </>
      }
      status={
        <PositionStatus
          position={formatDuration(shown)}
          rest={`${duration > 0 ? ` / ${formatDuration(duration)}` : ''}${
            shownClock === null ? '' : ` · ${formatClock(shownClock)}`
          }`}
        />
      }
      end={
        <>
          <CommandButton
            commands={commands}
            command="toggleCaptions"
            label={t('player.captions')}
            icon={Captions}
            active={captions}
          />
          {channelId !== null && (
            <CommandButton
              commands={commands}
              command="toggleJikkyo"
              label={jikkyoLabel}
              icon={MessageSquareText}
              active={jikkyoEnabled}
            />
          )}
          <SettingsButton>
            <QualitySelect choices={choices} quality={quality} onChange={changeQuality} />
            {audio.menu}
            <CaptionTrackSelect
              available={hasAutoCaptions}
              value={captionTrack}
              onChange={setCaptionTrack}
            />
            <RateSelect value={rate} onChange={setRate} />
            <CropSelect value={shell.crop} onChange={shell.setCrop} />
            <NormalizeVolumeToggle />
            <AudioDelaySetting />
            {channelId !== null && jikkyoEnabled && <CommentOffsetSetting />}
            <SleepTimerSelect timer={sleep} />
            {surroundAvailable && <VirtualSurroundToggle />}
          </SettingsButton>
          <PlayerToolButtons shell={shell} commands={commands} capture={capture} />
        </>
      }
    />
  );
  const overlay = (
    <>
      {autoCaptionsShown && (
        <PositionedAutoCaptionOverlay captions={autoCaptions} clock={currentPosition} />
      )}
      {jikkyo.overlay}
      <PlayerFeedback
        shell={shell}
        flash={shownFlash}
        scrub={
          scrub === null
            ? null
            : { seconds: scrub, label: seekLabel(scrub), previewUrl: file.previewUrl }
        }
        loadingHidden={error !== null}
      />
      {skipButton}
      <PlayerErrorOverlay shell={shell} error={error} onRetry={retry} />
    </>
  );

  const playerBox = (
    <PlayerBox shell={shell} inputs={inputs}>
      {route.kind === 'convert' && (
        <TsPlayer
          url={stream.url}
          handover={timeline.handover}
          mediaRef={mediaRef}
          onMedia={setMedia}
          controls={controlBar}
          captions={captions && !autoCaptionsShown}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={onCaptionText}
          onLoaded={timeline.onConvertLoaded}
          handoverTarget={stream.handoverTarget}
          onError={timeline.onError}
          onState={timeline.onConvertState}
          onAudio={audio.onAudio}
          onReading={streamMeter.onReading}
          audio={audio.playerAudio}
          overlay={overlay}
          coverRef={cm.coverRef}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          // 読み直すたびに URL を変え、hls.js を作り直す。fragment は、サーバーへは送られない。
          // kyusu は、インデックスの I フレームが 1 つもない間は、プレイリストを作れずに 503 を返す。インデックスができ始めるまで読まない。
          url={
            indexing && file.index.durationSeconds == null
              ? null
              : `${hlsURL(file.hlsUrl, route.preset, audio.kyusuAudio)}#${hlsStream.loads}`
          }
          mediaRef={mediaRef}
          onMedia={setMedia}
          onSegment={streamMeter.onSegment}
          controls={controlBar}
          captions={captions && !autoCaptionsShown}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={onCaptionText}
          live={false}
          startPosition={hlsStream.start}
          overlay={overlay}
          onLoaded={timeline.onHlsLoaded}
          onError={timeline.onError}
          coverRef={cm.coverRef}
        />
      )}
    </PlayerBox>
  );

  return (
    <WatchLayout
      player={
        <PlayerFrame shell={shell} route={route}>
          {playerBox}
        </PlayerFrame>
      }
      info={
        <>
          <WatchHeading
            title={program === null ? file.name : <ProgramTitle name={program.name} />}
            serviceName={serviceName}
            serviceLogoUrl={broadcast?.service?.logoUrl ?? null}
            time={
              program?.startAt != null && program.endAt != null
                ? { startAt: program.startAt, endAt: program.endAt }
                : null
            }
            actions={
              <Button
                variant="default"
                size="compact-sm"
                leftSection={<Share2 size={14} aria-hidden />}
                onClick={commands.copyPositionUrl}
              >
                {t('player.share')}
              </Button>
            }
          />
          {program !== null && (
            <WatchDescription text={program.description} extended={program.extended} />
          )}
          <Text size="sm" c="dimmed">
            {/* Google Drive のパスは ID をつないだものなので、名前を出す (#1902)。 */}
            {file.library.kind === 'GOOGLE_DRIVE' ? file.name : path} ·{' '}
            {formatBytes(Number(file.size))}
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
                        onCompleted: (_response, errors) =>
                          setCaptionsError((errors ?? []).length > 0),
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
                    <Anchor
                      component={Link}
                      to="/admin/recording"
                      hash={captionModelsAnchor}
                      size="sm"
                    >
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
          {error !== null && (
            <Alert color="red" role="alert">
              <Group justify="space-between" gap="xs">
                <Text size="sm">{error}</Text>
                <Button size="compact-sm" variant="light" color="red" onClick={retry}>
                  {t('player.retry')}
                </Button>
              </Group>
            </Alert>
          )}
        </>
      }
      side={
        <WatchPanel
          label={t('player.sidePanel')}
          tabs={[
            // 実況の過去ログがあれば、右のカラムで字幕の履歴と切り替えて出す。
            ...(jikkyo.comments !== null && jikkyo.comments.length > 0
              ? [
                  {
                    value: 'jikkyo',
                    label: t('jikkyo.toggle'),
                    content: (
                      <JikkyoRows
                        comments={jikkyo.comments}
                        count={listedCount}
                        offset={jikkyo.offset}
                        recordedFrom={recordedFrom}
                        frozen={shell.fullscreen.active}
                        onMuteUser={muteUser}
                      />
                    ),
                  },
                ]
              : []),
            {
              value: 'captions',
              label: t('captionHistory.tab'),
              content: (
                <CaptionHistory
                  entries={captionEntries}
                  label={(entry) => {
                    const at = clockAt(entry.at);

                    return at === null ? formatDuration(entry.at) : formatClock(at);
                  }}
                  onSelect={(entry) => seek(entry.at)}
                  canRegisterDRCS={drcs.canRegister}
                  height={watchCommentsHeight}
                />
              ),
            },
            {
              value: 'info',
              label: t('player.streamInfo.tab'),
              content: (
                <StreamInfo
                  meter={streamMeter.meter}
                  media={media}
                  quality={quality}
                  kind={route.kind === 'convert' || route.kind === 'kyusu' ? route.kind : null}
                  audios={audios}
                  transport={
                    file.quality == null
                      ? null
                      : {
                          dropped: Number(file.quality.drops),
                          errors: Number(file.quality.errors),
                          scrambled: Number(file.quality.scrambled),
                        }
                  }
                  average={null}
                  height={watchCommentsHeight}
                />
              ),
            },
          ]}
        />
      }
    />
  );
}

// JikkyoRows は、実況の過去ログのうち、再生位置までに流れた count 件の一覧 (#489)。時刻は、録り始め (recordedFrom) からの放送時刻で出す。
// 再生位置は 1 秒に何度も変わるので、流れたコメントの数が変わったときだけ作り直されるよう、部品に分けて React Compiler にメモ化させる。
function JikkyoRows({
  comments,
  count,
  offset,
  recordedFrom,
  frozen,
  onMuteUser,
}: {
  comments: readonly JikkyoComment[];
  count: number;
  offset: number;
  recordedFrom: string | null;
  frozen: boolean;
  onMuteUser: (userId: string) => void;
}) {
  const rows = listedUntil(comments, count).map(({ index, comment }) => {
    const seconds = comment.time + offset;

    return {
      key: index,
      label:
        recordedFrom == null
          ? formatDuration(seconds)
          : formatClock(broadcastTime(recordedFrom, noGaps, seconds)),
      content: comment.content,
      ...(comment.userId === undefined ? {} : { userId: comment.userId }),
    };
  });

  return (
    <CommentList rows={rows} frozen={frozen} onMuteUser={onMuteUser} height={watchCommentsHeight} />
  );
}
