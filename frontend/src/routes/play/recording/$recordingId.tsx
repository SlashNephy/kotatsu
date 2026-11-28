import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useListState } from '../../../recordings/listSearch';
import { Captions, MessageSquareText, Share2 } from 'lucide-react';
import { Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../../auth/returnTo';
import { ErrorBoundary, PageLoader, ProgramTitle } from '../../../futon';
import { useRecordedJikkyo } from '../../../jikkyo/useRecordedJikkyo';
import { graphQLErrorDetails, isUnauthenticated } from '../../../relay/environment';
import { CommentList } from '../../../jikkyo/CommentList';
import { WatchPanel } from '../../../yunomi/WatchPanel';
import { RuleRecordingList } from '../../../recordings/RuleRecordingList';
import {
  WatchDescription,
  WatchHeading,
  WatchLayout,
  watchCommentsHeight,
} from '../../../yunomi/WatchLayout';
import { isSurroundAudio, useVirtualSurround } from '../../../yunomi/useVirtualSurround';
import { CaptionHistory } from '../../../yunomi/CaptionHistory';
import { useDRCSReplacements } from '../../../yunomi/useDRCSReplacements';
import {
  insertRecordedCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../../yunomi/captionHistory';
import { countUntil, listedUntil } from '../../../jikkyo/commentList';
import { CommentOffsetSetting } from '../../../jikkyo/CommentOffsetSetting';
import { findPartMarkers } from '../../../jikkyo/partMarkers';
import { formatCommentOffset, useCommentOffsetCommands } from '../../../jikkyo/commentOffset';
import { HlsPlayer } from '../../../yunomi/HlsPlayer';
import {
  AutoCaptionOverlay,
  CaptionTrackSelect,
  type CaptionTrack,
} from '../../../yunomi/AutoCaptionOverlay';
import {
  broadcastTime,
  recordingPosition,
  formatClock,
  formatDuration,
} from '../../../yunomi/position';
import {
  RateSelect,
  QualitySelect,
  CropSelect,
  AudioDelaySetting,
  NormalizeVolumeToggle,
  SleepTimerSelect,
  VirtualSurroundToggle,
} from '../../../yunomi/QualitySelect';
import { hlsURL, toVideoFormat } from '../../../yunomi/route';
import { usePlayerAudio } from '../../../yunomi/usePlayerAudio';
import { type PlayerSnapshot } from '../../../yunomi/playerState';
import { SeekBar } from '../../../yunomi/SeekBar';
import { TsPlayer } from '../../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../../yunomi/usePlaybackRoute';
import { useAdaptiveQuality } from '../../../yunomi/useAdaptiveQuality';
import { usePlayerSettings } from '../../../yunomi/usePlayerSettings';
import { useWatchPositionSaver } from '../../../yunomi/useWatchPositionSaver';
import { usePlaybackReporter } from '../../../animeRecords/usePlaybackReporter';
import type { PlayerCommands } from '../../../yunomi/commands';
import { useCapture } from '../../../yunomi/PlayerTools';
import { StreamInfo, useStreamMeter } from '../../../yunomi/StreamInfo';
import { averageBitrate } from '../../../yunomi/streamMeter';
import { kindBitrates } from '../../../recordings/streamQuality';
import {
  CommandButton,
  PlayButton,
  PlayerControls,
  PositionStatus,
  SeekStepButtons,
  SettingsButton,
  VolumeControl,
} from '../../../yunomi/PlayerControls';
import { loadCmAutoSkip } from '../../../yunomi/cmAutoSkip';
import { cmMergeGap, cmSectionAt, mergeCmSections } from '../../../yunomi/cmSkip';
import { useFastForward } from '../../../yunomi/fastForward';
import { browserStorage } from '../../../yunomi/volume';
import { playableStart } from '../../../yunomi/reconnect';
import { resumePosition } from '../../../yunomi/resume';
import { parseStartSeconds } from '../../../yunomi/shareUrl';
import type { RecordingIdPlayQuery } from './__generated__/RecordingIdPlayQuery.graphql';
import { useHostedWatch, useWatchMini } from '../../../yunomi/WatchHost';
import { readableParams } from '../../../futon/readableId';
import {
  PlayerBox,
  PlayerErrorOverlay,
  PlayerFeedback,
  PlayerFrame,
  PlayerToolButtons,
  usePlayerInputs,
  usePlayerPresence,
  usePlayerShell,
} from '../../../yunomi/playerShell';
import { useVodTimeline } from '../../../yunomi/useVodTimeline';
import { useCmSkip } from '../../../yunomi/useCmSkip';

// PlaySearch は、再生の画面の URL のクエリ。t は、再生を始める位置 (録画の始まりからの秒)。キャプチャの履歴から撮った場面を開くのと、今の位置の URL の共有に使う (#457)。
type PlaySearch = { t?: number };

export const Route = createFileRoute('/play/recording/$recordingId')({
  // URL には素の ID を出し、画面には GraphQL の ID を渡す (#1346)。
  params: readableParams('recordingId', 'Recording'),
  // 右のカラムを置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => {
    const t = parseStartSeconds(search.t);

    return t === undefined ? {} : { t };
  },
  component: PlayPage,
});

// PlayPage は、録画を再生する画面。ブラウザの中で変換するか、kyusu の HLS で再生する (設計書 7.1)。
// 画面は、アプリのルートで描く (#1602)。
function PlayPage() {
  const { recordingId } = Route.useParams();
  const { t: start } = Route.useSearch();

  return useHostedWatch(<PlayWatch recordingId={recordingId} start={start} />);
}

// PlayWatch は、録画 recordingId を、位置 start (秒) から再生する画面の内容。
function PlayWatch({ recordingId, start }: { recordingId: string; start: number | undefined }) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();
  // 詳細に戻ったときに、開いた一覧に戻れるよう、一覧のクエリを引き継ぐ (#2146)。
  const listState = useListState();

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor
          size="sm"
          renderRoot={(props) => (
            <Link
              {...props}
              to="/recordings/$recordingId"
              params={{ recordingId }}
              state={listState}
            />
          )}
        >
          {t('player.backToRecording')}
        </Anchor>
      )}
      {/* 同じルートのまま別の録画へ移ったときは、前の録画のエラーを残さず読み直す (#1453)。 */}
      <ErrorBoundary
        resetKey={recordingId}
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor
                component={Link}
                to="/login"
                // 共有された URL をログインしていない端末で開いても、ログインの後に同じ位置から再生する。
                onClick={() => rememberReturnTo(window.location.pathname + window.location.search)}
              >
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
        description
        extended {
          heading
          text
        }
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
        cmSections {
          from
          to
        }
        autoCaptions {
          from
          to
          text
        }
        jikkyoCommentsUrl
        jikkyoOtherCommentsUrl
        jikkyoNicologCommentsUrl
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
        streamQuality {
          kind
          packets
        }
        service {
          name
          logoUrl
          jikkyoChannelId
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
        videoBitrate
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
  // shell は、再生元によらないプレイヤーの外側 (#2244)。別のウィンドウに出している間は、このページが隠れても、
  // 実況のコメントなどを描き続ける (設計書 7.5)。
  const shell = usePlayerShell();
  const { mediaRef, media, setMedia, pip, visible, fullscreen, controls, videoState, showFlash } =
    shell;
  const navigate = useNavigate();
  const listState = useListState();
  // BS4K と 8K の録画では、経路の選び方が変わる (#806)。
  // デコードが追いつかずにほかの画面で下げた段階の上限は、段階を下げる追っかけ再生でだけ引き継ぐ (#1513)。
  const playback = usePlaybackRoute(
    data.transcoding,
    data.node?.__typename === 'Recording' ? toVideoFormat(data.node.videoFormat) : 'HD',
    { inheritCap: data.node?.__typename === 'Recording' && data.node.recordedUntil == null },
  );
  const { route, choices, quality, setQuality } = playback;
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    commentSources,
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
  // duration は、録画の長さ (秒)。ファイルには途切れた区間の分の映像がないので、その分を差し引く。
  const duration =
    recording === null
      ? 0
      : recordingPosition(
          recording.recordedFrom,
          recording.gaps,
          recording.recordedUntil == null ? Date.now() : Date.parse(recording.recordedUntil),
        );
  // cmPositions は、CM とみなした区間 (#479) を、途切れた分を詰めた再生の位置にしたもの。録画の時間軸の日時で受け取る。
  const cmPositions = useMemo(
    () =>
      recording === null
        ? []
        : recording.cmSections.map((section) => ({
            from: recordingPosition(
              recording.recordedFrom,
              recording.gaps,
              Date.parse(section.from),
            ),
            to: recordingPosition(recording.recordedFrom, recording.gaps, Date.parse(section.to)),
          })),
    [recording],
  );
  // captionTrack は、出す字幕の種類 (#1682)。自動生成した字幕があるときだけ選べ、既定は放送の字幕にする。
  const [captionTrack, setCaptionTrack] = useState<CaptionTrack>('broadcast');
  const hasAutoCaptions = recording !== null && recording.autoCaptions.length > 0;
  const autoCaptionsShown = captions && hasAutoCaptions && captionTrack === 'auto';
  // cmSpans は、自動で飛ばすときに使う、続いた区間をまとめた CM の区間 (#1512)。
  const cmSpans = useMemo(() => mergeCmSections(cmPositions, cmMergeGap), [cmPositions]);
  // resumeAt は、再生を始める位置 (秒)。URL で位置を渡されたらそこから、なければ前回の続きから、続きもなければ 0 (設計書 7.5)。
  // Int64 は Relay の型では any になるので、数に直す。
  const savedMs = recording?.watchPositionMs == null ? null : Number(recording.watchPositionMs);
  // CM を自動で飛ばす設定で、URL で位置を渡されずに開いた位置が CM の中なら、最初から CM の終わりを求める (#1512)。
  // 開いてから飛ぶと、頭の CM を飛んでから続きへ移ったり、読み直すたびに通知が出たりする。CM の区間も開始位置も、
  // 開いた時点で揃っているので、ここで一度だけ決める。URL で渡された位置は、自分でシークしたときと同じく飛ばさない。
  const [opening] = useState(() => {
    if (start !== undefined) {
      return { at: Math.min(start, duration), skipped: false, resumed: false };
    }
    const at = resumePosition(savedMs, duration);
    const span = loadCmAutoSkip(browserStorage()) ? cmSectionAt(cmSpans, at) : null;

    return span === null
      ? { at, skipped: false, resumed: at > 0 }
      : { at: Math.min(span.to, duration), skipped: true, resumed: at > 0 };
  });
  // 終わりの直前から読むと、映像のコマが届かずに失敗するので、映像が入るだけ手前から始める (#1576)。
  const resumeAt = playableStart(opening.at, duration);
  // kyusu の HLS は、録画の全体を 1 つのプレイリストで表すので、video の時刻がそのまま録画の中の位置になる。
  const hls = route.kind === 'kyusu';
  // timeline は、録画の始まりからの秒で、位置を指定して読み直す配信 (設計書 7.2)。kyusu の経路では、仮想 HLS を流す。
  // 配信は、求めた位置の手前の I フレームから流すので、位置は、流し始めた位置から数える (#536)。
  // 終わりの直前から読むと、映像のコマが届かずに失敗するので、映像が入るだけ手前から読む (#1576)。
  // CM の区間が録画の終わりまで続く回で、CM を飛ばすと、ここに来る。
  const timeline = useVodTimeline({
    streamUrl: recording?.streamUrl ?? '',
    media,
    mediaRef,
    convert: route.kind === 'convert' && recording !== null,
    hls,
    initial: resumeAt,
    duration,
    restartFrom: (seconds) => playableStart(seconds, duration),
  });
  // positionRef は、別のウィンドウへの移し替えや、画質と音声の切り替えで、読み直すときに続ける位置。
  // currentPosition は、video の時刻から求め直した今の再生位置。実況のコメントを、画面の更新ごとに描くのに使う。
  const {
    stream,
    hlsStream,
    position,
    positionRef,
    currentPosition,
    restartAt: startAt,
    jumpTo,
    retry,
    error,
  } = timeline;
  const [seeking, setSeeking] = useState<number | null>(null);
  const [paused, setPaused] = useState(true);
  // cm は、CM とみなした区間 (#479) の帯と、飛ばすボタンと、自動で飛ばす判定 (#1512)。
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
    initialSeek: start === undefined ? null : resumeAt,
  });
  // seek は、利用者の操作で、指定の位置に移る。skip は、今の位置から相対に移り、CM を飛んだら true を返す。
  const { seek, skip } = cm;

  // channelId は、実況のチャンネルの ID。実況を切っているユーザーにもボタンを出すため、外部へ問い合わせずに返る値を使う (設計書 7.4)。
  const channelId = recording?.service.jikkyoChannelId ?? null;
  useWatchPositionSaver(recordingId, currentPosition, !paused);
  // 実際に再生した区間を送り、見たアニメを記録させる (#927)。
  usePlaybackReporter(recording === null ? null : { recordingId }, media, () =>
    recording === null
      ? null
      : broadcastTime(recording.recordedFrom, recording.gaps, currentPosition()),
  );
  // nicologCmSections は、上映会のコメント (NICOLOG) を合わせる、録画の CM の区間 (#1363)。CM の区間がなければ null にする。
  const nicologCmSections = cmPositions.length === 0 ? null : cmPositions;
  const jikkyo = useRecordedJikkyo(
    channelId !== null && jikkyoEnabled && recording !== null ? recording.jikkyoCommentsUrl : null,
    currentPosition,
    visible,
    // 録画中の録画を追っかけ再生している間は、リアルタイムのコメントも流す (#667)。
    channelId !== null && jikkyoEnabled && recording !== null && recording.recordedUntil == null
      ? { channelId, recordedFrom: recording.recordedFrom, gaps: recording.gaps }
      : null,
    commentSettings,
    channelId !== null && jikkyoEnabled && recording !== null
      ? recording.jikkyoOtherCommentsUrl
      : null,
    // 録画自身の過去ログの `OP` で合わせるので、それが読めるときだけ取得する。
    channelId !== null &&
      jikkyoEnabled &&
      nicologCmSections !== null &&
      recording !== null &&
      commentSources.includes('NICOLOG')
      ? { url: recording.jikkyoNicologCommentsUrl, cmSections: nicologCmSections }
      : null,
  );
  // recordedRows は、コメントの一覧に出す、再生位置までに流れた過去ログ (#489)。再生位置は 1 秒に何度も変わるので、
  // 流れたコメントの数が変わったときだけ作り直し、時刻の文字は、コメントごとに一度だけ作る。
  const listedCount =
    jikkyo.comments === null ? 0 : countUntil(jikkyo.comments, position - jikkyo.offset);
  const commentLabels = useMemo(
    () => new Map<number, string>(),
    [jikkyo.comments, recording, jikkyo.offset],
  );
  const recordedRows = useMemo(() => {
    if (jikkyo.comments === null || recording === null) {
      return [];
    }

    return listedUntil(jikkyo.comments, listedCount).map(({ index, comment }) => {
      let label = commentLabels.get(index);
      if (label === undefined) {
        label = formatClock(
          broadcastTime(recording.recordedFrom, recording.gaps, comment.time + jikkyo.offset),
        );
        commentLabels.set(index, label);
      }

      return {
        key: index,
        label,
        content: comment.content,
        ...(comment.userId === undefined ? {} : { userId: comment.userId }),
      };
    });
  }, [jikkyo.comments, jikkyo.offset, listedCount, recording, commentLabels]);
  // partBands は、実況のコメントから見つけた番組の区切りのマーク (#1358)。コメントの位置をずらしたら、マークも同じだけ動かす。
  const partBands = useMemo(
    () =>
      findPartMarkers(jikkyo.ownComments ?? []).map((marker) => {
        const at = Math.max(marker.time + jikkyo.offset, 0);
        const part = t(`player.partMarker.${marker.part}`);

        return {
          from: at,
          to: at,
          label: marker.noCmBefore ? t('player.partMarkerNoCm', { part }) : part,
          kind: 'part' as const,
        };
      }),
    [jikkyo.ownComments, jikkyo.offset, t],
  );
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

  // streamMeter は、右のカラムの「情報」に出す、届いた映像のビットレートと放送の形式をバッファに保持する (#828)。
  const streamMeter = useStreamMeter(playback.current);
  // 仮想サラウンド (#454)。5.1ch の番組を、ブラウザの中での変換で見ているときだけ使える。
  const surroundAvailable = route.kind === 'convert' && isSurroundAudio(recording?.audios ?? null);
  useVirtualSurround(media, surroundAvailable);

  // seek は、指定の位置に移る。kyusu の HLS は、プレイリストの中を移るだけで、読み直さない。
  // captionEntries は、字幕の履歴 (#450)。字幕が出たときの再生位置で並べ、行を押すとその位置へ移る。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  // currentCaption は、今出ている字幕の文。キャプチャに書き込む (#458)。
  const currentCaption = useRef<string | null>(null);
  // 字幕の外字を置き換えるテーブルを読む (#825)。
  const drcs = useDRCSReplacements();
  const onCaptionText = (text: string) => {
    const normalized = normalizeCaption(text);
    currentCaption.current = normalized === '' ? null : normalized;
    const at = currentPosition();
    setCaptionEntries((list) => insertRecordedCaption(list, at, text));
  };
  // changeQuality は、画質を変え、それまでの位置から続ける。
  const changeQuality = (next: string) => {
    startAt(positionRef.current);
    setQuality(next);
  };
  // 映像だけの遅れを飛び直しで修正する (#1513)。デコードが追いつかなければ、画質が「自動」の間は段階を下げ、それまでの位置から続ける。
  // 段階を下げるのは、録画中の追っかけ再生でだけ。
  const adaptive = useAdaptiveQuality(
    media,
    {
      automatic: playback.automatic,
      lowerAvailable: playback.lowerAvailable,
      stepDownForDecode: () => {
        const next = playback.stepDownForDecode();
        if (next !== null) {
          startAt(positionRef.current);
        }

        return next;
      },
      current: playback.current,
      networkLadder: playback.networkLadder,
      setNetworkStep: (name) => {
        startAt(positionRef.current);
        playback.setNetworkStep(name);
      },
    },
    {
      stepDown: stillRecording,
      pictureInPicture: pip.container !== null,
      // 録画は先に読むので、受け取りの速さで余裕を確認して上げる。オリジナルのビットレートは、録画の平均で比較する。
      network: {
        mode: 'rate',
        convert: route.kind === 'convert',
        topBitrate: recording === null ? null : averageBitrate(recording, Date.now()),
        // 録画中の追っかけ再生は端に追いつくと録画が伸びるのを待つので、終わりがない。
        ends: recording !== null && !stillRecording,
      },
    },
  );

  // 音声は、経路ごとのメニューで選び、画質を変えても同じ音声を保つ。kyusu の音声を変えるときも、それまでの位置から続ける。
  // kyusu の経路では、録画を始めたときの番組の音声の構成から選ぶ。
  const audio = usePlayerAudio(route, recording?.audios ?? null, () =>
    startAt(positionRef.current),
  );
  // 再生速度は、video の要素を作り直したときにも当て直す。
  const [rate, setRate] = useState(1);
  // 早回し (#1572) の間は、選んだ速さより速く再生する。録画中の追っかけ再生では、録画の端の手前で元の速さに戻す。
  const fastForward = useFastForward(rate, { growing: stillRecording, position, duration });
  useEffect(() => {
    if (media !== null) {
      media.defaultPlaybackRate = rate;
      media.playbackRate = fastForward.playbackRate;
    }
  }, [rate, fastForward.playbackRate, media, stream.url, hlsStream.loads]);

  // 早回しの間は、押している間ずっと、速さを映像の上に出しておく。
  const shownFlash = fastForward.active
    ? t('player.flash.fastForward', { rate: fastForward.playbackRate })
    : shell.flash;
  const capture = useCapture({
    crop: shell.crop,
    frameRef: shell.playerBoxRef,
    media: () => mediaRef.current,
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
  const saveCapture = async () => {
    if ((await capture.capture('save')) === 'saved') {
      showFlash(t('player.flash.captured'));
    }
  };
  // commentOffsetCommands は、実況のコメントの位置 (#1357) の操作。変えたら、ずらした量を映像の上に短く出す。
  const commentOffsetCommands = useCommentOffsetCommands((ms) =>
    showFlash(t('player.flash.commentOffset', { seconds: formatCommentOffset(ms) })),
  );
  // commands は、この画面で今使える操作 (#809)。キー、OS のメディア操作、ほかの端末からの操作は、どれもここから呼ぶ。
  const commands: PlayerCommands = {
    ...shell.commands,
    ...shell.seekCommands({ skip, seekTo: seek, currentPosition, retry }),
    ...(channelId === null || !jikkyoEnabled ? {} : commentOffsetCommands),
    toggleCaptions: () => setCaptions(!captions),
    ...(channelId === null ? {} : { toggleJikkyo: () => setJikkyoEnabled(!jikkyoEnabled) }),
    capture: () => void saveCapture(),
    selectAudio: audio.remote.select,
    stop: () =>
      void navigate({ to: '/recordings/$recordingId', params: { recordingId }, state: listState }),
  };
  // 映像を長押ししたまま横に滑らせて、再生位置を動かす (#1601)。離したら、自分でシークしたものとして扱う。
  const seekLabel = (value: number) =>
    recording === null
      ? formatDuration(value)
      : `${formatDuration(value)} (${formatClock(broadcastTime(recording.recordedFrom, recording.gaps, value))})`;
  const inputs = usePlayerInputs(shell, commands, {
    // タッチで映像の左右の 3 分の 1 をダブルタップすると、10 秒戻るか進む (#452)。
    onDoubleTap: (side) => {
      if (!skip(side === 'left' ? -10 : 10)) {
        showFlash(t(side === 'left' ? 'player.flash.seekBackward' : 'player.flash.seekForward'));
      }
    },
    slideSeek: {
      enabled: duration > 0,
      range: () => ({ position: currentPosition(), duration }),
      onScrub: setSeeking,
      onCommit: (target) => {
        setSeeking(null);
        seek(target);
      },
      onCancel: () => setSeeking(null),
    },
  });
  const { sleep, scrub } = inputs;
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
    playbackRate: fastForward.playbackRate,
    volume: videoState.volume,
    muted: videoState.muted,
    audios: audio.remote.choices,
    audio: audio.remote.selected,
    dataBroadcast: false,
    keyGroups: [],
  } satisfies PlayerSnapshot;
  usePlayerPresence(player, commands, recording === null || media === null ? null : player);

  // 止めているかを追う。位置は、stream と hlsStream が追う。
  useEffect(() => {
    if (media === null) {
      return;
    }
    const onPause = () => setPaused(true);
    const onPlay = () => setPaused(false);
    media.addEventListener('pause', onPause);
    media.addEventListener('play', onPlay);
    setPaused(media.paused);

    return () => {
      media.removeEventListener('pause', onPause);
      media.removeEventListener('play', onPlay);
    };
    // 画質を変えたときは、video の要素を作り直すので、付け直す。
  }, [media]);

  if (recording === null) {
    return <Alert color="amber">{t('recordings.notFound')}</Alert>;
  }
  const shown = seeking ?? position;
  // gapMarks は、録画が途切れた位置のマーク (#479)。途切れた区間はシークバーの長さに含めないので (#754)、長さのないマークにし、
  // カーソルを乗せたら、途切れていた長さを出す。
  const gapMarks = (recording?.gaps ?? []).map((gap) => {
    const at = recordingPosition(recording!.recordedFrom, recording!.gaps, Date.parse(gap.from));
    const length = Math.max((Date.parse(gap.to) - Date.parse(gap.from)) / 1000, 0);

    return { from: at, to: at, label: t('player.gapMark', { duration: formatDuration(length) }) };
  });
  // skipButton は、いま再生している CM の中で出す、区間の終わりへ飛ばすボタン (#1300)。
  const skipButton = seeking === null && cm.button(position, controls.visible);

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <SeekBar
          bands={[...cm.bands, ...gapMarks, ...partBands]}
          duration={duration}
          value={shown}
          previewUrl={recording.previewUrl}
          label={seekLabel}
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
          <SeekStepButtons commands={commands} onFastForward={fastForward.setHeld} />
          {/* 狭い画面 (スマートフォン) では、再生位置を出す幅を空けるため、ミュートのボタンを外す。本体の音量キーで操作できる。 */}
          <VolumeControl video={videoState} commands={commands} optional />
        </>
      }
      status={
        <PositionStatus
          position={formatDuration(shown)}
          rest={` / ${formatDuration(duration)} · ${formatClock(broadcastTime(recording.recordedFrom, recording.gaps, shown))}`}
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
          <CommandButton
            commands={commands}
            command="toggleJikkyo"
            label={jikkyoLabel}
            icon={MessageSquareText}
            active={jikkyoEnabled}
          />
          <SettingsButton>
            <QualitySelect
              choices={choices}
              quality={quality}
              current={playback.current}
              currentCodec={playback.currentCodec}
              onChange={changeQuality}
            />
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

  const playerBox = (
    <PlayerBox shell={shell} inputs={inputs}>
      {route.kind === 'convert' && (
        <TsPlayer
          url={stream.url}
          handover={timeline.handover}
          mediaRef={mediaRef}
          onMedia={setMedia}
          controls={controlBar}
          // 自動生成した字幕を出している間は、放送の字幕を重ねない。
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
          overlay={
            <>
              {autoCaptionsShown && (
                <AutoCaptionOverlay
                  recordedFrom={recording.recordedFrom}
                  gaps={recording.gaps}
                  captions={recording.autoCaptions}
                  clock={currentPosition}
                />
              )}
              {jikkyo.overlay}
              <PlayerFeedback
                shell={shell}
                flash={shownFlash}
                notice={adaptive.notice}
                scrub={
                  scrub === null
                    ? null
                    : { seconds: scrub, label: seekLabel(scrub), previewUrl: recording.previewUrl }
                }
                loadingHidden={error !== null}
              />
              {skipButton}
              <PlayerErrorOverlay shell={shell} error={error} onRetry={retry} />
            </>
          }
          onAudio={audio.onAudio}
          onReading={(reading) => {
            streamMeter.onReading(reading);
            adaptive.network.onReading(reading);
          }}
          audio={audio.playerAudio}
          coverRef={cm.coverRef}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          // 読み直すたびに URL を変え、hls.js を作り直す。fragment は、サーバーへは送られない。
          url={`${hlsURL(recording.streamUrl, route.preset, audio.kyusuAudio)}#${hlsStream.loads}`}
          mediaRef={mediaRef}
          onMedia={setMedia}
          onSegment={(segment) => {
            streamMeter.onSegment(segment);
            adaptive.network.onSegment(segment);
          }}
          controls={controlBar}
          // 自動生成した字幕を出している間は、放送の字幕を重ねない。
          captions={captions && !autoCaptionsShown}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={onCaptionText}
          live={false}
          startPosition={hlsStream.start}
          overlay={
            <>
              {autoCaptionsShown && (
                <AutoCaptionOverlay
                  recordedFrom={recording.recordedFrom}
                  gaps={recording.gaps}
                  captions={recording.autoCaptions}
                  clock={currentPosition}
                />
              )}
              {jikkyo.overlay}
              <PlayerFeedback
                shell={shell}
                flash={shownFlash}
                notice={adaptive.notice}
                scrub={
                  scrub === null
                    ? null
                    : { seconds: scrub, label: seekLabel(scrub), previewUrl: recording.previewUrl }
                }
                loadingHidden={error !== null}
              />
              {skipButton}
              <PlayerErrorOverlay shell={shell} error={error} onRetry={retry} />
            </>
          }
          onLoaded={timeline.onHlsLoaded}
          onError={timeline.onError}
          coverRef={cm.coverRef}
        />
      )}
    </PlayerBox>
  );

  // 実況の過去ログがあれば、右のカラムで字幕の履歴と切り替えて出す。
  const commentsShown = jikkyo.comments !== null && jikkyo.comments.length > 0;

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
            title={<ProgramTitle name={recording.name} />}
            serviceName={recording.service.name}
            serviceLogoUrl={recording.service.logoUrl}
            time={recording}
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
          <WatchDescription text={recording.description} extended={recording.extended} />
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
                          // 全画面の間は一覧が見えないので、コメントが届いても描き直さない。
                          frozen={fullscreen.active}
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
                    videoLag={adaptive.videoLag}
                    network={adaptive.network}
                    meter={streamMeter.meter}
                    media={media}
                    quality={playback.current}
                    kind={route.kind === 'convert' || route.kind === 'kyusu' ? route.kind : null}
                    audios={recording?.audios ?? null}
                    recordedKinds={
                      recording === null ? null : kindBitrates(recording.streamQuality, duration)
                    }
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
