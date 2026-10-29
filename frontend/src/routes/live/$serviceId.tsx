import { ActionIcon, Alert, Anchor, Button, Group, Menu, Modal, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import {
  Captions,
  EllipsisVertical,
  ExternalLink,
  History,
  Keyboard,
  MessageSquareText,
  Tv,
} from 'lucide-react';
import { startTransition, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import {
  ErrorBoundary,
  PageLoader,
  ProgramTitle,
  splitProgramMarks,
  toHalfWidth,
  useDocumentTitle,
} from '../../futon';
import { CommentList } from '../../jikkyo/CommentList';
import { WatchPanel } from '../../yunomi/WatchPanel';
import { LiveServiceList } from '../../guide/LiveServiceList';
import { WatchHeading, WatchLayout, watchCommentsHeight } from '../../yunomi/WatchLayout';
import { isSurroundAudio, useVirtualSurround } from '../../yunomi/useVirtualSurround';
import { useVolumeNormalizer } from '../../yunomi/volumeNormalize';
import { noCrop, type Crop } from '../../yunomi/crop';
import { CaptionHistory } from '../../yunomi/CaptionHistory';
import {
  appendLiveCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../yunomi/captionHistory';
import { formatClock } from '../../yunomi/position';
import { SeekBar } from '../../yunomi/SeekBar';
import { liveThresholdSeconds, useLiveTimeshift } from '../../yunomi/useLiveTimeshift';
import { JikkyoPanel } from '../../jikkyo/JikkyoPanel';
import { useLiveJikkyo } from '../../jikkyo/useLiveJikkyo';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { HlsPlayer } from '../../yunomi/HlsPlayer';
import { PipPlaceholder } from '../../yunomi/PipPlaceholder';
import { DataBroadcastRemote, DataBroadcastRemoteOverlay } from '../../yunomi/DataBroadcastRemote';
import { useDataBroadcastKeys } from '../../yunomi/dataBroadcastKeys';
import {
  BigPlayButton,
  PlayerLoading,
  CommandButton,
  FullscreenButton,
  LiveStatus,
  PlayButton,
  PlayerAlert,
  PlayerControls,
  playerControlsClasses,
  PlayerNotice,
  SettingsButton,
  VolumeControl,
  PlayerFlash,
  usePlayerFlash,
} from '../../yunomi/PlayerControls';
import {
  PlaybackUnavailable,
  QualitySelect,
  CropSelect,
  NormalizeVolumeToggle,
  VirtualSurroundToggle,
} from '../../yunomi/QualitySelect';
import { useStreamsDrains } from '../../changes/DataChanges';
import {
  canRetryImmediately,
  reconnectDelay,
  recordingsThatTookTuner,
  skipBrokenMedia,
} from '../../yunomi/reconnect';
import { dataBroadcastSource, hlsURL, toVideoFormat } from '../../yunomi/route';
import { RemoteKey, useDataBroadcast } from '../../yunomi/useDataBroadcast';
import { useRemotePlayer } from '../../yunomi/useRemotePlayer';
import { useDataBroadcastPreload } from '../../yunomi/dataBroadcastPreload';
import { useLiveCatchUp, useLowLatency } from '../../yunomi/liveCatchUp';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useMediaSession } from '../../yunomi/useMediaSession';
import { ExternalStreamURL } from '../../streams/ExternalStreamURL';
import { ShortcutsHelp } from '../../yunomi/ShortcutsHelp';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import { runCommand, type PlayerCommands } from '../../yunomi/commands';
import { useAutoPictureInPicture } from '../../yunomi/autoPictureInPicture';
import {
  CaptureButton,
  openPictureInPicture,
  PipButton,
  useCapture,
} from '../../yunomi/PlayerTools';
import { PlayerStats, StatsToggle } from '../../yunomi/PlayerStats';
import { onVideoSurface, useControlsVisibility } from '../../yunomi/useControlsVisibility';
import { useFullscreen } from '../../yunomi/useFullscreen';
import { useAutoplayBlocked } from '../../yunomi/useAutoplayBlocked';
import { useVideoLoading } from '../../yunomi/useVideoLoading';
import { useVideoState } from '../../yunomi/useVideoState';
import { stepVolume } from '../../yunomi/volume';
import { currentVolume } from '../../yunomi/volumeBoost';
import { useWakeLock } from '../../yunomi/useWakeLock';
import { useDocumentPictureInPicture } from '../../yunomi/useDocumentPictureInPicture';
import { useLiveLag } from '../../yunomi/useLiveLag';
import { useVisible } from '../../yunomi/useVisible';
import type { ServiceIdLiveQuery } from './__generated__/ServiceIdLiveQuery.graphql';
import type { ServiceIdRecordingsQuery } from './__generated__/ServiceIdRecordingsQuery.graphql';
import { ProgramOverlay } from '../../yunomi/ProgramOverlay';
import { neighborServices } from '../../yunomi/channels';

// volumeStep は、↑ と ↓ で音量を上げ下げする幅。
const volumeStep = 0.05;

export const Route = createFileRoute('/live/$serviceId')({
  // 右の列を置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  component: LiveServicePage,
});

const recordingsQuery = graphql`
  query ServiceIdRecordingsQuery {
    recordings(first: 20, filter: { states: [RECORDING] }) {
      edges {
        node {
          name
          recordedFrom
          service {
            name
            logoUrl
          }
        }
      }
    }
  }
`;

// LiveServicePage は、サービスをライブ視聴する画面。ブラウザの中で変換するか、kyusu の HLS で再生する (設計書 7.1)。
function LiveServicePage() {
  const { t } = useTranslation();
  const { serviceId } = Route.useParams();
  // 放送中の番組を出すので、画面を開いた時刻を、待たない親で決める。
  const [range, setRange] = useState(() => liveRange(Date.now()));
  // 番組が終わったら、次の番組を読み直す。読み直す間も、再生は続ける。
  const refresh = () => startTransition(() => setRange(liveRange(Date.now())));

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/live" size="sm">
        {t('live.back')}
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
                ? t('live.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Live key={serviceId} serviceId={serviceId} range={range} onProgramEnd={refresh} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// liveRange は、now から、次の番組まで読める時間帯。放送中の番組と、その次の番組を引くのに使う (#438)。
function liveRange(now: number): { now: number; from: string; to: string } {
  return {
    now,
    from: new Date(now).toISOString(),
    to: new Date(now + liveLookahead).toISOString(),
  };
}

// liveLookahead は、次の番組を探す、いまからの時間。
const liveLookahead = 6 * 3_600_000;

// undecidedRecheckMs は、終了時刻が未定の番組や、番組がないときに、放送中の番組を読み直す間隔。
const undecidedRecheckMs = 60_000;

// Disconnection は、視聴が切れたことと、つなぎ直すまでの秒と、切れた理由とみなす録画。
type Disconnection = {
  at: number;
  retryIn: number;
  recordings: { name: string; serviceName: string }[];
};

// DisconnectionReason は、視聴が切れた理由と、つなぎ直すまでの秒を示す。ページの知らせと、映像に重ねる知らせで使う。
function DisconnectionReason({ disconnection }: { disconnection: Disconnection }) {
  const { t } = useTranslation();

  return (
    <Stack gap={4}>
      {disconnection.recordings.length > 0 ? (
        <Text size="sm">
          {t('live.tunerTaken', {
            recordings: disconnection.recordings
              .map((recording) =>
                toHalfWidth(
                  `${recording.serviceName}「${splitProgramMarks(recording.name).name}」`,
                ),
              )
              .join('、'),
          })}
        </Text>
      ) : (
        <Text size="sm">{t('live.streamEnded')}</Text>
      )}
      <Text size="sm" opacity={0.75}>
        {t('live.retryIn', { seconds: disconnection.retryIn })}
      </Text>
    </Stack>
  );
}

function Live({
  serviceId,
  range,
  onProgramEnd,
}: {
  serviceId: string;
  range: { now: number; from: string; to: string };
  onProgramEnd: () => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const data = useLazyLoadQuery<ServiceIdLiveQuery>(
    graphql`
      query ServiceIdLiveQuery($id: ID!, $from: Time!, $to: Time!) {
        node(id: $id) {
          __typename
          ... on Service {
            name
            logoUrl
            streamUrl
            videoFormat
            jikkyo {
              id
              name
            }
            # タイムシフトのバッファにある番組の一覧を、メニューから開けるかを知る (設計書 5.7)。
            timeshiftPrograms {
              startAt
            }
            # タイムシフトのバッファの範囲。ライブ視聴から、過去へシークするのに使う (設計書 5.7)。
            timeshiftBuffer {
              from
              gaps {
                from
                to
              }
              streamUrl
              hlsUrl
            }
          }
        }
        guide(from: $from, to: $to, services: [$id]) {
          programs {
            name
            startAt
            endAt
            endUndecided
            audios {
              componentType
              languages
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
        # OS のメディア操作の前と次で移る局と、データ放送の選局で移る先を引く、番組表と同じ並びのサービス (#493、#440)。
        services {
          id
          name
          networkId
          serviceId
          channelType {
            type
          }
        }
      }
    `,
    { id: serviceId, from: range.from, to: range.to },
  );
  const navigate = useNavigate();
  const neighbors = neighborServices(data.services, serviceId);
  const tune = (id: string) => void navigate({ to: '/live/$serviceId', params: { serviceId: id } });
  const videoRef = useRef<HTMLVideoElement | null>(null);
  // 画質を変えると video の要素を作り直すので、見えているかは、変わらない外側の要素で調べる。
  const frameRef = useRef<HTMLDivElement | null>(null);
  const inPage = useVisible(frameRef);
  // pip は、視聴画面を出す別のウィンドウ。出している間は、このページが隠れても、実況のコメントなどを描き続ける (設計書 7.5)。
  const pip = useDocumentPictureInPicture();
  const visible = inPage || pip.container !== null;
  // playerBoxRef は、プレイヤーを包む要素。別のウィンドウに出すと移るので、キャプチャと全画面は、これを使う。
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  // video は、今の video の要素。画質を変えたときや、別のウィンドウに移したときに作り直される。
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoState = useVideoState(video);
  // ブラウザに音ありの自動再生を断られたら、映像の真ん中に再生ボタンを出す。
  const autoplay = useAutoplayBlocked(video);
  const videoLoading = useVideoLoading(video);
  const fullscreen = useFullscreen(playerBoxRef);
  // overlaid は、プレイヤーだけを出していて、ページの知らせやリモコンが見えないかどうか。そのときは、映像に重ねて出す (#435)。
  const overlaid = fullscreen.active || pip.container !== null;
  // shortcutsOpen は、キーボードショートカットの一覧を出しているかどうか。
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  // ショートカットの一覧を見ている間は、操作バーとカーソルを隠さない。
  const controls = useControlsVisibility({ playing: !videoState.paused && !shortcutsOpen });
  // 再生している間は、画面を消さない。
  useWakeLock(!videoState.paused);
  // videoFormat は、放送中の番組の映像の種類。BS4K と 8K では、経路の選び方が変わる (#806)。
  const videoFormat =
    data.node?.__typename === 'Service' ? toVideoFormat(data.node.videoFormat) : 'HD';
  const { route, choices, quality, setQuality } = usePlaybackRoute(data.transcoding, videoFormat);
  // BS4K と 8K のデータ放送は、BML でなく HTML5 で作られていて、web-bml では描けないので出さない (設計書 7.4)。
  const dataBroadcastAvailable = videoFormat === 'HD';
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    setJikkyoEnabled,
    comments: commentSettings,
    muteUser,
    captions: captionSettings,
  } = usePlayerSettings();
  const channel = data.node?.__typename === 'Service' ? (data.node.jikkyo ?? null) : null;
  const jikkyo = useLiveJikkyo(
    jikkyoEnabled ? (channel?.id ?? null) : null,
    videoRef,
    visible,
    commentSettings,
  );
  // dataBroadcastEnabled は、データ放送を出すかどうか。映像を覆うので、既定では出さない (設計書 7.4)。
  const [dataBroadcastEnabled, setDataBroadcastEnabled] = useState(false);
  const [dataBroadcastPreload] = useDataBroadcastPreload();
  // timeshift は、タイムシフトのバッファへのシーク (設計書 5.7)。ブラウザ内で変換する経路では TS を、kyusu の経路では仮想 HLS を流す。
  const timeshift = useLiveTimeshift(
    data.node?.__typename === 'Service' ? (data.node.timeshiftBuffer ?? null) : null,
    video,
    route.kind === 'convert' ? 'convert' : route.kind === 'kyusu' ? 'hls' : null,
  );
  // seeking は、シークバーを動かしている間の位置。
  const [seeking, setSeeking] = useState<number | null>(null);
  // 低遅延モードなら、放送からの遅れが広がったときに、少し速く再生して追いつく (#495)。
  const [lowLatency] = useLowLatency();
  // kyusu の経路は、hls.js が追いつくので、ブラウザ内で変換する経路でだけ、再生の速さを変える。バッファを再生している間は追いつかない。
  useLiveCatchUp(video, lowLatency && route.kind === 'convert' && !timeshift.behind);
  // dataBroadcastUsed は、このページでデータ放送を出したことがあるか。一度出したら、消しても受け取りを続け、次はすぐに出す。
  const [dataBroadcastUsed, setDataBroadcastUsed] = useState(false);
  if (dataBroadcastEnabled && !dataBroadcastUsed) {
    setDataBroadcastUsed(true);
  }
  // session は、つなぎ直すたびに増やし、URL を変えて読み直させる。
  const [session, setSession] = useState(0);
  const attempt = useRef(0);
  // preempted は、録画のためにチューナーを使われて切れたかどうか。つなぎ直せない間の理由に使う。
  const preempted = useRef(false);
  const [disconnection, setDisconnection] = useState<Disconnection | null>(null);
  // retryingImmediately は、知らせを出さずにすぐつなぎ直している最中かどうか。lastImmediateRetry は、その時刻 (#1083)。
  const retryingImmediately = useRef(false);
  const lastImmediateRetry = useRef<number | null>(null);
  // lastBrokenSkip は、バッファの再生で、壊れた所を最後に飛び越えた先の位置。
  const lastBrokenSkip = useRef<number | null>(null);

  // 視聴が切れたら、まず知らせを出さずにすぐつなぎ直す。tenban の入れ替えや回線の瞬断なら、これで戻る (#1083)。
  // ブラウザ内での変換では、切れる前に受け取った映像を流したまま、裏でつなぎ直して切り替える。
  // すぐのつなぎ直しでも戻れなければ、理由を調べ、知らせを出して、間隔を延ばしながらつなぎ直す。
  const disconnected = () => {
    if (disconnection !== null) {
      return;
    }
    const at = Date.now();
    if (
      !retryingImmediately.current &&
      attempt.current === 0 &&
      canRetryImmediately(lastImmediateRetry.current, at)
    ) {
      retryingImmediately.current = true;
      lastImmediateRetry.current = at;
      setSession((n) => n + 1);

      return;
    }
    retryingImmediately.current = false;
    // 録画にチューナーを使われて切れた後は、録画が続いている間はつなぎ直せない。
    const retried = attempt.current > 0 && preempted.current;
    const delay = reconnectDelay(attempt.current);
    setDisconnection({ at, retryIn: delay, recordings: [] });
    void fetchQuery<ServiceIdRecordingsQuery>(
      environment,
      recordingsQuery,
      {},
      {
        fetchPolicy: 'network-only',
      },
    )
      .toPromise()
      .then((result) => {
        const inProgress =
          result?.recordings.edges.map((edge) => ({
            name: edge.node.name,
            recordedFrom: edge.node.recordedFrom,
            serviceName: edge.node.service.name,
          })) ?? [];
        // 最初に切れたときは、その前後に始まった録画を理由とみなす。つなぎ直せなかったときは、
        // 録画が続いている間はチューナーが空かないので、録画中のものをすべて示す。
        const recordings = retried ? inProgress : recordingsThatTookTuner(inProgress, at);
        if (recordings.length > 0) {
          preempted.current = true;
        }
        setDisconnection((prev) => (prev?.at === at ? { ...prev, recordings } : prev));
      })
      // 理由を調べられなくても、つなぎ直しは続ける。
      .catch(() => {});
  };

  // reconnectNow は、数え終わるのを待たずに、すぐにつなぎ直す。
  const reconnectNow = () =>
    setDisconnection((prev) => (prev === null ? null : { ...prev, retryIn: 0 }));

  // つなぎ直すまでを数える。
  useEffect(() => {
    if (disconnection === null) {
      return;
    }
    if (disconnection.retryIn <= 0) {
      attempt.current += 1;
      setDisconnection(null);
      setSession((n) => n + 1);

      return;
    }
    const timer = setTimeout(
      () =>
        setDisconnection((prev) => (prev === null ? null : { ...prev, retryIn: prev.retryIn - 1 })),
      1000,
    );

    return () => clearTimeout(timer);
  }, [disconnection]);

  // 一時停止したら、放送から遅れる。ブラウザ内での変換では、長く止めていたら再開するときに読み直す。
  // kyusu の経路は、プレイヤーが読み直す (設計書 7.3)。どちらの経路でも、遅れている間は「ライブに戻る」で読み直せる (#449)。
  const reload = () => setSession((n) => n + 1);
  // tenban が止まる前に知らせてきたら、ブラウザ内での変換では、新しい接続を張ってから切り替える (#1085)。
  // kyusu の経路は、セグメントごとに読むので、止まる tenban を避けて次のセグメントを読む。つなぎ直さなくてよい。
  const drains = useStreamsDrains();
  const seenDrains = useRef(drains);
  useEffect(() => {
    if (drains === seenDrains.current) {
      return;
    }
    seenDrains.current = drains;
    if (route.kind === 'convert' && disconnection === null) {
      setSession((n) => n + 1);
    }
  }, [drains, route.kind, disconnection]);
  // statsShown は、再生の統計情報を映像の上に出しているかどうか (#499)。
  const [statsShown, setStatsShown] = useState(false);
  // 音量の補正 (#499)。端末ごとの設定に合わせる。
  useVolumeNormalizer(video);
  // crop は、L 字放送の切り抜き (#496)。その場限りで、画面を開き直したら戻す。
  const [crop, setCrop] = useState<Crop>(noCrop);
  // captionEntries は、字幕の履歴 (#450)。局を変えたら、画面ごと作り直すので捨てる。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  // currentCaption は、今出ている字幕の文。キャプチャに書き込む (#458)。
  const currentCaption = useRef<string | null>(null);
  const onCaptionText = (text: string) => {
    const normalized = normalizeCaption(text);
    currentCaption.current = normalized === '' ? null : normalized;
    // バッファを再生している間は、映像の放送時刻で並べる。
    const at = timeshift.behind ? timeshift.clockAt(timeshift.currentPosition()) : Date.now();
    setCaptionEntries((list) => appendLiveCaption(list, at, text));
  };
  // externalOpen は、外部プレイヤー用の URL を発行するダイアログを出しているかどうか (#487)。
  const [externalOpen, setExternalOpen] = useState(false);
  // tuneRequest は、データ放送の文書が選局しようとしたサービス。確かめてから移る (#440)。
  const [tuneRequest, setTuneRequest] = useState<{ id: string; name: string } | null>(null);
  // バッファへシークできるなら、長く止めた後も、止めた位置からバッファで続けるので、読み直さない。
  const behind = useLiveLag(video, {
    reloadOnResume: route.kind === 'convert' && !timeshift.available,
    onReload: reload,
  });
  // バッファの範囲は、伸びたり、古いチャンクを破棄して縮んだりするので、1 分ごとに読み直す。
  // バッファを再生している間も、シークや読み直しのときに、起点を最新のバッファに移すので読み直す。
  const timeshiftAvailable = timeshift.available;
  useEffect(() => {
    if (!timeshiftAvailable) {
      return;
    }
    const timer = setInterval(onProgramEnd, 60_000);

    return () => clearInterval(timer);
  }, [timeshiftAvailable, onProgramEnd]);
  // 時間帯に重なる番組のうち、読み直した時刻に放送中のものと、その次のもの。
  const programs = data.guide[0]?.programs ?? [];
  const program =
    programs.find(
      (candidate) =>
        Date.parse(candidate.startAt) <= range.now &&
        (candidate.endUndecided || Date.parse(candidate.endAt) > range.now),
    ) ?? null;
  const nextProgram =
    program === null || program.endUndecided
      ? null
      : (programs.find((candidate) => Date.parse(candidate.startAt) >= Date.parse(program.endAt)) ??
        null);
  // 番組が終わる時刻に、放送中の番組を読み直す。終了時刻が未定か、番組がなければ、1 分ごとに読み直す。
  const programEnd = program === null || program.endUndecided ? null : program.endAt;
  useEffect(() => {
    const delay = programEnd === null ? undecidedRecheckMs : Date.parse(programEnd) - Date.now();
    const timer = setTimeout(onProgramEnd, Math.max(delay, 1000));

    return () => clearTimeout(timer);
  }, [programEnd, range, onProgramEnd]);
  // 音声は、経路ごとのメニューで選び、画質を変えても同じ音声を保つ。kyusu の経路では、番組の音声の構成から選ぶ。
  // バッファを再生している間に画質か音声を替えたら、それまでの位置から読み直す。
  const restartBehind = () => {
    if (timeshift.behind) {
      timeshift.restart(timeshift.currentPosition());
    }
  };
  const audio = usePlayerAudio(route, program?.audios ?? null, restartBehind);
  // 仮想サラウンド (#454)。5.1ch の番組を、ブラウザの中での変換で見ているときだけ使える。
  const surroundAvailable = route.kind === 'convert' && isSurroundAudio(program?.audios ?? null);
  useVirtualSurround(video, surroundAvailable);
  const programName = program?.name ?? null;
  // programTitle は、タブの題名や OS のメディアの表示に出す番組名。印を描けないので囲み文字を外し、全角の英数字は半角にする。
  const programTitle =
    programName === null ? null : toHalfWidth(splitProgramMarks(programName).name);
  const serviceName = data.node?.__typename === 'Service' ? data.node.name : '';
  const serviceLogoUrl = data.node?.__typename === 'Service' ? (data.node.logoUrl ?? null) : null;
  // flash は、キーで操作した結果。映像の上に短く出す (#448)。
  const [flash, showFlash] = usePlayerFlash();
  const capture = useCapture({
    crop,
    frameRef: playerBoxRef,
    videoRef,
    serviceName,
    programName,
    captureTime: () =>
      timeshift.behind ? timeshift.clockAt(timeshift.currentPosition()) : Date.now(),
    caption: () => currentCaption.current,
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
  // skip は、今の位置から seconds 秒だけ進むか戻る。ライブなら、ライブの端から数える。
  const skip = (seconds: number) =>
    timeshift.seekTo((timeshift.behind ? timeshift.currentPosition() : timeshift.edge) + seconds);
  const [autoPictureInPicture] = useAutoPictureInPicture();
  const service = data.node?.__typename === 'Service' ? data.node : null;
  // データ放送は、プリロードする設定か、一度出したあとは、出していない間も裏で受け取り、出したらすぐに d ボタンで入れるようにする
  // (設計書 7.4)。
  // 視聴が切れている間と、映像が見えていない間は、データ放送の TS も読まない。
  const dataBroadcast = useDataBroadcast(
    service !== null &&
      dataBroadcastAvailable &&
      (dataBroadcastPreload || dataBroadcastUsed) &&
      visible &&
      disconnection === null
      ? dataBroadcastSource(service.streamUrl)
      : null,
    videoRef,
    dataBroadcastEnabled,
    {
      // 文書が勝手に局を変えないよう、移る先を出して確かめてから移る (#440)。
      onTune: (networkId, sid) => {
        const target = data.services.find(
          (candidate) => candidate.networkId === networkId && candidate.serviceId === sid,
        );
        if (target === undefined) {
          return false;
        }
        if (target.id !== serviceId) {
          setTuneRequest({ id: target.id, name: toHalfWidth(target.name) });
        }

        return true;
      },
      // 音声の切り替えは、確かめずに従う。
      onSelectAudio: audio.selectComponent,
    },
  );
  // commands は、この画面で今使える操作 (#809)。キー、OS のメディア操作、ほかの端末からの操作は、どれもここから呼ぶ。
  const commands: PlayerCommands = {
    play: () => void videoRef.current?.play().catch(() => {}),
    pause: () => videoRef.current?.pause(),
    togglePlay: () => {
      const video = videoRef.current;
      if (video?.paused === true) {
        void video.play().catch(() => {});
      } else {
        video?.pause();
      }
    },
    // バッファへシークできるなら、録画と同じく、← と → で戻ったり進んだりする。データ放送を出している間は、データ放送の操作に譲る。
    ...(timeshift.available && !dataBroadcastEnabled
      ? {
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
        }
      : {}),
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
    // データ放送のない映像 (HD でないもの) では、出し入れできない。
    ...(dataBroadcastAvailable
      ? {
          toggleDataBroadcast: () => {
            setDataBroadcastEnabled(!dataBroadcastEnabled);
            showFlash(
              t(
                dataBroadcastEnabled
                  ? 'player.flash.dataBroadcastOff'
                  : 'player.flash.dataBroadcastOn',
              ),
            );
          },
        }
      : {}),
    // データ放送を出していなければ、d ボタンで出す。リモコンのほかのボタンは、出している間だけ渡す。
    pressKey: (key) => {
      if (!dataBroadcastEnabled) {
        if (key === RemoteKey.d) {
          setDataBroadcastEnabled(true);
        }

        return;
      }
      dataBroadcast.press(key);
    },
    // r で読み直す。映像が止まったり乱れたりしたときに、ページを読み直さずに直す (#499)。
    restart: () => {
      reload();
      showFlash(t('player.flash.restarted'));
    },
    toggleStats: () => setStatsShown(!statsShown),
    showShortcuts: () => setShortcutsOpen(true),
    setVolume: videoState.setVolume,
    setMuted: (muted) => {
      if (muted !== videoState.muted) {
        videoState.toggleMute();
      }
    },
    selectAudio: audio.remote.select,
    stop: () => void navigate({ to: '/live' }),
    // 同じ種別の前後の局へ移る (#493)。移る先がなければ、使えない。
    ...(neighbors.previous === null ? {} : { previousChannel: () => tune(neighbors.previous!.id) }),
    ...(neighbors.next === null ? {} : { nextChannel: () => tune(neighbors.next!.id) }),
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
  useDocumentTitle([programTitle, toHalfWidth(serviceName)]);
  useMediaSession(
    {
      title: programTitle ?? toHalfWidth(serviceName),
      artist: toHalfWidth(serviceName),
      artwork: serviceLogoUrl,
    },
    commands,
    // ライブはシークできないので、再生位置は知らせない。
    { playing: !videoState.paused, position: null },
    // 設定で選んでいれば、タブを切り替えたときに、別のウィンドウに出す (#492)。
    autoPictureInPicture,
  );

  // データ放送を出している間は、⌥+矢印、Enter、Backspace、数字、⌥+b/r/g/y でリモコンを押せる (#441)。
  useDataBroadcastKeys(
    dataBroadcastEnabled && dataBroadcast.state === 'shown' && !dataBroadcast.receiving,
    dataBroadcast.keyGroups,
    (key) => runCommand(commands, 'pressKey', key),
  );
  // 同じアカウントのほかの端末から、このプレイヤーを操作できるようにする (#459、設計書 7.7)。
  useRemotePlayer(
    video === null
      ? null
      : {
          kind: 'LIVE',
          target: serviceId,
          title: program?.name ?? null,
          paused: videoState.paused,
          position: null,
          duration: null,
          volume: videoState.volume,
          muted: videoState.muted,
          audios: audio.remote.choices,
          audio: audio.remote.selected,
          dataBroadcast: dataBroadcastEnabled,
          keyGroups: [...dataBroadcast.keyGroups],
        },
    commands,
  );
  // commentRows は、右の列のコメントの一覧の行。操作バーの出し入れなどで描き直すたびに作らず、コメントが届いたときだけ作り直す。
  const commentRows = useMemo(
    () =>
      jikkyo.comments.map((comment) => ({
        key: comment.key,
        label: formatClock(comment.at),
        content: comment.content,
        ...(comment.userId === undefined ? {} : { userId: comment.userId }),
      })),
    [jikkyo.comments],
  );

  if (service === null) {
    return <Alert color="amber">{t('live.notFound')}</Alert>;
  }
  // resumeBehind は、バッファの配信が切れたら、知らせを出さずに今の位置から読み直す (#1083)。続けて切れるなら、ライブに戻る。
  // 映像の要素が復号に失敗して止まったなら、壊れた所を飛び越えて読み直す。
  const resumeBehind = () => {
    const position = timeshift.positionRef.current;
    if (videoRef.current?.error != null) {
      const next = skipBrokenMedia(position, lastBrokenSkip.current);
      if (next !== null) {
        lastBrokenSkip.current = next;
        timeshift.restart(next);

        return;
      }
    }
    const at = Date.now();
    if (!canRetryImmediately(lastImmediateRetry.current, at)) {
      timeshift.goLive();

      return;
    }
    lastImmediateRetry.current = at;
    timeshift.restart(position);
  };
  // session を付けて、つなぎ直すたびに URL を変える。
  const withSession = (base: string) =>
    `${base}${base.includes('?') ? '&' : '?'}session=${session}`;
  const onLoaded = () => {
    attempt.current = 0;
    preempted.current = false;
    retryingImmediately.current = false;
  };

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      // バッファへシークできるなら、バッファの始まりからライブの端までのシークバーを出す (設計書 5.7)。
      seek={
        timeshift.available ? (
          <SeekBar
            bands={[]}
            duration={timeshift.edge}
            value={seeking ?? timeshift.position}
            previewUrl={null}
            label={(value) =>
              value >= timeshift.edge - liveThresholdSeconds
                ? t('player.live')
                : formatClock(timeshift.clockAt(value))
            }
            onChange={setSeeking}
            onChangeEnd={(value) => {
              setSeeking(null);
              timeshift.seekTo(value);
            }}
          />
        ) : undefined
      }
      start={
        timeshift.available ? (
          <>
            <PlayButton video={videoState} commands={commands} />
            <VolumeControl video={videoState} commands={commands} />
          </>
        ) : (
          <VolumeControl video={videoState} commands={commands} />
        )
      }
      status={
        timeshift.behind ? (
          <>
            <LiveStatus onCatchUp={timeshift.goLive} />
            {` · ${formatClock(timeshift.clockAt(timeshift.position))}`}
          </>
        ) : (
          <LiveStatus onCatchUp={behind ? reload : undefined} />
        )
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
            label={t('jikkyo.toggle')}
            icon={MessageSquareText}
            active={jikkyoEnabled}
          />
          <CommandButton
            commands={commands}
            command="toggleDataBroadcast"
            label={t('player.dataBroadcast')}
            icon={Tv}
            active={dataBroadcastEnabled}
            optional
          />
          <SettingsButton>
            <QualitySelect
              choices={choices}
              quality={quality}
              onChange={(next) => {
                restartBehind();
                setQuality(next);
              }}
            />
            {audio.menu}
            <CropSelect value={crop} onChange={setCrop} />
            <NormalizeVolumeToggle />
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
            dataBroadcast
          />
          <PipButton pip={pip} />
          <FullscreenButton active={fullscreen.active} commands={commands} />
        </>
      }
    />
  );

  // overlays は、全画面と別のウィンドウで、映像に重ねる切断の知らせとデータ放送のリモコン (#435)。
  const overlays = overlaid && (
    <>
      {disconnection !== null && (
        <PlayerAlert
          title={t('live.disconnected')}
          action={{ label: t('live.reconnectNow'), onClick: reconnectNow }}
        >
          <DisconnectionReason disconnection={disconnection} />
        </PlayerAlert>
      )}
      {dataBroadcastEnabled && (
        <DataBroadcastRemoteOverlay
          state={dataBroadcast.state}
          receiving={dataBroadcast.receiving}
          keyGroups={dataBroadcast.keyGroups}
          onPress={(key) => runCommand(commands, 'pressKey', key)}
        />
      )}
    </>
  );

  const playerBox = (
    <div
      ref={playerBoxRef}
      className={playerControlsClasses.box}
      data-controls-visible={controls.visible}
      {...controls.containerProps}
      // マウスで映像をダブルクリックすると、全画面を切り替える (#452)。データ放送を出している間は、データ放送の操作に譲る。
      onDoubleClick={(event) => {
        if (!dataBroadcastEnabled && onVideoSurface(event.target)) {
          fullscreen.toggle();
        }
      }}
    >
      {route.kind === 'convert' && (
        <TsPlayer
          url={
            timeshift.behind
              ? timeshift.stream.url
              : disconnection === null
                ? withSession(service.streamUrl)
                : null
          }
          // シークしたときは、すぐに読み直す。ライブのつなぎ直しは、裏で読み込んでから切り替える。
          handover={!timeshift.behind}
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          superimpose={captionSettings.superimposeLive}
          captionStyle={captionSettings}
          crop={crop}
          onCaptionText={onCaptionText}
          onLoaded={() => {
            if (timeshift.behind) {
              timeshift.stream.onLoaded();
            }
            onLoaded();
          }}
          overlay={
            <>
              {/* データ放送と実況のコメントは、ライブのものなので、バッファを再生している間は重ねない。 */}
              {!timeshift.behind && dataBroadcast.layer}
              {!timeshift.behind && jikkyo.overlay}
              {/* 番組名は映像の下に出すので、映像の下が見えない別のウィンドウでだけ重ねる。 */}
              {pip.container !== null && (
                <ProgramOverlay
                  visible={controls.visible}
                  serviceName={serviceName}
                  current={program}
                  next={nextProgram}
                />
              )}
              {flash !== null && <PlayerFlash>{flash}</PlayerFlash>}
              {statsShown && <PlayerStats video={video} quality={quality} />}
              {dataBroadcast.receiving && (
                <PlayerNotice loading>{t('player.dataBroadcastReceiving')}</PlayerNotice>
              )}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
              {videoLoading && !autoplay.blocked && disconnection === null && <PlayerLoading />}
              {overlays}
            </>
          }
          onState={(state) => {
            // ライブのストリームもバッファも終わらないので、読み終えたのは、切れたということ。
            if (state === 'completed' || state === 'error') {
              if (timeshift.behind) {
                resumeBehind();
              } else {
                disconnected();
              }
            }
          }}
          onError={timeshift.behind ? resumeBehind : disconnected}
          onAudio={audio.onAudio}
          audio={audio.convertAudio}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          lowLatency={lowLatency && !timeshift.behind}
          url={
            // バッファを再生している間は、起点にしているバッファの仮想 HLS を、読み直すたびに URL を変えて読む。
            timeshift.behind && timeshift.frame !== null
              ? `${hlsURL(timeshift.frame.hlsUrl, route.preset, audio.kyusuAudio)}#${timeshift.hlsLoads}`
              : disconnection === null
                ? withSession(hlsURL(service.streamUrl, route.preset, audio.kyusuAudio))
                : null
          }
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          superimpose={captionSettings.superimposeLive}
          captionStyle={captionSettings}
          crop={crop}
          onCaptionText={onCaptionText}
          live={!timeshift.behind}
          startPosition={timeshift.hlsStart}
          onLoaded={onLoaded}
          overlay={
            <>
              {/* データ放送と実況のコメントは、ライブのものなので、バッファを再生している間は重ねない。 */}
              {!timeshift.behind && dataBroadcast.layer}
              {!timeshift.behind && jikkyo.overlay}
              {/* 番組名は映像の下に出すので、映像の下が見えない別のウィンドウでだけ重ねる。 */}
              {pip.container !== null && (
                <ProgramOverlay
                  visible={controls.visible}
                  serviceName={serviceName}
                  current={program}
                  next={nextProgram}
                />
              )}
              {flash !== null && <PlayerFlash>{flash}</PlayerFlash>}
              {statsShown && <PlayerStats video={video} quality={quality} />}
              {dataBroadcast.receiving && (
                <PlayerNotice loading>{t('player.dataBroadcastReceiving')}</PlayerNotice>
              )}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
              {videoLoading && !autoplay.blocked && disconnection === null && <PlayerLoading />}
              {overlays}
            </>
          }
          // チューナーを使われて切れると、hls.js が読み直しを諦めたところで、致命的なエラーになる。
          onError={timeshift.behind ? resumeBehind : disconnected}
        />
      )}
    </div>
  );

  return (
    <>
      <Modal
        opened={tuneRequest !== null}
        onClose={() => setTuneRequest(null)}
        title={t('live.tuneRequest.title')}
      >
        <Stack>
          <Text size="sm">{t('live.tuneRequest.body', { name: tuneRequest?.name ?? '' })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setTuneRequest(null)}>
              {t('live.tuneRequest.stay')}
            </Button>
            <Button
              onClick={() => {
                if (tuneRequest !== null) {
                  tune(tuneRequest.id);
                }
                setTuneRequest(null);
              }}
            >
              {t('live.tuneRequest.go')}
            </Button>
          </Group>
        </Stack>
      </Modal>
      {/* VLC や mpv で開く URL。開いている間は、ブラウザの視聴とは別にチューナーを使う (#487)。 */}
      <Modal
        opened={externalOpen}
        onClose={() => setExternalOpen(false)}
        title={t('streamURL.open')}
      >
        <ExternalStreamURL target={serviceId} live />
      </Modal>
      <WatchLayout
        player={
          <Stack gap="xs">
            <PlaybackUnavailable route={route} />
            {/* 画面の外に出たときと、タブが隠れたときも再生は続け、データ放送の受信と、実況のコメントの描画だけを止める (設計書 7.2)。 */}
            <div ref={frameRef}>
              {pip.container === null ? playerBox : <PipPlaceholder onBack={pip.close} />}
            </div>
            {pip.container !== null && createPortal(playerBox, pip.container)}
          </Stack>
        }
        info={
          <>
            <WatchHeading
              title={
                programName === null ? t('live.noProgram') : <ProgramTitle name={programName} />
              }
              serviceName={service.name}
              serviceLogoUrl={service.logoUrl}
              time={program}
              // 使うことの少ない操作は、メニューにまとめる (#487)。
              actions={
                <Menu position="bottom-end">
                  <Menu.Target>
                    <ActionIcon variant="default" size="md" aria-label={t('live.moreActions')}>
                      <EllipsisVertical size={16} aria-hidden />
                    </ActionIcon>
                  </Menu.Target>
                  <Menu.Dropdown>
                    <Menu.Item
                      leftSection={<ExternalLink size={14} aria-hidden />}
                      onClick={() => setExternalOpen(true)}
                    >
                      {t('streamURL.open')}
                    </Menu.Item>
                    {service.timeshiftPrograms.length > 0 && (
                      <Menu.Item
                        leftSection={<History size={14} aria-hidden />}
                        renderRoot={(props) => (
                          <Link {...props} to="/timeshift/$serviceId" params={{ serviceId }} />
                        )}
                      >
                        {t('timeshift.list')}
                      </Menu.Item>
                    )}
                  </Menu.Dropdown>
                </Menu>
              }
            />
            {disconnection !== null && (
              <Alert color="amber" role="alert" title={t('live.disconnected')}>
                <Stack gap="xs" align="flex-start">
                  <DisconnectionReason disconnection={disconnection} />
                  <Button size="compact-sm" variant="light" color="amber" onClick={reconnectNow}>
                    {t('live.reconnectNow')}
                  </Button>
                </Stack>
              </Alert>
            )}
            {dataBroadcastEnabled && (
              <DataBroadcastRemote
                state={dataBroadcast.state}
                receiving={dataBroadcast.receiving}
                keyGroups={dataBroadcast.keyGroups}
                onPress={(key) => runCommand(commands, 'pressKey', key)}
              />
            )}
          </>
        }
        side={
          <>
            {/* 実況のコメントの一覧と字幕の履歴を、見出しで切り替える。実況を切っていれば、字幕の履歴だけを出す。 */}
            <WatchPanel
              label={t('player.sidePanel')}
              tabs={[
                ...(channel !== null && jikkyoEnabled
                  ? [
                      {
                        value: 'jikkyo',
                        label: t('jikkyo.toggle'),
                        content: (
                          <CommentList
                            rows={commentRows}
                            onMuteUser={muteUser}
                            height={watchCommentsHeight}
                          />
                        ),
                        footer: (
                          <JikkyoPanel
                            channelName={channel.name}
                            status={jikkyo.status}
                            onPost={jikkyo.post}
                            force={jikkyo.force}
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
                      label={(entry) => formatClock(entry.at)}
                      height={watchCommentsHeight}
                    />
                  ),
                },
              ]}
            />
            <LiveServiceList serviceId={serviceId} />
          </>
        }
      />
    </>
  );
}
