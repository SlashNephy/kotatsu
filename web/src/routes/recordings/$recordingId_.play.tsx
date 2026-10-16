import { Alert, Anchor, Slider, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Captions, MessageSquareText } from 'lucide-react';
import { Suspense, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary, PageLoader, ProgramTitle, useDocumentTitle } from '../../futon';
import { useRecordedJikkyo } from '../../jikkyo/useRecordedJikkyo';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { HlsPlayer } from '../../yunomi/HlsPlayer';
import { PipPlaceholder } from '../../yunomi/PipPlaceholder';
import {
  broadcastTime,
  formatClock,
  formatDuration,
  playbackPosition,
} from '../../yunomi/position';
import { RateSelect, PlaybackUnavailable, QualitySelect } from '../../yunomi/QualitySelect';
import { hlsURL } from '../../yunomi/route';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useWatchPositionSaver } from '../../yunomi/useWatchPositionSaver';
import { useMediaSession } from '../../yunomi/useMediaSession';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import { CaptureButton, PipButton } from '../../yunomi/PlayerTools';
import {
  BigPlayButton,
  ControlButton,
  FullscreenButton,
  PlayButton,
  PlayerControls,
  playerControlsClasses,
  SettingsButton,
  VolumeControl,
} from '../../yunomi/PlayerControls';
import { controlsAttribute, useControlsVisibility } from '../../yunomi/useControlsVisibility';
import { useFullscreen } from '../../yunomi/useFullscreen';
import { useAutoplayBlocked } from '../../yunomi/useAutoplayBlocked';
import { useVideoState } from '../../yunomi/useVideoState';
import { useDocumentPictureInPicture } from '../../yunomi/useDocumentPictureInPicture';
import { resumePosition } from '../../yunomi/resume';
import { useVisible } from '../../yunomi/useVisible';
import type { RecordingIdPlayQuery } from './__generated__/RecordingIdPlayQuery.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';

export const Route = createFileRoute('/recordings/$recordingId_/play')({
  component: PlayPage,
});

