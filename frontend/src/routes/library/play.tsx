import type { PlayerState } from '@mpeg2toh264/player';
import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Captions, Keyboard, MessageSquareText, Share2 } from 'lucide-react';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useMergedRef } from '@mantine/hooks';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useMutation, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../auth/returnTo';
import {
  ErrorBoundary,
  PageLoader,
  ProgramTitle,
  formatBytes,
  useDocumentTitle,
} from '../../futon';
import { useRecordedJikkyo } from '../../jikkyo/useRecordedJikkyo';
import { CommentList } from '../../jikkyo/CommentList';
import type { JikkyoComment } from '../../jikkyo/comments';
import { countUntil, listedUntil } from '../../jikkyo/commentList';
import { CommentOffsetSetting } from '../../jikkyo/CommentOffsetSetting';
import { findPartMarkers } from '../../jikkyo/partMarkers';
import { formatCommentOffset, useCommentOffsetCommands } from '../../jikkyo/commentOffset';
import { isSurroundAudio, useVirtualSurround } from '../../yunomi/useVirtualSurround';
import { useVisible } from '../../yunomi/useVisible';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { WatchPanel } from '../../yunomi/WatchPanel';
import {
  WatchDescription,
  WatchHeading,
  WatchLayout,
  watchCommentsHeight,
} from '../../yunomi/WatchLayout';
import { useVolumeNormalizer } from '../../yunomi/volumeNormalize';
import { useAudioDelayCommands, useAudioDelayer } from '../../yunomi/audioDelay';
import { useSleepTimer } from '../../yunomi/sleepTimer';
import { noCrop, type Crop } from '../../yunomi/crop';
import { CaptionHistory } from '../../yunomi/CaptionHistory';
import { StreamInfo, useStreamMeter } from '../../yunomi/StreamInfo';
import { useDRCSReplacements } from '../../yunomi/useDRCSReplacements';
import {
  insertRecordedCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../yunomi/captionHistory';
import { PipHost } from '../../yunomi/PipHost';
import { PipPlaceholder } from '../../yunomi/PipPlaceholder';
import { broadcastTime, formatClock, formatDuration } from '../../yunomi/position';
import { useSeekableStream } from '../../yunomi/useSeekableStream';
import { useFastForward } from '../../yunomi/fastForward';
import {
  RateSelect,
  CropSelect,
  AudioDelaySetting,
  NormalizeVolumeToggle,
  SleepTimerSelect,
  PlaybackUnavailable,
  QualitySelect,
  VirtualSurroundToggle,
} from '../../yunomi/QualitySelect';
import { hlsURL, toVideoFormat } from '../../yunomi/route';
import { HlsPlayer } from '../../yunomi/HlsPlayer';
import { useHlsStream } from '../../yunomi/useHlsStream';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { documentTitle, type PlayerSnapshot } from '../../yunomi/playerState';
import { SeekBar } from '../../yunomi/SeekBar';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useMediaSession } from '../../yunomi/useMediaSession';
import { ShortcutsHelp } from '../../yunomi/ShortcutsHelp';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import { usePlayerContextMenu } from '../../yunomi/PlayerContextMenu';
import { usePlayerGamepad } from '../../yunomi/gamepad';
import { usePlayerMouse } from '../../yunomi/mouseActions';
import type { PlayerCommands } from '../../yunomi/commands';
import {
  CaptureButton,
  openPictureInPicture,
  PipButton,
  useCapture,
} from '../../yunomi/PlayerTools';
import {
  PlayerAlert,
  BigPlayButton,
  PlayerLoading,
  CommandButton,
  FullscreenButton,
  PlayButton,
  PlayerControls,
  PositionStatus,
  SeekStepButtons,
  playerControlsClasses,
  SettingsButton,
  VolumeControl,
  PlayerFlash,
  SkipButton,
  usePlayerFlash,
} from '../../yunomi/PlayerControls';
import { loadCmAutoSkip, useCmAutoSkip } from '../../yunomi/cmAutoSkip';
import { CmAutoSkipper } from '../../yunomi/cmAutoSkipper';
import { cmButtonSectionAt, cmMergeGap, cmSectionAt, mergeCmSections } from '../../yunomi/cmSkip';
import type { PlayerCover } from '../../yunomi/playerCover';
import { useVideoFrames } from '../../yunomi/useVideoFrames';
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
import type { playGenerateLibraryCaptionsMutation } from './__generated__/playGenerateLibraryCaptionsMutation.graphql';
import { onVideoSurface, useControlsVisibility } from '../../yunomi/useControlsVisibility';
import { useDoubleTap } from '../../yunomi/useDoubleTap';
import { stepBrightness } from '../../yunomi/brightness';
import { swipeVolumeStep } from '../../yunomi/swipe';
import { useSwipeControlsSetting, volumeChangeable } from '../../yunomi/swipeControls';
import { useSwipeGestures } from '../../yunomi/useSwipeGestures';
import { ScrubBubble } from '../../yunomi/ScrubBubble';
import { useFullscreen } from '../../yunomi/useFullscreen';
import { useSlideSeek } from '../../yunomi/useSlideSeek';
import { useScreenCrop } from '../../yunomi/useScreenCrop';
import { useAutoplayBlocked } from '../../yunomi/useAutoplayBlocked';
import { useAutoPictureInPicture } from '../../yunomi/autoPictureInPicture';
import { useVideoLoading } from '../../yunomi/useVideoLoading';
import { useVideoState } from '../../yunomi/useVideoState';
import { browserStorage, stepVolume } from '../../yunomi/volume';
import { currentVolume } from '../../yunomi/volumeBoost';
import { useWakeLock } from '../../yunomi/useWakeLock';
import { useDocumentPictureInPicture } from '../../yunomi/useDocumentPictureInPicture';
import { useStreamsDrains } from '../../changes/DataChanges';
import {
  canRetryImmediately,
  endedEarly,
  playableStart,
  skipBrokenMedia,
} from '../../yunomi/reconnect';
import { copyPositionUrl, parseStartSeconds } from '../../yunomi/shareUrl';
import { useLibraryPositionSaver } from '../../yunomi/useWatchPositionSaver';
import { useHostedWatch, useWatchMedia, useWatchMini } from '../../yunomi/WatchHost';
import {
  clampSeek,
  fileDuration,
  libraryResumePosition,
  parentPath,
  seekLimit,
} from '../../library/library';
import type { playLibraryFileQuery } from './__generated__/playLibraryFileQuery.graphql';
import type { PlayerMedia } from '../../yunomi/playerMedia';

