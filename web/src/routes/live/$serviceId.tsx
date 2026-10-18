import {
  ActionIcon,
  Alert,
  Anchor,
  Button,
  Group,
  Menu,
  Modal,
  Select,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import {
  Captions,
  EllipsisVertical,
  ExternalLink,
  Keyboard,
  MessageSquareText,
  Tv,
} from 'lucide-react';
import { startTransition, Suspense, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import {
  ErrorBoundary,
  PageLoader,
  ProgramTitle,
  splitProgramMarks,
  toHalfWidth,
  useDocumentTitle,
} from '../../futon';
import { CommentListToggle } from '../../jikkyo/CommentList';
import { CaptionHistoryToggle } from '../../yunomi/CaptionHistory';
import { appendLiveCaption, type CaptionEntry } from '../../yunomi/captionHistory';
import { formatClock } from '../../yunomi/position';
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
  ControlButton,
  FullscreenButton,
  LiveStatus,
  PlayerAlert,
  PlayerControls,
  playerControlsClasses,
  PlayerNotice,
  SettingsButton,
  VolumeControl,
  PlayerFlash,
  usePlayerFlash,
} from '../../yunomi/PlayerControls';
import { PlaybackUnavailable, QualitySelect } from '../../yunomi/QualitySelect';
import { reconnectDelay, recordingsThatTookTuner } from '../../yunomi/reconnect';
import { dataBroadcastSource, hlsURL } from '../../yunomi/route';
import { useDataBroadcast } from '../../yunomi/useDataBroadcast';
import { useDataBroadcastPreload } from '../../yunomi/dataBroadcastPreload';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useMediaSession } from '../../yunomi/useMediaSession';
import type { ShortcutAction } from '../../yunomi/shortcuts';
import { ExternalStreamURL } from '../../streams/ExternalStreamURL';
import { ShortcutsHelp } from '../../yunomi/ShortcutsHelp';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
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
import { ServiceLabel } from '../../guide/ServiceLabel';

// volumeStep は、↑ と ↓ で音量を上げ下げする幅。
const volumeStep = 0.05;

// channelGroups は、選局の欄の選択肢を、チャンネル種別ごとにまとめて返す。
function channelGroups(
  services: readonly {
    id: string;
    name: string;
    channelType: { type: string; displayName: string };
  }[],
) {
  const groups: { group: string; items: { value: string; label: string }[] }[] = [];
  for (const service of services) {
    let group = groups.find((candidate) => candidate.group === service.channelType.displayName);
    if (group === undefined) {
      group = { group: service.channelType.displayName, items: [] };
      groups.push(group);
    }
    group.items.push({ value: service.id, label: toHalfWidth(service.name) });
  }

  return groups;
}

