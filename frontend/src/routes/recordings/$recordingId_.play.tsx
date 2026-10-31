import type { PlayerState } from '@mpeg2toh264/player';
import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Captions, Keyboard, MessageSquareText } from 'lucide-react';
import { Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { ErrorBoundary, PageLoader, ProgramTitle, useDocumentTitle } from '../../futon';
import { useRecordedJikkyo } from '../../jikkyo/useRecordedJikkyo';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { CommentList } from '../../jikkyo/CommentList';
import { WatchPanel } from '../../yunomi/WatchPanel';
import { RuleRecordingList } from '../../recordings/RuleRecordingList';
import { WatchHeading, WatchLayout, watchCommentsHeight } from '../../yunomi/WatchLayout';
import { isSurroundAudio, useVirtualSurround } from '../../yunomi/useVirtualSurround';
import { useVolumeNormalizer } from '../../yunomi/volumeNormalize';
import { useAudioDelayCommands, useAudioDelayer } from '../../yunomi/audioDelay';
import { noCrop, type Crop } from '../../yunomi/crop';
import { CaptionHistory } from '../../yunomi/CaptionHistory';
import {
  insertRecordedCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../yunomi/captionHistory';
import { countUntil, listedUntil } from '../../jikkyo/commentList';
import { HlsPlayer } from '../../yunomi/HlsPlayer';
import { PipPlaceholder } from '../../yunomi/PipPlaceholder';
import {
  broadcastTime,
  recordingPosition,
  parseStreamStart,
  formatClock,
  formatDuration,
  playbackPosition,
  skipJump,
} from '../../yunomi/position';
import {
  RateSelect,
  PlaybackUnavailable,
  QualitySelect,
  CropSelect,
  AudioDelaySetting,
  NormalizeVolumeToggle,
  VirtualSurroundToggle,
} from '../../yunomi/QualitySelect';
import { hlsURL, toVideoFormat } from '../../yunomi/route';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { useRemotePlayer } from '../../yunomi/useRemotePlayer';
import { documentTitle, usePlayerEvents, type PlayerSnapshot } from '../../yunomi/playerState';
import { SeekBar } from '../../yunomi/SeekBar';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useWatchPositionSaver } from '../../yunomi/useWatchPositionSaver';
import { useMediaSession } from '../../yunomi/useMediaSession';
import { ShortcutsHelp } from '../../yunomi/ShortcutsHelp';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import type { PlayerCommands } from '../../yunomi/commands';
import { useAutoPictureInPicture } from '../../yunomi/autoPictureInPicture';
import {
  CaptureButton,
  openPictureInPicture,
  PipButton,
  useCapture,
} from '../../yunomi/PlayerTools';
import { PlayerStats, StatsToggle } from '../../yunomi/PlayerStats';
import { StreamInfo, useStreamMeter } from '../../yunomi/StreamInfo';
import { averageBitrate } from '../../yunomi/streamMeter';
import {
  PlayerAlert,
  BigPlayButton,
  PlayerLoading,
  CommandButton,
  FullscreenButton,
  PlayButton,
  PlayerControls,
  playerControlsClasses,
  SettingsButton,
  VolumeControl,
  PlayerFlash,
  usePlayerFlash,
} from '../../yunomi/PlayerControls';
import { onVideoSurface, useControlsVisibility } from '../../yunomi/useControlsVisibility';
import { useDoubleTap } from '../../yunomi/useDoubleTap';
import { useFullscreen } from '../../yunomi/useFullscreen';
import { useAutoplayBlocked } from '../../yunomi/useAutoplayBlocked';
import { useVideoLoading } from '../../yunomi/useVideoLoading';
import { useVideoState } from '../../yunomi/useVideoState';
import { stepVolume } from '../../yunomi/volume';
import { currentVolume } from '../../yunomi/volumeBoost';
import { useWakeLock } from '../../yunomi/useWakeLock';
import { useDocumentPictureInPicture } from '../../yunomi/useDocumentPictureInPicture';
import { useStreamsDrains } from '../../changes/DataChanges';
import { canRetryImmediately, endedEarly, skipBrokenMedia } from '../../yunomi/reconnect';
import { resumePosition } from '../../yunomi/resume';
import { useVisible } from '../../yunomi/useVisible';
import type { RecordingIdPlayQuery } from './__generated__/RecordingIdPlayQuery.graphql';

// volumeStep は、↑ と ↓ で音量を上げ下げする幅。
const volumeStep = 0.05;

// PlaySearch は、再生の画面の URL のクエリ。t は、再生を始める位置 (録画の始まりからの秒)。キャプチャの履歴から、撮った場面を開くのに使う (#457)。
type PlaySearch = { t?: number };

export const Route = createFileRoute('/recordings/$recordingId_/play')({
  // 右の列を置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => {
    const t = typeof search.t === 'number' ? search.t : Number(search.t);

    return Number.isFinite(t) && t >= 0 ? { t } : {};
  },
  component: PlayPage,
});

