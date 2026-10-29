import type { PlayerState } from '@mpeg2toh264/player';
import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Captions, Keyboard } from 'lucide-react';
import { Suspense, useEffect, useRef, useState } from 'react';
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
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { WatchPanel } from '../../yunomi/WatchPanel';
import { WatchHeading, WatchLayout, watchCommentsHeight } from '../../yunomi/WatchLayout';
import { useVolumeNormalizer } from '../../yunomi/volumeNormalize';
import { noCrop, type Crop } from '../../yunomi/crop';
import { CaptionHistory } from '../../yunomi/CaptionHistory';
import {
  insertRecordedCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../yunomi/captionHistory';
import { PipPlaceholder } from '../../yunomi/PipPlaceholder';
import {
  broadcastTime,
  recordingPosition,
  formatClock,
  formatDuration,
} from '../../yunomi/position';
import { useSeekableStream } from '../../yunomi/useSeekableStream';
import { RateSelect, CropSelect, NormalizeVolumeToggle } from '../../yunomi/QualitySelect';
import { toVideoFormat } from '../../yunomi/route';
import { SeekBar } from '../../yunomi/SeekBar';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useMediaSession } from '../../yunomi/useMediaSession';
import type { ShortcutAction } from '../../yunomi/shortcuts';
import { ShortcutsHelp } from '../../yunomi/ShortcutsHelp';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import {
  CaptureButton,
  openPictureInPicture,
  PipButton,
  useCapture,
} from '../../yunomi/PlayerTools';
import { PlayerStats, StatsToggle } from '../../yunomi/PlayerStats';
import {
  PlayerAlert,
  BigPlayButton,
  PlayerLoading,
  ControlButton,
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
import { useAutoPictureInPicture } from '../../yunomi/autoPictureInPicture';
import { useVideoLoading } from '../../yunomi/useVideoLoading';
import { useVideoState } from '../../yunomi/useVideoState';
import { stepVolume } from '../../yunomi/volume';
import { currentVolume } from '../../yunomi/volumeBoost';
import { useWakeLock } from '../../yunomi/useWakeLock';
import { useDocumentPictureInPicture } from '../../yunomi/useDocumentPictureInPicture';
import { useStreamsDrains } from '../../changes/DataChanges';
import { canRetryImmediately, endedEarly, skipBrokenMedia } from '../../yunomi/reconnect';
import type { ServiceIdPlayTimeshiftQuery } from './__generated__/ServiceIdPlayTimeshiftQuery.graphql';

// volumeStep は、↑ と ↓ で音量を上げ下げする幅。
const volumeStep = 0.05;

// PlaySearch は、再生の画面の URL のクエリ。program は、再生する番組の始まりの時刻 (RFC 3339)。
type PlaySearch = { program: string };

export const Route = createFileRoute('/timeshift/$serviceId_/play')({
  // 右の列を置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => ({
    program: typeof search.program === 'string' ? search.program : '',
  }),
  component: PlayPage,
});

