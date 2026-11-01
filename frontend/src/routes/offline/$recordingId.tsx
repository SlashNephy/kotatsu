import { Alert, Anchor, Stack } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Captions, MessageSquareText } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { PageLoader, ProgramTitle, useDocumentTitle } from '../../futon';
import { setTimeZone } from '../../futon/timeZone';
import { useRecordedJikkyo } from '../../jikkyo/useRecordedJikkyo';
import { openOfflineRecording, type OfflineFiles } from '../../offline/store';
import type { PlayerCommands } from '../../yunomi/commands';
import {
  BigPlayButton,
  CommandButton,
  FullscreenButton,
  PlayButton,
  PlayerAlert,
  PlayerControls,
  playerControlsClasses,
  PlayerLoading,
  VolumeControl,
} from '../../yunomi/PlayerControls';
import {
  broadcastTime,
  formatClock,
  formatDuration,
  recordingPosition,
} from '../../yunomi/position';
import { SeekBar } from '../../yunomi/SeekBar';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { onVideoSurface, useControlsVisibility } from '../../yunomi/useControlsVisibility';
import { useAutoplayBlocked } from '../../yunomi/useAutoplayBlocked';
import { useFullscreen } from '../../yunomi/useFullscreen';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import { useVideoLoading } from '../../yunomi/useVideoLoading';
import { useVideoState } from '../../yunomi/useVideoState';
import { useWakeLock } from '../../yunomi/useWakeLock';
import { WatchHeading, WatchLayout } from '../../yunomi/WatchLayout';

export const Route = createFileRoute('/offline/$recordingId')({
  staticData: { fluid: true },
  component: OfflinePlayPage,
});

// OfflinePlayPage は、端末に保存した録画を再生する画面 (#480、設計書 7.8)。サーバーにつながらなくても見られるよう、
// GraphQL を読まず、保存した TS をブラウザの中で変換して流す (設計書 7.2)。
function OfflinePlayPage() {
  const { t } = useTranslation();
  const { recordingId } = Route.useParams();
  // files は、開いた録画のファイル。読み込む前は undefined、保存していなければ null。
  const [files, setFiles] = useState<OfflineFiles | null | undefined>(undefined);
  useEffect(() => {
    let active = true;
    openOfflineRecording(recordingId).then(
      (opened) => {
        if (active) {
          setFiles(opened);
        }
      },
      () => {
        if (active) {
          setFiles(null);
        }
      },
    );

    return () => {
      active = false;
    };
  }, [recordingId]);

  return (
    <Stack gap="sm">
      <Anchor size="sm" component={Link} to="/offline">
        {t('offline.back')}
      </Anchor>
      {files === undefined ? (
        <PageLoader />
      ) : files === null ? (
        <Alert color="amber">{t('offline.notFound')}</Alert>
      ) : (
        <OfflinePlayer key={recordingId} files={files} />
      )}
    </Stack>
  );
}

// useObjectURL は、file を読む blob: の URL を作り、使い終えたら手放す。
// 変換のプレイヤーは、この URL を Range で読むので、映像の途中へのシークもプレイヤーが受け持つ。
function useObjectURL(file: Blob | null): string | null {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    if (file === null) {
      setUrl(null);

      return;
    }
    const created = URL.createObjectURL(file);
    setUrl(created);

    return () => URL.revokeObjectURL(created);
  }, [file]);

  return url;
}