// volumeStep は、↑ と ↓ で音量を上げ下げする幅。
const volumeStep = 0.05;

// noGaps は、ライブラリのファイルの途切れた区間。ファイル 1 本を流すので、録画のセグメントの間のような区間はない。
const noGaps: readonly { from: string; to: string }[] = [];

// PlaySearch は、再生の画面の URL のクエリ。library と path は再生するファイル、t は再生を始める位置 (ファイルの始まりからの秒)。
type PlaySearch = { library: string; path: string; t?: number };

export const Route = createFileRoute('/library/play')({
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
  query playLibraryFileQuery($library: String!, $path: String!) {
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
  } = useLazyLoadQuery<playLibraryFileQuery>(playQuery, variables);
  const mediaRef = useRef<PlayerMedia | null>(null);
  const pip = useDocumentPictureInPicture();
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  const [media, setMedia] = useState<PlayerMedia | null>(null);
  // 小さなプレイヤーは、この経路で再生中かを確認し、再生と一時停止を操作する。
  useWatchMedia(media);
  const videoState = useVideoState(media);
  const autoplay = useAutoplayBlocked(media);
  const videoLoading = useVideoLoading(media);
  const fullscreen = useFullscreen(playerBoxRef);
  const overlaid = fullscreen.active || pip.container !== null;
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const controls = useControlsVisibility({ playing: !videoState.paused && !shortcutsOpen });
  useWakeLock(!videoState.paused);
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
  const [generateCaptions, generatingCaptions] = useMutation<playGenerateLibraryCaptionsMutation>(
    graphql`
      mutation playGenerateLibraryCaptionsMutation($id: ID!) {
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
      fetchQuery<playLibraryFileQuery>(environment, playQuery, variables, {
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
  // stream は、ファイルの始まりからの秒で、位置を指定して読み直す配信 (設計書 7.2)。kyusu の経路では、仮想 HLS を流す。
  // 再生できない経路 (8K や kyusu のない UHD) では、配信に HEAD を送らない。
  const stream = useSeekableStream(
    file.streamUrl,
    media,
    initialPosition,
    route.kind === 'convert',
  );
  const hlsStream = useHlsStream(media, hls, initialPosition);
  const { position, positionRef, currentPosition } = hls ? hlsStream : stream;
  const [seeking, setSeeking] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const lastAutoRetry = useRef<number | null>(null);
  // lastBrokenSkip は、壊れた所を最後に飛び越えた先の位置。
  const lastBrokenSkip = useRef<number | null>(null);
  // handover は、次の読み直しで、今の映像を流したまま裏で読み込んで切り替えるかどうか (#1085)。
  const [handover, setHandover] = useState(false);
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
  // 画質を変えると video の要素を作り直すので、見えているかは、変わらない外側の要素で確認する。
  const frameRef = useRef<HTMLDivElement | null>(null);
  const visible = useVisible(frameRef) || pip.container !== null;
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
  useVolumeNormalizer(media);
  useAudioDelayer(media);
  // 仮想サラウンド (#454)。5.1ch の番組を、ブラウザの中での変換で見ているときだけ使える。
  const audios = file.audios.length === 0 ? null : file.audios;
  const surroundAvailable = route.kind === 'convert' && isSurroundAudio(audios);
  useVirtualSurround(media, surroundAvailable);
  const [crop, setCrop] = useState<Crop>(noCrop);
  const shownCrop = useScreenCrop(crop, fullscreen.active);
  // restartAt は、指定の位置から読み直す。
  const restartAt = (seconds: number) => {
    setHandover(false);
    if (hls) {
      hlsStream.restartAt(seconds);

      return;
    }
    stream.restartAt(seconds);
  };
  // tenban が止まる前に通知してきたら、今の位置から、新しい接続を張ってから切り替える (#1085)。
  const drains = useStreamsDrains();
  const seenDrains = useRef(drains);
  useEffect(() => {
    if (drains === seenDrains.current) {
      return;
    }
    seenDrains.current = drains;
    if (route.kind === 'convert' && error === null) {
      stream.restartAt(positionRef.current, true);
      setHandover(true);
    }
    // 読み直しの位置は ref から読むので、通知が届いたときだけ動かす。
  }, [drains, error, hls]);
  const retry = () => {
    setError(null);
    restartAt(positionRef.current);
  };
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
  // lastSeek は、利用者が最後にシークした位置 (#1282)。そこが CM の区間の中なら、自動では飛ばさない。
  const lastSeek = useRef<number | null>(start === undefined ? null : initialPosition);
  // jumpTo は、指定の位置に移り、実際に移った先を返す。シークできる所 (インデックスのできた所) より先へは移らない。
  // 自動で CM を飛ぶときにも使うので、利用者のシークとしては数えない。
  // kyusu の仮想 HLS は、プレイリストの中を移るだけで、読み直さない。
  const jumpTo = (target: number) => {
    const seconds = clampSeek(target, file.index);
    const current = mediaRef.current;
    if (hls && current !== null) {
      current.seek(seconds);
      hlsStream.seekTo(seconds);

      return seconds;
    }
    restartAt(seconds);

    return seconds;
  };
  // seek は、利用者の操作で、指定の位置に移る。
  const seek = (target: number) => {
    lastSeek.current = jumpTo(target);
  };
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

  const togglePlay = () => {
    const current = mediaRef.current;
    if (current === null) {
      return;
    }
    if (current.paused) {
      void current.play().catch(() => {});
    } else {
      current.pause();
    }
  };
  const [flash, showFlash] = usePlayerFlash();
  const shownFlash = fastForward.active
    ? t('player.flash.fastForward', { rate: fastForward.playbackRate })
    : flash;
  // CM を自動で飛ばす (#1512)。録画と同じく、CM のコマを画面に出さないよう、飛ぶ直前に蓋を出し、飛び先が映ってから外す。
  const [cmAutoSkip] = useCmAutoSkip();
  // coverRef は、映像の上に重ねる蓋。プレイヤーが出し入れの口を入れる。
  const coverRef = useRef<PlayerCover | null>(null);
  // live は、CM を自動で飛ばす判定が読む、描画ごとに変わる値。判定は描画の外 (コマごと) で行う。
  const live = useRef({ cmAutoSkip, cmSpans, scrubbing: seeking !== null, jumpTo, showFlash, t });
  live.current = { cmAutoSkip, cmSpans, scrubbing: seeking !== null, jumpTo, showFlash, t };
  const [skipper] = useState(
    () =>
      new CmAutoSkipper({
        enabled: () => live.current.cmAutoSkip,
        scrubbing: () => live.current.scrubbing,
        paused: () => mediaRef.current?.paused ?? true,
        lastSeek: () => lastSeek.current,
        sections: () => live.current.cmSpans,
        cover: (kind) => coverRef.current?.show(kind),
        uncover: () => coverRef.current?.hide(),
        jump: (target) => live.current.jumpTo(target),
        notify: () => live.current.showFlash(live.current.t('player.flash.cmSkipped')),
        setTimer: (callback, ms) => {
          const timer = setTimeout(callback, ms);

          return () => clearTimeout(timer);
        },
      }),
  );
  useEffect(() => () => skipper.dispose(), [skipper]);
  // 判定は、映ったコマごとに、そのコマの時刻で行う。読み直していて位置を信じられない間は、位置を渡さない。
  useVideoFrames(media, ({ mediaTime, presented }) => {
    if (media === null) {
      return;
    }
    const time = mediaTime ?? media.currentTime;
    skipper.frame({
      position: hls ? hlsStream.framePosition(time) : stream.framePosition(time),
      seeking: media.seeking,
      paused: media.paused,
      presented,
    });
  });
  // 開いた位置が CM の中で、最初から CM の終わりを求めたときは、蓋で覆い、CM の終わり以上のコマが映るまで待つ。
  // 続きから開いたときは、続きから始めたことだけを伝え、CM を飛ばした通知は出さない。
  const openingHeld = useRef(false);
  useEffect(() => {
    if (!opening.skipped || openingHeld.current || media === null) {
      return;
    }
    openingHeld.current = true;
    skipper.holdUntil(opening.at);
    if (!opening.resumed) {
      showFlash(t('player.flash.cmSkipped'));
    }
  }, [media]);
  // 自動で飛ばす設定を入れた瞬間に CM の中にいれば、その場で飛ぶ。設定を入れたことを、飛ばしたい意思として扱う (#1512)。
  const previousCmAutoSkip = useRef(cmAutoSkip);
  useEffect(() => {
    if (cmAutoSkip && !previousCmAutoSkip.current) {
      lastSeek.current = null;
    }
    previousCmAutoSkip.current = cmAutoSkip;
  }, [cmAutoSkip]);
  // skip は、今の位置から seconds 秒移る。録画と同じく、先へ移った先が CM の中なら、自動で飛ばす設定のときは、
  // 蓋を出して続いた CM の終わりまで 1 回で飛ぶ。自分でシークして見ていた CM の中は飛ばさない (#1512)。
  const skip = (seconds: number) => {
    const target = currentPosition() + seconds;
    const watching = lastSeek.current;
    const span = cmSectionAt(cmSpans, target);
    const insideWatched =
      span !== null && watching !== null && span.from <= watching && watching < span.to;
    if (seconds > 0 && !insideWatched && skipper.skipFrom(target)) {
      return;
    }
    seek(target);
  };
  // キャプチャ (#908)。放送時刻は、TS の TOT から読んだ録り始めから数える。分からなければ、撮った時刻にする。
  const capture = useCapture({
    crop,
    frameRef: playerBoxRef,
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
  const doubleTap = useDoubleTap((side) => {
    if (side === 'center') {
      return;
    }
    skip(side === 'left' ? -10 : 10);
    showFlash(t(side === 'left' ? 'player.flash.seekBackward' : 'player.flash.seekForward'));
  });
  const changeVolume = (delta: number) => {
    const current = mediaRef.current;
    if (current === null) {
      return;
    }
    const next = stepVolume(currentVolume(current), delta);
    videoState.setVolume(next);
    showFlash(t('player.flash.volume', { percent: Math.round(next * 100) }));
  };
  const [autoPictureInPicture] = useAutoPictureInPicture();
  const audioDelayCommands = useAudioDelayCommands((ms) =>
    showFlash(t('player.flash.audioDelay', { ms })),
  );
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
    ...audioDelayCommands,
    ...(channelId === null || !jikkyoEnabled ? {} : commentOffsetCommands),
    ...(channelId === null ? {} : { toggleJikkyo: () => setJikkyoEnabled(!jikkyoEnabled) }),
    capture: () => void saveCapture(),
    play: () => void mediaRef.current?.play().catch(() => {}),
    pause: () => mediaRef.current?.pause(),
    togglePlay,
    seekBackward: () => {
      skip(-10);
      showFlash(t('player.flash.seekBackward'));
    },
    seekForward: () => {
      skip(10);
      showFlash(t('player.flash.seekForward'));
    },
    seekBackwardLong: () => {
      skip(-30);
      showFlash(t('player.flash.seekBackwardLong'));
    },
    seekForwardLong: () => {
      skip(30);
      showFlash(t('player.flash.seekForwardLong'));
    },
    seekBackward15: () => {
      skip(-15);
      showFlash(t('player.flash.seekBackward15'));
    },
    seekForward15: () => {
      skip(15);
      showFlash(t('player.flash.seekForward15'));
    },
    toggleCaptions: () => setCaptions(!captions),
    toggleMute: () => {
      const current = mediaRef.current;
      if (current === null) {
        return;
      }
      videoState.toggleMute();
      showFlash(t(current.muted ? 'player.flash.muted' : 'player.flash.unmuted'));
    },
    toggleFullscreen: fullscreen.toggle,
    volumeUp: () => changeVolume(volumeStep),
    volumeDown: () => changeVolume(-volumeStep),
    copyPositionUrl: () => {
      copyPositionUrl(currentPosition(), playerBoxRef.current).then(
        () => showFlash(t('player.flash.urlCopied')),
        () => showFlash(t('player.flash.urlCopyFailed')),
      );
    },
    restart: () => {
      retry();
      showFlash(t('player.flash.restarted'));
    },
    seekTo: seek,
    showShortcuts: () => setShortcutsOpen(true),
    setVolume: videoState.setVolume,
    // スリープタイマーと「再生をやめる」のキーは、プレイヤーを閉じて、ファイルのあったフォルダへ戻る (#1426)。
    stop: backToFolder,
    ...(pip.supported
      ? {
          pictureInPicture: () => {
            if (pip.container === null) {
              openPictureInPicture(pip).catch(() => {});
            }
          },
        }
      : {}),
  };
  const sleep = useSleepTimer(() => commands.stop?.());
  usePlayerShortcuts(commands);
  const contextMenu = usePlayerContextMenu(commands);
  usePlayerGamepad(commands);
  const mouseRef = usePlayerMouse(commands);
  const [swipeControls] = useSwipeControlsSetting();
  const [volumeAvailable] = useState(volumeChangeable);
  const [brightness, setBrightness] = useState(1);
  const brightnessRef = useRef(brightness);
  brightnessRef.current = brightness;
  useSwipeGestures(playerBoxRef, {
    enabled: swipeControls && fullscreen.active,
    controlsVisible: controls.visible,
    volumeAvailable,
    onSwipeStart: controls.hide,
    onStep: (band, steps) => {
      if (band === 'volume') {
        changeVolume(steps * swipeVolumeStep);

        return;
      }
      const next = stepBrightness(brightnessRef.current, steps);
      brightnessRef.current = next;
      setBrightness(next);
      showFlash(t('player.flash.brightness', { percent: Math.round(next * 100) }));
    },
  });
  const seekLabel = (value: number) => {
    const at = clockAt(value);

    return at === null ? formatDuration(value) : `${formatDuration(value)} (${formatClock(at)})`;
  };
  const scrub = useSlideSeek(playerBoxRef, {
    enabled: limit > 0,
    range: () => ({ position: currentPosition(), duration }),
    onSlideStart: controls.hide,
    onScrub: (value) => setSeeking(value === null ? null : Math.min(value, limit)),
    onCommit: (target) => {
      setSeeking(null);
      seek(target);
    },
    onCancel: () => setSeeking(null),
    onLongPress: contextMenu.openAt,
  });
  const boxRef = useMergedRef(playerBoxRef, mouseRef);
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
  const watchMini = useWatchMini();
  useDocumentTitle(documentTitle(player), !watchMini);
  useMediaSession(player, commands, autoPictureInPicture);

  // resumeAfterCut は、配信が切れたら、通知を出さずに今の位置から読み直す (#1083)。続けて切れるなら、エラーを出して利用者に任せる。
  const resumeAfterCut = () => {
    if (mediaRef.current?.error != null) {
      const next = skipBrokenMedia(positionRef.current, lastBrokenSkip.current);
      if (next !== null) {
        lastBrokenSkip.current = next;
        restartAt(clampSeek(next, file.index));

        return;
      }
    }
    const now = Date.now();
    if (!canRetryImmediately(lastAutoRetry.current, now)) {
      setError(t('player.error'));

      return;
    }
    lastAutoRetry.current = now;
    restartAt(positionRef.current);
  };
  // onConvertState は、配信がファイルの終わりに届く前に終わったら、途中で切れたとみなして読み直す。
  // インデックスを作っている途中は、長さが見込みなので、終わりかどうかを決められない。読み直しを繰り返さないよう、見分けない。
  const onConvertState = (state: PlayerState) => {
    const current = mediaRef.current;
    if (state !== 'completed' || current === null || indexing) {
      return;
    }
    const { buffered } = current;
    const end =
      buffered.length === 0
        ? positionRef.current
        : stream.positionOf(buffered.end(buffered.length - 1));
    if (endedEarly(end, duration)) {
      resumeAfterCut();
    }
  };
  const errorAlert = overlaid && error !== null && (
    <PlayerAlert title={error} action={{ label: t('player.retry'), onClick: retry }} />
  );
  const shown = seeking ?? position;
  const shownClock = clockAt(shown);
  // cmBands は、CM とみなした区間 (設計書 5.8)。
  const cmBands = cmPositions.map(({ from, to }) => ({
    from,
    to,
    label: t('player.cmSection', { duration: formatDuration(Math.max(to - from, 0)) }),
    kind: 'cm' as const,
  }));
  // bands は、CM の区間と、インデックスをまだ作っている所 (シークできないので帯で示す) と、実況のコメントから見つけた番組の区切りのマーク。
  const bands = [
    ...cmBands,
    ...(indexing && duration > limit
      ? [{ from: limit, to: duration, label: t('library.indexing') }]
      : []),
    ...partBands,
  ];
  // currentCm は、いま再生している CM の区間 (#1300)。あれば、区間の終わりへ飛ばすボタンを出す。
  const currentCm = seeking === null ? cmButtonSectionAt(cmBands, position) : null;
  const skipButton = currentCm !== null && (
    <SkipButton
      raised={controls.visible}
      // CM から出るための操作なので、CM を見たい意思 (lastSeek) を消す (#1512)。自動で飛ばす設定なら、続いた CM の終わりまで、
      // 蓋を出して 1 回で飛ぶ。
      onClick={() => {
        lastSeek.current = null;
        if (!skipper.skipFrom(currentPosition())) {
          jumpTo(currentCm.to);
        }
      }}
    >
      {t('player.skipCm')}
    </SkipButton>
  );

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
            <CropSelect value={crop} onChange={setCrop} />
            <NormalizeVolumeToggle />
            <AudioDelaySetting />
            {channelId !== null && jikkyoEnabled && <CommentOffsetSetting />}
            <SleepTimerSelect timer={sleep} />
            {surroundAvailable && <VirtualSurroundToggle />}
          </SettingsButton>
          {/* 幅 375px ほどのスマートフォンでは、15 秒のシークのボタンと再生位置を収めるため、キャプチャのボタンを外す。 */}
          <CaptureButton capture={capture} optional="compact" />
          <CommandButton
            commands={commands}
            command="showShortcuts"
            label={t('player.shortcuts.title')}
            icon={Keyboard}
            optional
          />
          <ShortcutsHelp
            opened={shortcutsOpen}
            onClose={() => setShortcutsOpen(false)}
            commands={commands}
            target={playerBoxRef}
          />
          <PipButton pip={pip} />
          <FullscreenButton active={fullscreen.active} commands={commands} />
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
      {shownFlash !== null && <PlayerFlash>{shownFlash}</PlayerFlash>}
      {scrub !== null && (
        <ScrubBubble seconds={scrub} label={seekLabel(scrub)} previewUrl={file.previewUrl} />
      )}
      {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
      {videoLoading && !autoplay.blocked && error === null && <PlayerLoading />}
      {skipButton}
      {errorAlert}
    </>
  );

  const playerBox = (
    <div
      ref={boxRef}
      onContextMenu={contextMenu.onContextMenu}
      className={playerControlsClasses.box}
      data-controls-visible={controls.visible}
      {...controls.containerProps}
      // マウスで映像を押すと、再生と一時停止を切り替える。タッチでは、操作バーを出し隠しするので切り替えない。
      onClick={(event) => {
        const pointer =
          event.nativeEvent instanceof PointerEvent ? event.nativeEvent.pointerType : 'mouse';
        if (pointer === 'mouse' && onVideoSurface(event.target)) {
          togglePlay();
        }
      }}
      onDoubleClick={(event) => {
        if (onVideoSurface(event.target)) {
          fullscreen.toggle();
        }
      }}
      onPointerUp={doubleTap}
    >
      {contextMenu.menu}
      {route.kind === 'convert' && (
        <TsPlayer
          url={stream.url}
          handover={handover}
          mediaRef={mediaRef}
          onMedia={setMedia}
          controls={controlBar}
          captions={captions && !autoCaptionsShown}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shownCrop}
          brightness={brightness}
          onCaptionText={onCaptionText}
          onLoaded={(handed) => {
            stream.onLoaded(handed);
            setError(null);
          }}
          handoverTarget={stream.handoverTarget}
          onError={resumeAfterCut}
          onState={onConvertState}
          onAudio={audio.onAudio}
          onReading={streamMeter.onReading}
          audio={audio.playerAudio}
          overlay={overlay}
          coverRef={coverRef}
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
          crop={shownCrop}
          brightness={brightness}
          onCaptionText={onCaptionText}
          live={false}
          startPosition={hlsStream.start}
          overlay={overlay}
          onLoaded={() => setError(null)}
          onError={resumeAfterCut}
          coverRef={coverRef}
        />
      )}
    </div>
  );

  return (
    <WatchLayout
      player={
        <Stack gap="xs">
          <PlaybackUnavailable route={route} />
          <div ref={frameRef}>
            <PipHost container={pip.container} placeholder={<PipPlaceholder onBack={pip.close} />}>
              {playerBox}
            </PipHost>
          </div>
        </Stack>
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
                        frozen={fullscreen.active}
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