// PlayPage は、タイムシフトのバッファにある番組を、頭から再生する画面 (設計書 5.7)。
// バッファは、ブラウザの中で変換する経路だけで再生する。
function PlayPage() {
  const { t } = useTranslation();
  const { serviceId } = Route.useParams();
  const { program } = Route.useSearch();

  return (
    <Stack gap="sm">
      <Anchor
        size="sm"
        renderRoot={(props) => <Link {...props} to="/live/$serviceId" params={{ serviceId }} />}
      >
        {t('timeshift.backToLive')}
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
                ? t('timeshift.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Player key={`${serviceId}:${program}`} serviceId={serviceId} startAt={program} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// playQuery は、再生する番組。放送中は、番組の終わりと途切れた区間を知るために読み直す。
const playQuery = graphql`
  query ServiceIdPlayTimeshiftQuery($serviceId: ID!, $startAt: Time!) {
    timeshiftProgram(serviceId: $serviceId, startAt: $startAt) {
      name
      startAt
      endAt
      playableFrom
      gaps {
        from
        to
      }
      streamUrl
      service {
        name
        logoUrl
        videoFormat
      }
    }
  }
`;

// refreshMs は、放送中の番組を再生している間に、番組を読み直す間隔。終わった時刻が分かったら、長さを伸ばすのをやめる。
const refreshMs = 30_000;

function Player({ serviceId, startAt }: { serviceId: string; startAt: string }) {
  const { t } = useTranslation();
  const variables = { serviceId, startAt };
  const { timeshiftProgram: program } = useLazyLoadQuery<ServiceIdPlayTimeshiftQuery>(
    playQuery,
    variables,
  );
  const videoRef = useRef<HTMLVideoElement | null>(null);
  // 別のウィンドウに移したり戻したりすると、プレイヤーを作り直すので、今の位置から読み直す。
  const pip = useDocumentPictureInPicture(() => restartAt(positionRef.current));
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoState = useVideoState(video);
  const autoplay = useAutoplayBlocked(video);
  const videoLoading = useVideoLoading(video);
  const fullscreen = useFullscreen(playerBoxRef);
  const overlaid = fullscreen.active || pip.container !== null;
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const controls = useControlsVisibility({ playing: !videoState.paused && !shortcutsOpen });
  useWakeLock(!videoState.paused);
  // バッファは kyusu で変換しないので、ブラウザの中で変換できる端末と番組だけで再生する。
  const { route } = usePlaybackRoute(
    { available: false, presets: [] },
    toVideoFormat(program.service.videoFormat),
  );
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    captions: captionSettings,
  } = usePlayerSettings();
  // 放送中なら、番組が終わるまで読み直す。
  const environment = useRelayEnvironment();
  const onAir = program.endAt == null;
  useEffect(() => {
    if (!onAir) {
      return;
    }
    const timer = setInterval(() => {
      fetchQuery<ServiceIdPlayTimeshiftQuery>(environment, playQuery, variables, {
        fetchPolicy: 'network-only',
      })
        .toPromise()
        .catch(() => {});
    }, refreshMs);

    return () => clearInterval(timer);
    // variables は描画ごとに作り直すが、中身は serviceId と startAt だけで決まる。
  }, [environment, onAir, serviceId, startAt]);
  // duration は、再生できる長さ (秒)。途切れた区間の分は、バッファに映像がないので引く。放送中は今まで伸ばす。
  const duration = recordingPosition(
    program.playableFrom,
    program.gaps,
    program.endAt == null ? Date.now() : Date.parse(program.endAt),
  );
  // stream は、再生できる範囲の始まりからの秒で、位置を指定して読み直す配信 (設計書 7.2)。
  const stream = useSeekableStream(program.streamUrl, video, 0);
  const { position, positionRef, currentPosition } = stream;
  const [seeking, setSeeking] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const lastAutoRetry = useRef<number | null>(null);
  // lastBrokenSkip は、壊れた所を最後に飛び越えた先の位置。
  const lastBrokenSkip = useRef<number | null>(null);
  // handover は、次の読み直しで、今の映像を流したまま裏で読み込んで切り替えるかどうか (#1085)。
  const [handover, setHandover] = useState(false);

  const [statsShown, setStatsShown] = useState(false);
  useVolumeNormalizer(video);
  const [crop, setCrop] = useState<Crop>(noCrop);
  // restartAt は、指定の位置から読み直す。
  const restartAt = (seconds: number) => {
    setHandover(false);
    stream.restartAt(seconds);
  };
  // tenban が止まる前に知らせてきたら、今の位置から、新しい接続を張ってから切り替える (#1085)。
  const drains = useStreamsDrains();
  const seenDrains = useRef(drains);
  useEffect(() => {
    if (drains === seenDrains.current) {
      return;
    }
    seenDrains.current = drains;
    if (error === null) {
      restartAt(positionRef.current);
      setHandover(true);
    }
    // 読み直しの位置は ref から読むので、知らせが届いたときだけ動かす。
  }, [drains, error]);
  const retry = () => {
    setError(null);
    restartAt(positionRef.current);
  };
  // captionEntries は、字幕の履歴 (#450)。字幕が出たときの再生位置で並べ、行を押すとその位置へ移る。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  const currentCaption = useRef<string | null>(null);
  const onCaptionText = (text: string) => {
    const normalized = normalizeCaption(text);
    currentCaption.current = normalized === '' ? null : normalized;
    const at = currentPosition();
    setCaptionEntries((list) => insertRecordedCaption(list, at, text));
  };
  // seek は、指定の位置に移る。再生できる範囲の外へは移らない。
  const seek = (target: number) => restartAt(Math.min(Math.max(target, 0), duration));
  const [rate, setRate] = useState(1);
  useEffect(() => {
    if (video !== null) {
      video.defaultPlaybackRate = rate;
      video.playbackRate = rate;
    }
  }, [rate, video, stream.url]);

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
  const skip = (seconds: number) => seek(currentPosition() + seconds);
  const [flash, showFlash] = usePlayerFlash();
  const capture = useCapture({
    crop,
    frameRef: playerBoxRef,
    videoRef,
    serviceName: program.service.name,
    programName: program.name,
    captureTime: () => broadcastTime(program.playableFrom, program.gaps, currentPosition()),
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
    restart: () => {
      retry();
      showFlash(t('player.flash.restarted'));
    },
    toggleStats: () => setStatsShown(!statsShown),
    showShortcuts: () => setShortcutsOpen(true),
  } satisfies Partial<Record<ShortcutAction, () => void>>;
  usePlayerShortcuts(shortcuts);
  const programTitle = toHalfWidth(splitProgramMarks(program.name).name);
  useDocumentTitle([programTitle === '' ? null : programTitle]);
  const [autoPictureInPicture] = useAutoPictureInPicture();
  useMediaSession(
    { title: programTitle, artist: toHalfWidth(program.service.name), artwork: null },
    {
      play: () => void videoRef.current?.play().catch(() => {}),
      pause: () => videoRef.current?.pause(),
      seekBackward: () => skip(-10),
      seekForward: () => skip(10),
      seekTo: (seconds) => seek(seconds),
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
    {
      playing: !videoState.paused,
      position: { duration, position: seeking ?? position, playbackRate: rate },
    },
  );

  // resumeAfterCut は、配信が切れたら、知らせを出さずに今の位置から読み直す (#1083)。続けて切れるなら、エラーを出して利用者に任せる。
  const resumeAfterCut = () => {
    // 映像の要素が復号に失敗して止まったなら、壊れた所を飛び越えて読み直す。
    if (videoRef.current?.error != null) {
      const next = skipBrokenMedia(positionRef.current, lastBrokenSkip.current);
      if (next !== null) {
        lastBrokenSkip.current = next;
        restartAt(next);

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
  // onConvertState は、配信が番組の終わりに届く前に終わったら、途中で切れたとみなして読み直す。
  const onConvertState = (state: PlayerState) => {
    const video = videoRef.current;
    if (state !== 'completed' || video === null) {
      return;
    }
    const { buffered } = video;
    const end =
      buffered.length === 0
        ? positionRef.current
        : stream.positionOf(buffered.end(buffered.length - 1));
    if (endedEarly(end, duration)) {
      resumeAfterCut();
    }
  };
  // 番組が終わった位置まで再生したら、次の番組に進まずに止める。
  useEffect(() => {
    if (!onAir && position >= duration && video !== null && !video.paused) {
      video.pause();
    }
  }, [onAir, position, duration, video]);
  const errorAlert = overlaid && error !== null && (
    <PlayerAlert title={error} action={{ label: t('player.retry'), onClick: retry }} />
  );
  const shown = seeking ?? position;
  // gapMarks は、バッファが途切れた位置の印。途切れた区間はシークバーの長さに含めないので、長さのない印にする。
  const gapMarks = program.gaps.map((gap) => {
    const at = recordingPosition(program.playableFrom, program.gaps, Date.parse(gap.from));
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
          previewUrl={null}
          label={(value) =>
            `${formatDuration(value)} (${formatClock(broadcastTime(program.playableFrom, program.gaps, value))})`
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
          <PlayButton video={videoState} />
          <VolumeControl video={videoState} />
        </>
      }
      status={`${formatDuration(shown)} / ${formatDuration(duration)} · ${formatClock(broadcastTime(program.playableFrom, program.gaps, shown))}`}
      end={
        <>
          <ControlButton
            label={t('player.captions')}
            icon={Captions}
            active={captions}
            onClick={() => setCaptions(!captions)}
          />
          <SettingsButton>
            <RateSelect value={rate} onChange={setRate} />
            <CropSelect value={crop} onChange={setCrop} />
            <NormalizeVolumeToggle />
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
          />
          <PipButton pip={pip} />
          <FullscreenButton active={fullscreen.active} onToggle={fullscreen.toggle} />
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
      <TsPlayer
        url={stream.url}
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
          stream.onLoaded();
          setError(null);
        }}
        onError={resumeAfterCut}
        onState={onConvertState}
        overlay={
          <>
            {flash !== null && <PlayerFlash>{flash}</PlayerFlash>}
            {statsShown && <PlayerStats video={video} quality={null} />}
            {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
            {videoLoading && !autoplay.blocked && error === null && <PlayerLoading />}
            {errorAlert}
          </>
        }
      />
    </div>
  );

  return (
    <WatchLayout
      player={
        route.kind === 'convert' ? (
          <div>
            {pip.container === null ? playerBox : <PipPlaceholder onBack={pip.close} />}
            {pip.container !== null && createPortal(playerBox, pip.container)}
          </div>
        ) : (
          <Alert color="amber" role="alert" title={t('player.unavailable.title')}>
            {t('timeshift.convertOnly')}
          </Alert>
        )
      }
      info={
        <>
          <WatchHeading
            title={
              program.name === '' ? t('timeshift.untitled') : <ProgramTitle name={program.name} />
            }
            serviceName={program.service.name}
            serviceLogoUrl={program.service.logoUrl}
            time={{
              startAt: program.startAt,
              endAt: program.endAt ?? program.startAt,
              endUndecided: program.endAt == null,
            }}
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
        <WatchPanel
          label={t('player.sidePanel')}
          tabs={[
            {
              value: 'captions',
              label: t('captionHistory.tab'),
              content: (
                <CaptionHistory
                  entries={captionEntries}
                  label={(entry) =>
                    formatClock(broadcastTime(program.playableFrom, program.gaps, entry.at))
                  }
                  onSelect={(entry) => seek(entry.at)}
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