// PlayPage は、録画を再生する画面。ブラウザの中で変換するか、kyusu の HLS で再生する (設計書 7.1)。
function PlayPage() {
  const { t } = useTranslation();
  const { recordingId } = Route.useParams();

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
          <Player key={recordingId} recordingId={recordingId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Player({ recordingId }: { recordingId: string }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<RecordingIdPlayQuery>(
    graphql`
      query RecordingIdPlayQuery($id: ID!) {
        node(id: $id) {
          __typename
          ... on Recording {
            name
            streamUrl
            recordedFrom
            recordedUntil
            gaps {
              from
              to
            }
            jikkyoCommentsUrl
            watchPositionMs
            thumbnailUrl
            audios {
              componentType
              languages
            }
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
          }
        }
      }
    `,
    { id: recordingId },
  );
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
  // ブラウザに音ありの自動再生を断られたら、映像の真ん中に再生ボタンを出す。
  const autoplay = useAutoplayBlocked(video);
  const fullscreen = useFullscreen(playerBoxRef);
  const controls = useControlsVisibility({ playing: !videoState.paused });
  const { route, choices, quality, setQuality } = usePlaybackRoute(data.transcoding);
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    setJikkyoEnabled,
  } = usePlayerSettings();
  const recording = data.node?.__typename === 'Recording' ? data.node : null;
  const duration =
    recording === null
      ? 0
      : Math.max(
          ((recording.recordedUntil == null ? Date.now() : Date.parse(recording.recordedUntil)) -
            Date.parse(recording.recordedFrom)) /
            1000,
          0,
        );
  // resumeAt は、前回の続きから再生する位置 (秒)。続きがなければ 0 (設計書 7.5)。
  // Int64 は Relay の型では any になるので、数に直す。
  const savedMs = recording?.watchPositionMs == null ? null : Number(recording.watchPositionMs);
  const [resumeAt] = useState(() => resumePosition(savedMs, duration));
  // kyusu の HLS は、録画の全体を 1 つのプレイリストで表すので、video の時刻がそのまま録画の中の位置になる。
  const hls = route.kind === 'kyusu';
  // offset は、今の読み込みを始めた、録画の始まりからの秒。ブラウザの中で変換するときは、シークすると、
  // 新しい開始位置で読み直す (設計書 7.2)。kyusu の HLS では、画質を変えたときに続きから読む位置に使う。
  const [offset, setOffset] = useState(resumeAt);
  // loads は、読み込みを始め直した回数。同じ位置へシークし直したときも、URL を変えて読み直させる。
  const [loads, setLoads] = useState(0);
  // base は、読み込んだ映像の最初のフレームの、video の時刻。変換した映像の時刻は 0 から始まるとは限らない。
  const base = useRef(0);
  // phase は、読み込みの段階。読み直している間に、前の映像の時刻で位置を動かさないよう、base を決めるまで位置を更新しない。
  const phase = useRef<'loading' | 'awaitingBase' | 'playing'>('loading');
  const [position, setPosition] = useState(resumeAt);
  // positionRef は、別のウィンドウへの移し替えや、画質と音声の切り替えで、読み直すときに続ける位置。
  const positionRef = useRef(resumeAt);
  const [seeking, setSeeking] = useState<number | null>(null);
  const [paused, setPaused] = useState(true);
  const [error, setError] = useState<string | null>(null);

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

    return playbackPosition(offset, base.current, video.currentTime);
  };
  const channel = recording?.service.jikkyo ?? null;
  useWatchPositionSaver(recordingId, currentPosition, !paused);
  const jikkyo = useRecordedJikkyo(
    channel !== null && jikkyoEnabled && recording !== null ? recording.jikkyoCommentsUrl : null,
    currentPosition,
    visible,
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

  // startAt は、指定の位置から読み直す。
  const startAt = (seconds: number) => {
    phase.current = 'loading';
    positionRef.current = seconds;
    setPosition(seconds);
    setOffset(seconds);
    setLoads((n) => n + 1);
  };

  // seek は、指定の位置に移る。kyusu の HLS は、プレイリストの中を移るだけで、読み直さない。
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
  usePlayerShortcuts({
    togglePlay,
    seekBackward: () => skip(-10),
    seekForward: () => skip(10),
    toggleCaptions: () => setCaptions(!captions),
    toggleMute: videoState.toggleMute,
    toggleFullscreen: fullscreen.toggle,
  });
  useDocumentTitle([recording?.name ?? null]);
  useMediaSession(
    {
      title: recording?.name ?? '',
      artist: recording?.service.name ?? '',
      artwork: recording?.thumbnailUrl ?? null,
    },
    {
      play: () => void videoRef.current?.play().catch(() => {}),
      pause: () => videoRef.current?.pause(),
      seekBackward: () => skip(-10),
      seekForward: () => skip(10),
      seekTo: (seconds) => seek(seconds),
    },
    {
      playing: !videoState.paused,
      position: { duration, position: seeking ?? position, playbackRate: rate },
    },
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
        phase.current = 'playing';
      }
      const next = playbackPosition(offset, base.current, video.currentTime);
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
  }, [offset, hls, video]);

  if (recording === null) {
    return <Alert color="amber">{t('recordings.notFound')}</Alert>;
  }
  // フラグメントはサーバーに送られないので、読み込みを始め直すためだけに使う。
  const url = `${recording.streamUrl}?start=${offset.toFixed(1)}#${loads}`;
  const onError = () => setError(t('player.error'));
  const shown = seeking ?? position;

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <Slider
          aria-label={t('player.seek')}
          size={4}
          color="mikan"
          min={0}
          max={Math.max(duration, 1)}
          step={1}
          value={Math.min(shown, duration)}
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
          <PlayButton video={videoState} />
          <VolumeControl video={videoState} />
        </>
      }
      status={`${formatDuration(shown)} / ${formatDuration(duration)} · ${formatClock(broadcastTime(recording.recordedFrom, recording.gaps, shown))}`}
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
              label={jikkyoLabel}
              icon={MessageSquareText}
              active={jikkyoEnabled}
              onClick={() => setJikkyoEnabled(!jikkyoEnabled)}
            />
          )}
          <SettingsButton>
            <QualitySelect choices={choices} quality={quality} onChange={changeQuality} />
            {audio.menu}
            <RateSelect value={rate} onChange={setRate} />
          </SettingsButton>
          <CaptureButton
            frameRef={playerBoxRef}
            videoRef={videoRef}
            serviceName={recording.service.name}
            programName={recording.name}
            captureTime={() =>
              broadcastTime(recording.recordedFrom, recording.gaps, currentPosition())
            }
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
      // マウスで映像を押すと、再生と一時停止を切り替える。タッチでは、押すと操作バーを出し隠しするので切り替えない。
      // 操作バーや、映像に重ねたボタンを押したときは、そのボタンの操作だけにする。
      onClick={(event) => {
        const pointer =
          event.nativeEvent instanceof PointerEvent ? event.nativeEvent.pointerType : 'mouse';
        if (
          pointer === 'mouse' &&
          event.target instanceof Element &&
          event.target.closest(`[${controlsAttribute}], button, a, input, [role="slider"]`) === null
        ) {
          togglePlay();
        }
      }}
    >
      {route.kind === 'convert' && (
        <TsPlayer
          url={url}
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          onLoaded={() => {
            // 最初のフレームが出たときの時刻を、base にする。
            phase.current = 'awaitingBase';
            setError(null);
          }}
          onError={onError}
          overlay={
            <>
              {jikkyo.overlay}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
            </>
          }
          onAudio={audio.onAudio}
          audio={audio.convertAudio}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          url={hlsURL(recording.streamUrl, route.preset, audio.kyusuAudio)}
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          live={false}
          startPosition={offset}
          overlay={
            <>
              {jikkyo.overlay}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
            </>
          }
          onLoaded={() => setError(null)}
          onError={onError}
        />
      )}
    </div>
  );

  return (
    <Stack gap="xs">
      <Title order={2} size="h4">
        <ProgramTitle name={recording.name} />
      </Title>
      <Text size="sm" c="dimmed">
        <ServiceLabel name={recording.service.name} logoUrl={recording.service.logoUrl} />
      </Text>
      <PlaybackUnavailable route={route} />
      <div ref={frameRef}>
        {pip.container === null ? playerBox : <PipPlaceholder onBack={pip.close} />}
      </div>
      {pip.container !== null && createPortal(playerBox, pip.container)}
      {error !== null && (
        <Alert color="red" role="alert">
          {error}
        </Alert>
      )}
    </Stack>
  );
}