// PlayPage は、録画を再生する画面。ブラウザの中で変換するか、kyusu の HLS で再生する (設計書 7.1)。
function PlayPage() {
  const { t } = useTranslation();
  const { recordingId } = Route.useParams();
  const { t: start } = Route.useSearch();

  return (
    <Stack gap="sm">
      <Anchor
        size="sm"
        renderRoot={(props) => (
          <Link {...props} to="/recordings/$recordingId" params={{ recordingId }} />
        )}
      >
        {t('player.backToRecording')}
      </Anchor>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">
              {graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('recordings.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Player key={recordingId} recordingId={recordingId} start={start} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// playQuery は、再生する録画。録画中は、録画を終えた時刻 (recordedUntil) を知るために読み直す。
const playQuery = graphql`
  query RecordingIdPlayQuery($id: ID!) {
    node(id: $id) {
      __typename
      ... on Recording {
        name
        startAt
        endAt
        streamUrl
        videoFormat
        recordedFrom
        recordedUntil
        gaps {
          from
          to
        }
        jikkyoCommentsUrl
        rule {
          id
          name
        }
        watchPositionMs
        thumbnailUrl
        previewUrl
        audios {
          componentType
          languages
          samplingRate
        }
        files {
          kind
          size
        }
        drops
        errors
        scrambled
        service {
          name
          logoUrl
          jikkyo {
            id
            name
          }
        }
      }
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

// recordingRefreshMs は、録画中の録画を追っかけ再生している間に、録画を読み直す間隔 (#671)。
// 録画を終えた時刻が分かるまでは、録画の長さを今の時刻まで伸ばして扱うので、終えたら読み直して止める。
const recordingRefreshMs = 30_000;

function Player({ recordingId, start }: { recordingId: string; start: number | undefined }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<RecordingIdPlayQuery>(playQuery, { id: recordingId });
  const videoRef = useRef<HTMLVideoElement | null>(null);
  // 画質を変えると video の要素を作り直すので、見えているかは、変わらない外側の要素で調べる。
  const frameRef = useRef<HTMLDivElement | null>(null);
  const inPage = useVisible(frameRef);
  // pip は、視聴画面を出す別のウィンドウ。出している間は、このページが隠れても、実況のコメントなどを描き続ける (設計書 7.5)。
  // 別のウィンドウに移したり戻したりすると、プレイヤーを作り直すので、今の位置から読み直す。
  const pip = useDocumentPictureInPicture(() => startAt(positionRef.current));
  const visible = inPage || pip.container !== null;
  // playerBoxRef は、プレイヤーを包む要素。別のウィンドウに出すと移るので、キャプチャと全画面は、これを使う。
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  // video は、今の video の要素。画質を変えたときや、別のウィンドウに移したときに作り直される。
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoState = useVideoState(video);
  const navigate = useNavigate();
  // ブラウザに音ありの自動再生を断られたら、映像の真ん中に再生ボタンを出す。
  const autoplay = useAutoplayBlocked(video);
  const videoLoading = useVideoLoading(video);
  const fullscreen = useFullscreen(playerBoxRef);
  // overlaid は、プレイヤーだけを出していて、ページの知らせが見えないかどうか。そのときは、映像に重ねて出す (#435)。
  const overlaid = fullscreen.active || pip.container !== null;
  // shortcutsOpen は、キーボードショートカットの一覧を出しているかどうか。
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  // ショートカットの一覧を見ている間は、操作バーとカーソルを隠さない。
  const controls = useControlsVisibility({ playing: !videoState.paused && !shortcutsOpen });
  // 再生している間は、画面を消さない。
  useWakeLock(!videoState.paused);
  // BS4K と 8K の録画では、経路の選び方が変わる (#806)。
  const { route, choices, quality, setQuality } = usePlaybackRoute(
    data.transcoding,
    data.node?.__typename === 'Recording' ? toVideoFormat(data.node.videoFormat) : 'HD',
  );
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    setJikkyoEnabled,
    comments: commentSettings,
    muteUser,
    captions: captionSettings,
  } = usePlayerSettings();
  const recording = data.node?.__typename === 'Recording' ? data.node : null;
  // 録画中なら、録画を終えるまで読み直し、終えた時刻を受け取ったら、録画の長さを伸ばすのをやめる (#671)。
  const environment = useRelayEnvironment();
  const stillRecording = recording !== null && recording.recordedUntil == null;
  useEffect(() => {
    if (!stillRecording) {
      return;
    }
    const timer = setInterval(() => {
      fetchQuery<RecordingIdPlayQuery>(
        environment,
        playQuery,
        { id: recordingId },
        { fetchPolicy: 'network-only' },
      )
        .toPromise()
        .catch(() => {});
    }, recordingRefreshMs);

    return () => clearInterval(timer);
  }, [environment, stillRecording, recordingId]);
  // duration は、録画の長さ (秒)。ファイルには途切れた区間の分の映像がないので、その分を引く。
  const duration =
    recording === null
      ? 0
      : recordingPosition(
          recording.recordedFrom,
          recording.gaps,
          recording.recordedUntil == null ? Date.now() : Date.parse(recording.recordedUntil),
        );
  // resumeAt は、再生を始める位置 (秒)。URL で位置を渡されたらそこから、なければ前回の続きから、続きもなければ 0 (設計書 7.5)。
  // Int64 は Relay の型では any になるので、数に直す。
  const savedMs = recording?.watchPositionMs == null ? null : Number(recording.watchPositionMs);
  const [resumeAt] = useState(() =>
    start === undefined ? resumePosition(savedMs, duration) : Math.min(start, duration),
  );
  // kyusu の HLS は、録画の全体を 1 つのプレイリストで表すので、video の時刻がそのまま録画の中の位置になる。
  const hls = route.kind === 'kyusu';
  // offset は、今の読み込みを始めた、録画の始まりからの秒。ブラウザの中で変換するときは、シークすると、
  // 新しい開始位置で読み直す (設計書 7.2)。kyusu の HLS では、画質を変えたときに続きから読む位置に使う。
  const [offset, setOffset] = useState(resumeAt);
  // loads は、読み込みを始め直した回数。同じ位置へシークし直したときも、URL を変えて読み直させる。
  const [loads, setLoads] = useState(0);
  // startedAt は、ブラウザの中で変換するときに、配信が実際に流し始めた位置 (録画の始まりからの秒)。配信は、指定した位置の
  // 手前の I フレームから流すので、再生位置は、指定した位置 (offset) ではなく、ここから数える (#536)。分かるまでは offset を使う。
  const [streamStart, setStreamStart] = useState<{ key: string; at: number } | null>(null);
  const streamKey = `${offset}:${loads}`;
  const startedAt = !hls && streamStart?.key === streamKey ? streamStart.at : offset;
  const streamUrl = recording?.streamUrl ?? null;
  useEffect(() => {
    if (hls || streamUrl === null) {
      return;
    }
    // 配信は、HEAD でも、流し始める位置を返す。
    const controller = new AbortController();
    fetch(`${streamUrl}?start=${offset.toFixed(1)}`, {
      method: 'HEAD',
      credentials: 'same-origin',
      signal: controller.signal,
    })
      .then((response) => {
        const at = response.ok ? parseStreamStart(response.headers.get('X-Kotatsu-Start')) : null;
        if (at !== null) {
          setStreamStart({ key: `${offset}:${loads}`, at });
        }
      })
      .catch(() => {});

    return () => controller.abort();
  }, [hls, streamUrl, offset, loads]);
  // base は、読み込んだ映像の最初のフレームの、video の時刻。変換した映像の時刻は 0 から始まるとは限らない。
  const base = useRef(0);
  // lastVideoTime は、最後に見た video の時刻。変換した映像の時刻が、途切れた区間で飛んだのを見つけるのに使う。
  const lastVideoTime = useRef(0);
  // convertedPosition は、ブラウザの中で変換した映像の時刻 current から、再生位置を求める。途切れた区間で飛んだ分は数えない。
  const convertedPosition = (current: number) => {
    base.current = skipJump(base.current, lastVideoTime.current, current);
    lastVideoTime.current = current;

    return playbackPosition(startedAt, base.current, current);
  };
  // phase は、読み込みの段階。読み直している間に、前の映像の時刻で位置を動かさないよう、base を決めるまで位置を更新しない。
  const phase = useRef<'loading' | 'awaitingBase' | 'playing'>('loading');
  const [position, setPosition] = useState(resumeAt);
  // positionRef は、別のウィンドウへの移し替えや、画質と音声の切り替えで、読み直すときに続ける位置。
  const positionRef = useRef(resumeAt);
  const [seeking, setSeeking] = useState<number | null>(null);
  const [paused, setPaused] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // lastAutoRetry は、切れたときに、知らせを出さずに今の位置から読み直した時刻 (#1083)。
  const lastAutoRetry = useRef<number | null>(null);
  // lastBrokenSkip は、壊れた所を最後に飛び越えた先の位置。
  const lastBrokenSkip = useRef<number | null>(null);
  // handover は、次の読み直しで、今の映像を流したまま裏で読み込んで切り替えるかどうか (#1085)。
  // tenban が止まる前の知らせで読み直すときだけ使い、シークなどでは、すぐ読み直す。
  const [handover, setHandover] = useState(false);

  // currentPosition は、今の再生位置 (録画の始まりからの秒) を返す。実況のコメントを、画面の更新ごとに描くのに使う。
  // timeupdate で更新する position は間隔が粗いので、video の時刻から求め直す。
  const currentPosition = () => {
    const video = videoRef.current;
    if (video === null) {
      return positionRef.current;
    }
    // 読み直している間は、video の時刻がまだ読み始める位置になっていないので、前に求めた位置を使う。
    if (phase.current !== 'playing') {
      return positionRef.current;
    }
    if (hls) {
      return video.currentTime;
    }

    return convertedPosition(video.currentTime);
  };
  const channel = recording?.service.jikkyo ?? null;
  useWatchPositionSaver(recordingId, currentPosition, !paused);
  const jikkyo = useRecordedJikkyo(
    channel !== null && jikkyoEnabled && recording !== null ? recording.jikkyoCommentsUrl : null,
    currentPosition,
    visible,
    // 録画中の録画を追っかけ再生している間は、リアルタイムのコメントも流す (#667)。
    channel !== null && recording !== null && recording.recordedUntil == null
      ? { channelId: channel.id, recordedFrom: recording.recordedFrom, gaps: recording.gaps }
      : null,
    commentSettings,
  );
  // recordedRows は、コメントの一覧に出す、再生位置までに流れた過去ログ (#489)。再生位置は 1 秒に何度も変わるので、
  // 流れたコメントの数が変わったときだけ作り直し、時刻の文字は、コメントごとに一度だけ作る。
  const listedCount = jikkyo.comments === null ? 0 : countUntil(jikkyo.comments, position);
  const commentLabels = useMemo(() => new Map<number, string>(), [jikkyo.comments, recording]);
  const recordedRows = useMemo(() => {
    if (jikkyo.comments === null || recording === null) {
      return [];
    }

    return listedUntil(jikkyo.comments, listedCount).map(({ index, comment }) => {
      let label = commentLabels.get(index);
      if (label === undefined) {
        label = formatClock(broadcastTime(recording.recordedFrom, recording.gaps, comment.time));
        commentLabels.set(index, label);
      }

      return {
        key: index,
        label,
        content: comment.content,
        ...(comment.userId === undefined ? {} : { userId: comment.userId }),
      };
    });
  }, [jikkyo.comments, listedCount, recording, commentLabels]);
  // 実況の過去ログの読み込み具合は、実況のボタンのツールチップで示す。
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

  // statsShown は、再生の統計情報を映像の上に出しているかどうか (#499)。
  const [statsShown, setStatsShown] = useState(false);
  // streamMeter は、右の列の「情報」に出す、届いた映像のビットレートと放送の形式を溜める (#828)。
  const streamMeter = useStreamMeter(quality);
  // 音量の補正 (#499)。端末ごとの設定に合わせる。
  useVolumeNormalizer(video);
  useAudioDelayer(video);
  // 仮想サラウンド (#454)。5.1ch の番組を、ブラウザの中での変換で見ているときだけ使える。
  const surroundAvailable = route.kind === 'convert' && isSurroundAudio(recording?.audios ?? null);
  useVirtualSurround(video, surroundAvailable);
  // crop は、L 字放送の切り抜き (#496)。その場限りで、画面を開き直したら戻す。
  const [crop, setCrop] = useState<Crop>(noCrop);
  // startAt は、指定の位置から読み直す。
  const startAt = (seconds: number) => {
    setHandover(false);
    phase.current = 'loading';
    positionRef.current = seconds;
    setPosition(seconds);
    setOffset(seconds);
    setLoads((n) => n + 1);
  };
  // tenban が止まる前に知らせてきたら、ブラウザの中で変換するときは、今の位置から、新しい接続を張ってから切り替える (#1085)。
  // kyusu の HLS は、セグメントごとに読むので、つなぎ直さなくてよい。
  const drains = useStreamsDrains();
  const seenDrains = useRef(drains);
  useEffect(() => {
    if (drains === seenDrains.current) {
      return;
    }
    seenDrains.current = drains;
    // エラーで止まっている間は、「やり直す」に任せる。
    if (!hls && error === null) {
      startAt(positionRef.current);
      setHandover(true);
    }
    // startAt と positionRef は、描画ごとに作り直すが、読み直しの位置は ref から読むので、知らせが届いたときだけ動かす。
  }, [drains, hls, error]);
  // retry は、今の位置から読み直す。エラーの知らせも消し、読み込めたら出さないままにする。
  const retry = () => {
    setError(null);
    startAt(positionRef.current);
  };

  // seek は、指定の位置に移る。kyusu の HLS は、プレイリストの中を移るだけで、読み直さない。
  // captionEntries は、字幕の履歴 (#450)。字幕が出たときの再生位置で並べ、行を押すとその位置へ移る。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  // currentCaption は、今出ている字幕の文。キャプチャに書き込む (#458)。
  const currentCaption = useRef<string | null>(null);
  const onCaptionText = (text: string) => {
    const normalized = normalizeCaption(text);
    currentCaption.current = normalized === '' ? null : normalized;
    const at = currentPosition();
    setCaptionEntries((list) => insertRecordedCaption(list, at, text));
  };
  const seek = (target: number) => {
    // 録画の外へは移らない。OS のメディア操作からも、範囲の外の位置が届くことがある。
    const seconds = Math.min(Math.max(target, 0), duration);
    const video = videoRef.current;
    if (hls && video !== null) {
      video.currentTime = seconds;
      positionRef.current = seconds;
      setPosition(seconds);

      return;
    }
    startAt(seconds);
  };

  // changeQuality は、画質を変え、それまでの位置から続ける。
  const changeQuality = (next: string) => {
    startAt(positionRef.current);
    setQuality(next);
  };

  // 音声は、経路ごとのメニューで選び、画質を変えても同じ音声を保つ。kyusu の音声を変えるときも、それまでの位置から続ける。
  // kyusu の経路では、録画を始めたときの番組の音声の構成から選ぶ。
  const audio = usePlayerAudio(route, recording?.audios ?? null, () =>
    startAt(positionRef.current),
  );
  // 再生速度は、video の要素を作り直したときにも当て直す。
  const [rate, setRate] = useState(1);
  useEffect(() => {
    if (video !== null) {
      video.defaultPlaybackRate = rate;
      video.playbackRate = rate;
    }
  }, [rate, video, loads]);

  const togglePlay = () => {
    const video = videoRef.current;
    if (video === null) {
      return;
    }
    if (video.paused) {
      void video.play().catch(() => {});
    } else {
      video.pause();
    }
  };
  // skip は、今の位置から seconds 秒だけ進むか戻る。
  const skip = (seconds: number) => seek(currentPosition() + seconds);
  // flash は、キーで操作した結果。映像の上に短く出す (#448)。
  const [flash, showFlash] = usePlayerFlash();
  const capture = useCapture({
    crop,
    frameRef: playerBoxRef,
    videoRef,
    serviceName: recording?.service.name ?? '',
    programName: recording?.name ?? null,
    captureTime: () =>
      recording == null
        ? Date.now()
        : broadcastTime(recording.recordedFrom, recording.gaps, currentPosition()),
    recordingId,
    position: currentPosition,
    caption: () => currentCaption.current,
  });
  const doubleTap = useDoubleTap((side) => {
    if (side === 'center') {
      return;
    }
    skip(side === 'left' ? -10 : 10);
    showFlash(t(side === 'left' ? 'player.flash.seekBackward' : 'player.flash.seekForward'));
  });
  const saveCapture = async () => {
    if ((await capture.capture('save')) === 'saved') {
      showFlash(t('player.flash.captured'));
    }
  };
  // 押し続けると、描き直す前に次のキーが来るので、今の音量は video の要素から読む。映像がなければ何もしない。
  const changeVolume = (delta: number) => {
    const video = videoRef.current;
    if (video === null) {
      return;
    }
    const next = stepVolume(currentVolume(video), delta);
    videoState.setVolume(next);
    showFlash(t('player.flash.volume', { percent: Math.round(next * 100) }));
  };
  const [autoPictureInPicture] = useAutoPictureInPicture();
  // commands は、この画面で今使える操作 (#809)。キー、OS のメディア操作、ほかの端末からの操作は、どれもここから呼ぶ。
  // audioDelayCommands は、音ずれ補正 (#818) の操作。変えたら、長さを映像の上に短く出す。
  const audioDelayCommands = useAudioDelayCommands((ms) =>
    showFlash(t('player.flash.audioDelay', { ms })),
  );
  const commands: PlayerCommands = {
    ...audioDelayCommands,
    play: () => void videoRef.current?.play().catch(() => {}),
    pause: () => videoRef.current?.pause(),
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
    toggleCaptions: () => setCaptions(!captions),
    ...(channel === null ? {} : { toggleJikkyo: () => setJikkyoEnabled(!jikkyoEnabled) }),
    toggleMute: () => {
      const video = videoRef.current;
      if (video === null) {
        return;
      }
      videoState.toggleMute();
      showFlash(t(video.muted ? 'player.flash.muted' : 'player.flash.unmuted'));
    },
    toggleFullscreen: fullscreen.toggle,
    volumeUp: () => changeVolume(volumeStep),
    volumeDown: () => changeVolume(-volumeStep),
    capture: () => void saveCapture(),
    // r で、今の位置から読み直す。映像が止まったり乱れたりしたときに、ページを読み直さずに直す (#499)。
    restart: () => {
      retry();
      showFlash(t('player.flash.restarted'));
    },
    seekTo: seek,
    toggleStats: () => setStatsShown(!statsShown),
    showShortcuts: () => setShortcutsOpen(true),
    setVolume: videoState.setVolume,
    setMuted: (muted) => {
      if (muted !== videoState.muted) {
        videoState.toggleMute();
      }
    },
    selectAudio: audio.remote.select,
    stop: () => void navigate({ to: '/recordings/$recordingId', params: { recordingId } }),
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
  usePlayerShortcuts(commands);
  // player は、プレイヤーの今の状態 (#811)。タブの題名、OS のメディア操作、ほかの端末は、どれもこれを読む。
  const player = {
    kind: 'RECORDING',
    target: recordingId,
    serviceName: recording?.service.name ?? '',
    program: recording === null ? null : { id: recordingId, name: recording.name },
    artwork: recording?.thumbnailUrl ?? null,
    paused,
    position,
    duration,
    playbackRate: rate,
    volume: videoState.volume,
    muted: videoState.muted,
    audios: audio.remote.choices,
    audio: audio.remote.selected,
    dataBroadcast: false,
    keyGroups: [],
  } satisfies PlayerSnapshot;
  const playerEvents = usePlayerEvents(player);
  useDocumentTitle(documentTitle(player));
  useMediaSession(
    player,
    commands,
    // 設定で選んでいれば、タブを切り替えたときに、別のウィンドウに出す (#492)。
    autoPictureInPicture,
  );

  // 再生位置を、録画の始まりからの秒で追う。
  useEffect(() => {
    if (video === null) {
      return;
    }
    const update = () => {
      if (hls) {
        // kyusu の HLS は、読み始める位置へ移るまで video の時刻が 0 のことがあるので、再生を始めるまで位置を動かさない。
        if (phase.current !== 'playing') {
          return;
        }
        positionRef.current = video.currentTime;
        setPosition(video.currentTime);

        return;
      }
      if (phase.current === 'loading') {
        return;
      }
      if (phase.current === 'awaitingBase') {
        if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) {
          return;
        }
        base.current = video.currentTime;
        lastVideoTime.current = video.currentTime;
        phase.current = 'playing';
      }
      const next = convertedPosition(video.currentTime);
      positionRef.current = next;
      setPosition(next);
    };
    const onPause = () => setPaused(true);
    const onPlay = () => setPaused(false);
    // kyusu の HLS は、読み始める位置へ移り、映像が流れ始めたところから、video の時刻を録画の中の位置として使う。
    const onPlaying = () => {
      if (hls) {
        phase.current = 'playing';
      }
    };
    video.addEventListener('timeupdate', update);
    video.addEventListener('pause', onPause);
    video.addEventListener('play', onPlay);
    video.addEventListener('playing', onPlaying);
    setPaused(video.paused);

    return () => {
      video.removeEventListener('timeupdate', update);
      video.removeEventListener('pause', onPause);
      video.removeEventListener('play', onPlay);
      video.removeEventListener('playing', onPlaying);
    };
    // 画質を変えたときと、別のウィンドウに出したり戻したりしたときは、video の要素を作り直すので、付け直す。
  }, [startedAt, hls, video]);

  // 同じアカウントのほかの端末から、このプレイヤーを操作できるようにする (#459、設計書 7.7)。
  useRemotePlayer(recording === null || video === null ? null : player, playerEvents, commands);

  if (recording === null) {
    return <Alert color="amber">{t('recordings.notFound')}</Alert>;
  }
  // フラグメントはサーバーに送られないので、読み込みを始め直すためだけに使う。
  const url = `${recording.streamUrl}?start=${offset.toFixed(1)}#${loads}`;
  // resumeAfterCut は、配信が切れたら、知らせを出さずに今の位置から読み直す (#1083)。tenban の入れ替えや回線の瞬断なら、これで戻る。
  // 続けて切れるなら、読み直しを繰り返さずに、エラーを出して利用者に任せる。
  const resumeAfterCut = () => {
    // 放送の受信で壊れた音声のフレームなどで、映像の要素が復号に失敗して止まったなら、壊れた所を飛び越えて読み直す (#1131)。
    // 同じ位置から読み直すと、同じフレームでまた止まる。
    if (videoRef.current?.error != null) {
      const next = skipBrokenMedia(positionRef.current, lastBrokenSkip.current);
      if (next !== null) {
        lastBrokenSkip.current = next;
        startAt(Math.min(next, duration));

        return;
      }
    }
    const now = Date.now();
    if (!canRetryImmediately(lastAutoRetry.current, now)) {
      setError(t('player.error'));

      return;
    }
    lastAutoRetry.current = now;
    startAt(positionRef.current);
  };
  const onError = resumeAfterCut;
  // onConvertState は、ブラウザの中で変換する配信が、録画の終わりに届く前に終わったら、途中で切れたとみなして読み直す。
  const onConvertState = (state: PlayerState) => {
    const video = videoRef.current;
    if (state !== 'completed' || video === null) {
      return;
    }
    const { buffered } = video;
    const end =
      buffered.length === 0
        ? positionRef.current
        : playbackPosition(startedAt, base.current, buffered.end(buffered.length - 1));
    if (endedEarly(end, duration)) {
      resumeAfterCut();
    }
  };
  // errorAlert は、全画面と別のウィンドウで、再生のエラーを映像に重ねて出す (#435)。
  const errorAlert = overlaid && error !== null && (
    <PlayerAlert title={error} action={{ label: t('player.retry'), onClick: retry }} />
  );
  const shown = seeking ?? position;
  // gapMarks は、録画が途切れた位置の印 (#479)。途切れた区間はシークバーの長さに含めないので (#754)、長さのない印にし、
  // カーソルを乗せたら、途切れていた長さを出す。
  const gapMarks = (recording?.gaps ?? []).map((gap) => {
    const at = recordingPosition(recording!.recordedFrom, recording!.gaps, Date.parse(gap.from));
    const length = Math.max((Date.parse(gap.to) - Date.parse(gap.from)) / 1000, 0);

    return { from: at, to: at, label: t('player.gapMark', { duration: formatDuration(length) }) };
  });

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <SeekBar
          bands={gapMarks}
          duration={duration}
          value={shown}
          previewUrl={recording.previewUrl}
          label={(value) =>
            `${formatDuration(value)} (${formatClock(broadcastTime(recording.recordedFrom, recording.gaps, value))})`
          }
          onChange={setSeeking}
          onChangeEnd={(value) => {
            setSeeking(null);
            seek(value);
          }}
        />
      }
      start={
        <>
          <PlayButton video={videoState} commands={commands} />
          <VolumeControl video={videoState} commands={commands} />
        </>
      }
      status={`${formatDuration(shown)} / ${formatDuration(duration)} · ${formatClock(broadcastTime(recording.recordedFrom, recording.gaps, shown))}`}
      end={
        <>
          <CommandButton
            commands={commands}
            command="toggleCaptions"
            label={t('player.captions')}
            icon={Captions}
            active={captions}
          />
          <CommandButton
            commands={commands}
            command="toggleJikkyo"
            label={jikkyoLabel}
            icon={MessageSquareText}
            active={jikkyoEnabled}
          />
          <SettingsButton>
            <QualitySelect choices={choices} quality={quality} onChange={changeQuality} />
            {audio.menu}
            <RateSelect value={rate} onChange={setRate} />
            <CropSelect value={crop} onChange={setCrop} />
            <NormalizeVolumeToggle />
            <AudioDelaySetting />
            {surroundAvailable && <VirtualSurroundToggle />}
            <StatsToggle shown={statsShown} onChange={setStatsShown} />
          </SettingsButton>
          <CaptureButton capture={capture} />
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

  const playerBox = (
    <div
      ref={playerBoxRef}
      className={playerControlsClasses.box}
      data-controls-visible={controls.visible}
      {...controls.containerProps}
      // マウスで映像を押すと、再生と一時停止を切り替える。タッチでは、押すと操作バーを出し隠しするので切り替えない。
      // 操作バーや、映像に重ねたボタンを押したときは、そのボタンの操作だけにする。
      onClick={(event) => {
        const pointer =
          event.nativeEvent instanceof PointerEvent ? event.nativeEvent.pointerType : 'mouse';
        if (pointer === 'mouse' && onVideoSurface(event.target)) {
          togglePlay();
        }
      }}
      // マウスで映像をダブルクリックすると、全画面を切り替える (#452)。1 回目と 2 回目のクリックで、再生と一時停止は元に戻る。
      onDoubleClick={(event) => {
        if (onVideoSurface(event.target)) {
          fullscreen.toggle();
        }
      }}
      // タッチで映像の左右の 3 分の 1 をダブルタップすると、10 秒戻るか進む (#452)。
      onPointerUp={doubleTap}
    >
      {route.kind === 'convert' && (
        <TsPlayer
          url={url}
          handover={handover}
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={crop}
          onCaptionText={onCaptionText}
          onLoaded={() => {
            // 最初のフレームが出たときの時刻を、base にする。
            phase.current = 'awaitingBase';
            setError(null);
          }}
          onError={onError}
          onState={onConvertState}
          overlay={
            <>
              {jikkyo.overlay}
              {flash !== null && <PlayerFlash>{flash}</PlayerFlash>}
              {statsShown && <PlayerStats video={video} quality={quality} />}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
              {videoLoading && !autoplay.blocked && error === null && <PlayerLoading />}
              {errorAlert}
            </>
          }
          onAudio={audio.onAudio}
          onReading={streamMeter.onReading}
          audio={audio.convertAudio}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          // 読み直すたびに URL を変え、hls.js を作り直す。fragment は、サーバーへは送られない。
          url={`${hlsURL(recording.streamUrl, route.preset, audio.kyusuAudio)}#${loads}`}
          videoRef={videoRef}
          onVideo={setVideo}
          onSegment={streamMeter.onSegment}
          controls={controlBar}
          captions={captions}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={crop}
          onCaptionText={onCaptionText}
          live={false}
          startPosition={offset}
          overlay={
            <>
              {jikkyo.overlay}
              {flash !== null && <PlayerFlash>{flash}</PlayerFlash>}
              {statsShown && <PlayerStats video={video} quality={quality} />}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
              {videoLoading && !autoplay.blocked && error === null && <PlayerLoading />}
              {errorAlert}
            </>
          }
          onLoaded={() => setError(null)}
          onError={onError}
        />
      )}
    </div>
  );

  // 実況の過去ログがあれば、右の列で字幕の履歴と切り替えて出す。
  const commentsShown = jikkyo.comments !== null && jikkyo.comments.length > 0;

  return (
    <WatchLayout
      player={
        <Stack gap="xs">
          <PlaybackUnavailable route={route} />
          <div ref={frameRef}>
            {pip.container === null ? playerBox : <PipPlaceholder onBack={pip.close} />}
          </div>
          {pip.container !== null && createPortal(playerBox, pip.container)}
        </Stack>
      }
      info={
        <>
          <WatchHeading
            title={<ProgramTitle name={recording.name} />}
            serviceName={recording.service.name}
            serviceLogoUrl={recording.service.logoUrl}
            time={recording}
          />
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
        <>
          {/* 実況の過去ログのうち再生位置までに流れたもの (#489) と、字幕の履歴と再生の情報 (#828) を、見出しで切り替える。過去ログがなければ、コメントの一覧を出さない。 */}
          <WatchPanel
            label={t('player.sidePanel')}
            tabs={[
              ...(commentsShown
                ? [
                    {
                      value: 'jikkyo',
                      label: t('jikkyo.toggle'),
                      content: (
                        <CommentList
                          rows={recordedRows}
                          onMuteUser={muteUser}
                          height={watchCommentsHeight}
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
                    label={(entry) =>
                      formatClock(broadcastTime(recording.recordedFrom, recording.gaps, entry.at))
                    }
                    onSelect={(entry) => seek(entry.at)}
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
                    video={video}
                    quality={quality}
                    kind={route.kind === 'convert' || route.kind === 'kyusu' ? route.kind : null}
                    audios={recording?.audios ?? null}
                    average={recording === null ? null : averageBitrate(recording, Date.now())}
                    transport={
                      recording === null
                        ? null
                        : {
                            dropped: Number(recording.drops),
                            errors: Number(recording.errors),
                            scrambled: Number(recording.scrambled),
                          }
                    }
                    height={watchCommentsHeight}
                  />
                ),
              },
            ]}
          />
          {recording.rule != null && (
            // 一覧は読み込みを待たず、映像を先に出す。
            <Suspense fallback={null}>
              <RuleRecordingList
                ruleId={recording.rule.id}
                ruleName={recording.rule.name}
                recordingId={recordingId}
              />
            </Suspense>
          )}
        </>
      }
    />
  );
}