function OfflinePlayer({ files }: { files: OfflineFiles }) {
  const { t } = useTranslation();
  const { meta } = files;
  // オフラインではサーバーのタイムゾーンを読めないので、保存したときのタイムゾーンで日時を書く。
  setTimeZone(meta.timeZone);
  useDocumentTitle([meta.name]);
  const videoURL = useObjectURL(files.video);
  const jikkyoURL = useObjectURL(files.jikkyo);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoState = useVideoState(video);
  const autoplay = useAutoplayBlocked(video);
  const videoLoading = useVideoLoading(video);
  const fullscreen = useFullscreen(playerBoxRef);
  const controls = useControlsVisibility({ playing: !videoState.paused });
  useWakeLock(!videoState.paused);
  // 字幕と実況の切り替えは、サーバーのユーザーの設定を読めないので、その場限りにする。
  const [captions, setCaptions] = useState(true);
  const [jikkyoEnabled, setJikkyoEnabled] = useState(true);
  const [error, setError] = useState(false);
  // loads は、読み直した回数。「やり直す」で、同じファイルを読み直させる。
  const [loads, setLoads] = useState(0);

  // 保存した TS は、配信が切り詰めた 1 本のファイルなので、video の時刻が、そのまま録画の始まりからの位置になる。
  // 長さは、プレイヤーがファイルから求めて video に渡す。分かるまでは、録画の時間帯から求める。
  const fallbackDuration = recordingPosition(
    meta.recordedFrom,
    meta.gaps,
    Date.parse(meta.recordedUntil),
  );
  const [position, setPosition] = useState(0);
  const [duration, setDuration] = useState(fallbackDuration);
  const [seeking, setSeeking] = useState<number | null>(null);
  useEffect(() => {
    if (video === null) {
      return;
    }
    const update = () => setPosition(video.currentTime);
    const onDuration = () => {
      if (Number.isFinite(video.duration) && video.duration > 0) {
        setDuration(video.duration);
      }
    };
    video.addEventListener('timeupdate', update);
    video.addEventListener('durationchange', onDuration);
    onDuration();

    return () => {
      video.removeEventListener('timeupdate', update);
      video.removeEventListener('durationchange', onDuration);
    };
  }, [video]);
  const currentPosition = () => videoRef.current?.currentTime ?? position;
  const jikkyo = useRecordedJikkyo(jikkyoEnabled ? jikkyoURL : null, currentPosition, true);

  const seek = (target: number) => {
    const element = videoRef.current;
    if (element === null) {
      return;
    }
    element.currentTime = Math.min(Math.max(target, 0), duration);
    setPosition(element.currentTime);
  };
  const togglePlay = () => {
    const element = videoRef.current;
    if (element === null) {
      return;
    }
    if (element.paused) {
      void element.play().catch(() => {});
    } else {
      element.pause();
    }
  };
  const commands: PlayerCommands = {
    play: () => void videoRef.current?.play().catch(() => {}),
    pause: () => videoRef.current?.pause(),
    togglePlay,
    seekBackward: () => seek(currentPosition() - 10),
    seekForward: () => seek(currentPosition() + 10),
    seekBackwardLong: () => seek(currentPosition() - 30),
    seekForwardLong: () => seek(currentPosition() + 30),
    seekTo: seek,
    toggleCaptions: () => setCaptions(!captions),
    ...(files.jikkyo === null ? {} : { toggleJikkyo: () => setJikkyoEnabled(!jikkyoEnabled) }),
    toggleMute: videoState.toggleMute,
    setVolume: videoState.setVolume,
    setMuted: (muted) => {
      if (muted !== videoState.muted) {
        videoState.toggleMute();
      }
    },
    toggleFullscreen: fullscreen.toggle,
    restart: () => {
      setError(false);
      setLoads((n) => n + 1);
    },
  };
  usePlayerShortcuts(commands);

  const shown = seeking ?? position;
  const clock = (seconds: number) =>
    formatClock(broadcastTime(meta.recordedFrom, meta.gaps, seconds));
  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <SeekBar
          duration={duration}
          value={shown}
          previewUrl={null}
          label={(value) => `${formatDuration(value)} (${clock(value)})`}
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
      status={`${formatDuration(shown)} / ${formatDuration(duration)} · ${clock(shown)}`}
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
          <FullscreenButton active={fullscreen.active} commands={commands} />
        </>
      }
    />
  );

  return (
    <WatchLayout
      player={
        <div
          ref={playerBoxRef}
          className={playerControlsClasses.box}
          data-controls-visible={controls.visible}
          {...controls.containerProps}
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
        >
          <TsPlayer
            // フラグメントは、読み直すためだけに付ける。blob: の URL でも、読むときには外される。
            url={videoURL === null ? null : `${videoURL}#${loads}`}
            videoRef={videoRef}
            onVideo={setVideo}
            controls={controlBar}
            captions={captions}
            onLoaded={() => setError(false)}
            onError={() => setError(true)}
            overlay={
              <>
                {jikkyo.overlay}
                {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
                {videoLoading && !autoplay.blocked && !error && <PlayerLoading />}
                {error && (
                  <PlayerAlert
                    title={t('player.error')}
                    action={{ label: t('player.retry'), onClick: () => commands.restart?.() }}
                  />
                )}
              </>
            }
          />
        </div>
      }
      info={
        <WatchHeading
          title={<ProgramTitle name={meta.name} />}
          serviceName={meta.serviceName}
          serviceLogoUrl={null}
          time={meta}
        />
      }
      side={null}
    />
  );
}