export const Route = createFileRoute('/live/$serviceId')({
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
            jikkyo {
              id
              name
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
          }
        }
        # 視聴画面の中で選局するための、番組表と同じ並びのサービス (#493)。
        services {
          id
          name
          networkId
          serviceId
          channelType {
            type
            displayName
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
  const { route, choices, quality, setQuality } = usePlaybackRoute(data.transcoding);
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    setJikkyoEnabled,
    comments: commentSettings,
    muteUser,
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

  // 視聴が切れたら、理由を調べ、間隔を延ばしながらつなぎ直す。
  const disconnected = () => {
    if (disconnection !== null) {
      return;
    }
    const at = Date.now();
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
  // statsShown は、再生の統計情報を映像の上に出しているかどうか (#499)。
  const [statsShown, setStatsShown] = useState(false);
  // captionEntries は、字幕の履歴 (#450)。局を変えたら、画面ごと作り直すので捨てる。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  const onCaptionText = (text: string) =>
    setCaptionEntries((list) => appendLiveCaption(list, Date.now(), text));
  // externalOpen は、外部プレイヤー用の URL を発行するダイアログを出しているかどうか (#487)。
  const [externalOpen, setExternalOpen] = useState(false);
  // tuneRequest は、データ放送の文書が選局しようとしたサービス。確かめてから移る (#440)。
  const [tuneRequest, setTuneRequest] = useState<{ id: string; name: string } | null>(null);
  const behind = useLiveLag(video, { reloadOnResume: route.kind === 'convert', onReload: reload });
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
  const audio = usePlayerAudio(route, program?.audios ?? null);
  const programName = program?.name ?? null;
  const serviceName = data.node?.__typename === 'Service' ? data.node.name : '';
  const serviceLogoUrl = data.node?.__typename === 'Service' ? (data.node.logoUrl ?? null) : null;
  // flash は、キーで操作した結果。映像の上に短く出す (#448)。
  const [flash, showFlash] = usePlayerFlash();
  const capture = useCapture({
    frameRef: playerBoxRef,
    videoRef,
    serviceName,
    programName,
    captureTime: Date.now,
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
  const shortcuts = {
    togglePlay: () => {
      const video = videoRef.current;
      if (video?.paused === true) {
        void video.play().catch(() => {});
      } else {
        video?.pause();
      }
    },
    toggleCaptions: () => setCaptions(!captions),
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
    toggleDataBroadcast: () => {
      setDataBroadcastEnabled(!dataBroadcastEnabled);
      showFlash(
        t(dataBroadcastEnabled ? 'player.flash.dataBroadcastOff' : 'player.flash.dataBroadcastOn'),
      );
    },
    // r で読み直す。映像が止まったり乱れたりしたときに、ページを読み直さずに直す (#499)。
    restart: () => {
      reload();
      showFlash(t('player.flash.restarted'));
    },
    toggleStats: () => setStatsShown(!statsShown),
    showShortcuts: () => setShortcutsOpen(true),
  } satisfies Partial<Record<ShortcutAction, () => void>>;
  usePlayerShortcuts(shortcuts);
  useDocumentTitle([programName, serviceName]);
  const [autoPictureInPicture] = useAutoPictureInPicture();
  useMediaSession(
    { title: programName ?? serviceName, artist: serviceName, artwork: serviceLogoUrl },
    {
      play: () => void videoRef.current?.play().catch(() => {}),
      pause: () => videoRef.current?.pause(),
      // OS の前と次のボタンで、同じ種別の前後の局へ移る (#493)。移る先がなければ、OS にボタンを出さない。
      ...(neighbors.previous === null ? {} : { previousTrack: () => tune(neighbors.previous!.id) }),
      ...(neighbors.next === null ? {} : { nextTrack: () => tune(neighbors.next!.id) }),
      // 設定で選んでいれば、タブを切り替えたときに、別のウィンドウに出す (#492)。
      ...(autoPictureInPicture && pip.supported
        ? {
            enterPictureInPicture: () => {
              if (pip.container === null) {
                openPictureInPicture(pip).catch(() => {});
              }
            },
          }
        : {}),
    },
    // ライブはシークできないので、再生位置は知らせない。
    { playing: !videoState.paused, position: null },
  );

  const service = data.node?.__typename === 'Service' ? data.node : null;
  // データ放送は、プリロードする設定か、一度出したあとは、出していない間も裏で受け取り、出したらすぐに d ボタンで入れるようにする
  // (設計書 7.4)。
  // 視聴が切れている間と、映像が見えていない間は、データ放送の TS も読まない。
  const dataBroadcast = useDataBroadcast(
    service !== null &&
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
          setTuneRequest({ id: target.id, name: target.name });
        }

        return true;
      },
      // 音声の切り替えは、確かめずに従う。
      onSelectAudio: audio.selectComponent,
    },
  );
  // データ放送を出している間は、⌥+矢印、Enter、Backspace、数字、⌥+b/r/g/y でリモコンを押せる (#441)。
  useDataBroadcastKeys(
    dataBroadcastEnabled && dataBroadcast.state === 'shown' && !dataBroadcast.receiving,
    dataBroadcast.keyGroups,
    dataBroadcast.press,
  );
  if (service === null) {
    return <Alert color="amber">{t('live.notFound')}</Alert>;
  }
  // session を付けて、つなぎ直すたびに URL を変える。
  const withSession = (base: string) =>
    `${base}${base.includes('?') ? '&' : '?'}session=${session}`;
  const onLoaded = () => {
    attempt.current = 0;
    preempted.current = false;
  };

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      start={<VolumeControl video={videoState} />}
      status={<LiveStatus onCatchUp={behind ? reload : undefined} />}
      end={
        <>
          <ControlButton
            label={t('player.captions')}
            icon={Captions}
            active={captions}
            onClick={() => setCaptions(!captions)}
          />
          {channel !== null && (
            <ControlButton
              label={t('jikkyo.toggle')}
              icon={MessageSquareText}
              active={jikkyoEnabled}
              onClick={() => setJikkyoEnabled(!jikkyoEnabled)}
            />
          )}
          <ControlButton
            label={t('player.dataBroadcast')}
            icon={Tv}
            active={dataBroadcastEnabled}
            optional
            onClick={() => setDataBroadcastEnabled(!dataBroadcastEnabled)}
          />
          <SettingsButton>
            <QualitySelect choices={choices} quality={quality} onChange={setQuality} />
            {audio.menu}
            <StatsToggle shown={statsShown} onChange={setStatsShown} />
          </SettingsButton>
          <CaptureButton capture={capture} />
          <ControlButton
            label={t('player.shortcuts.title')}
            icon={Keyboard}
            optional
            onClick={() => setShortcutsOpen(true)}
          />
          <ShortcutsHelp
            opened={shortcutsOpen}
            onClose={() => setShortcutsOpen(false)}
            actions={Object.keys(shortcuts) as ShortcutAction[]}
            target={playerBoxRef}
            dataBroadcast
          />
          <PipButton pip={pip} />
          <FullscreenButton active={fullscreen.active} onToggle={fullscreen.toggle} />
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
          onPress={dataBroadcast.press}
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
          url={disconnection === null ? withSession(service.streamUrl) : null}
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          onCaptionText={onCaptionText}
          onLoaded={onLoaded}
          overlay={
            <>
              {dataBroadcast.layer}
              {jikkyo.overlay}
              <ProgramOverlay
                visible={controls.visible}
                serviceName={serviceName}
                current={program}
                next={nextProgram}
              />
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
            // ライブのストリームは終わらないので、読み終えたのは、切れたということ。
            if (state === 'completed' || state === 'error') {
              disconnected();
            }
          }}
          onError={disconnected}
          onAudio={audio.onAudio}
          audio={audio.convertAudio}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          url={
            disconnection === null
              ? withSession(hlsURL(service.streamUrl, route.preset, audio.kyusuAudio))
              : null
          }
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          onCaptionText={onCaptionText}
          live
          onLoaded={onLoaded}
          overlay={
            <>
              {dataBroadcast.layer}
              {jikkyo.overlay}
              <ProgramOverlay
                visible={controls.visible}
                serviceName={serviceName}
                current={program}
                next={nextProgram}
              />
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
          onError={disconnected}
        />
      )}
    </div>
  );

  return (
    <Stack gap="xs">
      <Group justify="space-between" wrap="wrap" gap="xs">
        <Title order={2} size="h4">
          <ServiceLabel name={service.name} logoUrl={service.logoUrl} />
        </Title>
        {/* 一覧に戻らずに、ほかの局へ移る (#493)。種別ごとにまとめ、番組表と同じ順に並べる。 */}
        <Group gap="xs" wrap="nowrap">
          <Select
            size="xs"
            w={240}
            aria-label={t('live.switchChannel')}
            placeholder={t('live.switchChannel')}
            data={channelGroups(data.services)}
            value={serviceId}
            onChange={(id) => {
              if (id !== null && id !== serviceId) {
                tune(id);
              }
            }}
            searchable
            allowDeselect={false}
          />
          {/* 使うことの少ない操作は、メニューにまとめる (#487)。 */}
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
            </Menu.Dropdown>
          </Menu>
        </Group>
      </Group>
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
      <Text size="sm" c="dimmed">
        {programName === null ? t('live.noProgram') : <ProgramTitle name={programName} />}
      </Text>
      <PlaybackUnavailable route={route} />
      {/* 画面の外に出たときと、タブが隠れたときも再生は続け、データ放送の受信と、実況のコメントの描画だけを止める (設計書 7.2)。 */}
      <div ref={frameRef}>
        {pip.container === null ? playerBox : <PipPlaceholder onBack={pip.close} />}
      </div>
      {pip.container !== null && createPortal(playerBox, pip.container)}
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
          onPress={dataBroadcast.press}
        />
      )}
      <CaptionHistoryToggle entries={captionEntries} label={(entry) => formatClock(entry.at)} />
      {channel !== null && jikkyoEnabled && (
        <>
          <JikkyoPanel channelName={channel.name} status={jikkyo.status} onPost={jikkyo.post} />
          <CommentListToggle
            rows={() =>
              jikkyo.comments.map((comment) => ({
                key: comment.key,
                label: formatClock(comment.at),
                content: comment.content,
                ...(comment.userId === undefined ? {} : { userId: comment.userId }),
              }))
            }
            onMuteUser={muteUser}
          />
        </>
      )}
    </Stack>
  );
}
