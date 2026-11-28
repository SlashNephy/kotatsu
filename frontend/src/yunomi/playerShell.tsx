import { Stack } from '@mantine/core';
import { useMergedRef } from '@mantine/hooks';
import { Keyboard } from 'lucide-react';
import { type ReactNode, type RefObject, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useDocumentTitle } from '../futon';
import { useAudioDelayCommands, useAudioDelayer } from './audioDelay';
import { useAutoPictureInPicture } from './autoPictureInPicture';
import { stepBrightness } from './brightness';
import type { PlayerCommands } from './commands';
import { noCrop, type Crop } from './crop';
import { usePlayerGamepad } from './gamepad';
import { usePlayerMouse } from './mouseActions';
import { PipHost } from './PipHost';
import { PipPlaceholder } from './PipPlaceholder';
import {
  BigPlayButton,
  CommandButton,
  FullscreenButton,
  PlayerAlert,
  playerControlsClasses,
  PlayerFlash,
  PlayerLoading,
  usePlayerFlash,
} from './PlayerControls';
import { usePlayerContextMenu } from './PlayerContextMenu';
import type { PlayerMedia } from './playerMedia';
import {
  documentTitle,
  usePlayerEvents,
  type PlayerEvents,
  type PlayerSnapshot,
} from './playerState';
import { type Capture, CaptureButton, openPictureInPicture, PipButton } from './PlayerTools';
import { PlaybackUnavailable } from './QualitySelect';
import type { PlaybackRoute } from './route';
import { ScrubBubble } from './ScrubBubble';
import { copyPositionUrl } from './shareUrl';
import { ShortcutsHelp } from './ShortcutsHelp';
import { useSleepTimer, type SleepTimer } from './sleepTimer';
import { swipeVolumeStep } from './swipe';
import { useSwipeControlsSetting, volumeChangeable } from './swipeControls';
import { onVideoSurface, useControlsVisibility } from './useControlsVisibility';
import { useDocumentPictureInPicture } from './useDocumentPictureInPicture';
import { type TapSide, useDoubleTap } from './useDoubleTap';
import { useFullscreen } from './useFullscreen';
import { useMediaSession } from './useMediaSession';
import { usePlayerShortcuts } from './usePlayerShortcuts';
import { useRemoteOpen } from './useRemoteOpen';
import { type RemotePlayerSnapshot, useRemotePlayer } from './useRemotePlayer';
import { useScreenCrop } from './useScreenCrop';
import { type SlideSeekOptions, useSlideSeek } from './useSlideSeek';
import { useSwipeGestures } from './useSwipeGestures';
import { useAutoplayBlocked } from './useAutoplayBlocked';
import { useVideoLoading } from './useVideoLoading';
import { useVideoState, type VideoState } from './useVideoState';
import { useVisible } from './useVisible';
import { useWakeLock } from './useWakeLock';
import { stepVolume } from './volume';
import { currentVolume } from './volumeBoost';
import { useVolumeNormalizer } from './volumeNormalize';
import { useWatchMedia, useWatchMini } from './WatchHost';

// 視聴の画面 (ライブ視聴、タイムシフト、録画、ライブラリのファイル、端末に保存した録画の再生) で、再生元によらない
// プレイヤーの外側をまとめる (#2244)。再生元ごとの画面は、何をどの経路で流し、位置とシークをどう扱うかだけを持つ。

// volumeStep は、↑ と ↓ で音量を上げ下げする幅。
const volumeStep = 0.05;

// PlayerShell は、usePlayerShell が返す、プレイヤーの外側の状態と操作。
export type PlayerShell = {
  // mediaRef と media は、今映している経路 (#1516)。画質を変えたときに作り直される。部品は <video> ではなく、これを受け取る。
  mediaRef: RefObject<PlayerMedia | null>;
  media: PlayerMedia | null;
  setMedia: (media: PlayerMedia | null) => void;
  // frameRef は、画質を変えても作り直さない、プレイヤーの外側の要素。見えているかを、これで確認する。
  frameRef: RefObject<HTMLDivElement | null>;
  // playerBoxRef は、プレイヤーを包む要素。別のウィンドウに出すと移るので、キャプチャと全画面は、これを使う。
  playerBoxRef: RefObject<HTMLDivElement | null>;
  // pip は、視聴画面を出す別のウィンドウ (設計書 7.5)。
  pip: ReturnType<typeof useDocumentPictureInPicture>;
  // visible は、ページで見えているか、別のウィンドウに出しているか。見えている間だけ、実況のコメントなどを描く。
  visible: boolean;
  fullscreen: ReturnType<typeof useFullscreen>;
  // overlaid は、プレイヤーだけを出していて、ページの通知が見えないかどうか。そのときは、映像に重ねて出す (#435)。
  overlaid: boolean;
  controls: ReturnType<typeof useControlsVisibility>;
  videoState: VideoState;
  autoplay: ReturnType<typeof useAutoplayBlocked>;
  videoLoading: boolean;
  // shortcutsOpen は、キーボードショートカットの一覧を出しているかどうか。
  shortcutsOpen: boolean;
  setShortcutsOpen: (open: boolean) => void;
  // flash は、キーで操作した結果。映像の上に短く出す (#448)。
  flash: string | null;
  showFlash: (text: string) => void;
  // crop は、L 字放送の切り抜きと表示の比率 (#496、#824)。shownCrop は、全画面の比率に合わせて映す切り抜き (#1599)。
  crop: Crop;
  setCrop: (crop: Crop) => void;
  shownCrop: Crop;
  togglePlay: () => void;
  // changeVolume は、音量を delta だけ変え、変えた音量を映像の上に出す。
  changeVolume: (delta: number) => void;
  // commands は、再生元によらない操作 (#809)。再生元ごとの画面は、これにシークなどの操作を足す。
  commands: PlayerCommands;
  // seekCommands は、位置を指定して移れる再生元の、相対のシークと位置の共有、読み直しの操作を作る。
  // skip が true を返したら (CM を飛んだなど)、進む向きのシークの通知を出さない。
  seekCommands: (seek: {
    skip: (seconds: number) => boolean | void;
    seekTo: (seconds: number) => void;
    currentPosition: () => number;
    retry: () => void;
  }) => PlayerCommands;
};

// usePlayerShell は、プレイヤーの外側の状態を用意する。再生元ごとの画面の先頭で呼ぶ。
export function usePlayerShell(): PlayerShell {
  const { t } = useTranslation();
  const mediaRef = useRef<PlayerMedia | null>(null);
  // 画質を変えると video の要素を作り直すので、見えているかは、変わらない外側の要素で確認する。
  const frameRef = useRef<HTMLDivElement | null>(null);
  const inPage = useVisible(frameRef);
  const pip = useDocumentPictureInPicture();
  const visible = inPage || pip.container !== null;
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  const [media, setMedia] = useState<PlayerMedia | null>(null);
  // 小さなプレイヤーは、この経路で再生中かを確認し、再生と一時停止を操作する。
  useWatchMedia(media);
  const videoState = useVideoState(media);
  // ブラウザに音ありの自動再生を拒否されたら、映像の真ん中に再生ボタンを出す。
  const autoplay = useAutoplayBlocked(media);
  const videoLoading = useVideoLoading(media);
  const fullscreen = useFullscreen(playerBoxRef);
  const overlaid = fullscreen.active || pip.container !== null;
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  // ショートカットの一覧を見ている間は、操作バーとカーソルを隠さない。
  const controls = useControlsVisibility({ playing: !videoState.paused && !shortcutsOpen });
  // 再生している間は、画面を消さない。
  useWakeLock(!videoState.paused);
  // 音量の補正 (#499) と音ずれ補正 (#818)。端末ごとの設定に合わせる。
  useVolumeNormalizer(media);
  useAudioDelayer(media);
  // crop は、その場限りで、画面を開き直したら戻す。
  const [crop, setCrop] = useState<Crop>(noCrop);
  const shownCrop = useScreenCrop(crop, fullscreen.active);
  const [flash, showFlash] = usePlayerFlash();

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
  // 押し続けると、描き直す前に次のキーが来るので、今の音量は mediaRef から読む。映像がなければ何もしない。
  const changeVolume = (delta: number) => {
    const current = mediaRef.current;
    if (current === null) {
      return;
    }
    const next = stepVolume(currentVolume(current), delta);
    videoState.setVolume(next);
    showFlash(t('player.flash.volume', { percent: Math.round(next * 100) }));
  };
  // audioDelayCommands は、音ずれ補正 (#818) の操作。変えたら、長さを映像の上に短く出す。
  const audioDelayCommands = useAudioDelayCommands((ms) =>
    showFlash(t('player.flash.audioDelay', { ms })),
  );
  const commands: PlayerCommands = {
    ...audioDelayCommands,
    play: () => void mediaRef.current?.play().catch(() => {}),
    pause: () => mediaRef.current?.pause(),
    togglePlay,
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
    showShortcuts: () => setShortcutsOpen(true),
    setVolume: videoState.setVolume,
    setMuted: (muted) => {
      if (muted !== videoState.muted) {
        videoState.toggleMute();
      }
    },
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
  const seekCommands: PlayerShell['seekCommands'] = ({ skip, seekTo, currentPosition, retry }) => ({
    seekBackward: () => {
      skip(-10);
      showFlash(t('player.flash.seekBackward'));
    },
    seekForward: () => {
      if (skip(10) !== true) {
        showFlash(t('player.flash.seekForward'));
      }
    },
    seekBackwardLong: () => {
      skip(-30);
      showFlash(t('player.flash.seekBackwardLong'));
    },
    seekForwardLong: () => {
      if (skip(30) !== true) {
        showFlash(t('player.flash.seekForwardLong'));
      }
    },
    seekBackward15: () => {
      skip(-15);
      showFlash(t('player.flash.seekBackward15'));
    },
    seekForward15: () => {
      if (skip(15) !== true) {
        showFlash(t('player.flash.seekForward15'));
      }
    },
    copyPositionUrl: () => {
      copyPositionUrl(currentPosition(), playerBoxRef.current).then(
        () => showFlash(t('player.flash.urlCopied')),
        () => showFlash(t('player.flash.urlCopyFailed')),
      );
    },
    // r で、今の位置から読み直す。映像が止まったり乱れたりしたときに、ページを読み直さずに直す (#499)。
    restart: () => {
      retry();
      showFlash(t('player.flash.restarted'));
    },
    seekTo,
  });

  return {
    mediaRef,
    media,
    setMedia,
    frameRef,
    playerBoxRef,
    pip,
    visible,
    fullscreen,
    overlaid,
    controls,
    videoState,
    autoplay,
    videoLoading,
    shortcutsOpen,
    setShortcutsOpen,
    flash,
    showFlash,
    crop,
    setCrop,
    shownCrop,
    togglePlay,
    changeVolume,
    commands,
    seekCommands,
  };
}

// PlayerInputs は、usePlayerInputs が返す、入力を受け付ける要素に渡す値と、入力で変わる状態。
export type PlayerInputs = {
  // sleep は、スリープタイマー (#821)。時間になったら、commands.stop でプレイヤーを閉じる。
  sleep: SleepTimer;
  contextMenu: ReturnType<typeof usePlayerContextMenu>;
  // scrub は、映像を長押ししたまま滑らせている間の行き先 (#1601)。滑らせていなければ null。
  scrub: number | null;
  // brightness は、全画面の縦のスワイプで変えた明るさ (#1598)。その場限りで保存しない。
  brightness: number;
  boxRef: (element: HTMLDivElement | null) => void;
  // onDoubleTap は、プレイヤーを包む要素の pointerup のハンドラー。ダブルタップでシークしない画面では undefined。
  onDoubleTap: ((event: React.PointerEvent<HTMLElement>) => void) | undefined;
};

// PlayerInputsOptions は、再生元ごとに変わる入力の扱い。
export type PlayerInputsOptions = {
  // onDoubleTap は、タッチで映像の左右の 3 分の 1 をダブルタップしたとき (#452) に呼ぶ。真ん中は呼ばない。
  // ダブルタップでシークしない画面 (ライブ視聴) では渡さない。
  onDoubleTap?: (side: Exclude<TapSide, 'center'>) => void;
  // slideSeek は、映像を長押ししたまま横に滑らせるシーク (#1601)。onLongPress は、メニューを開くので渡さない。
  slideSeek: Omit<SlideSeekOptions, 'onSlideStart' | 'onLongPress'>;
};

// usePlayerInputs は、キー、ゲームパッド、マウス、タッチの操作と、スリープタイマーを、commands につなぐ。
export function usePlayerInputs(
  shell: PlayerShell,
  commands: PlayerCommands,
  options: PlayerInputsOptions,
): PlayerInputs {
  const { t } = useTranslation();
  const sleep = useSleepTimer(() => commands.stop?.());
  usePlayerShortcuts(commands);
  // contextMenu は、映像を右クリックしたときのメニュー (#1140)。
  const contextMenu = usePlayerContextMenu(commands);
  // ゲームパッドでも、同じ操作を呼べるようにする (#810)。
  usePlayerGamepad(commands);
  // 映像の上のホイールと中クリックも、設定の操作にする (#810)。
  const mouseRef = usePlayerMouse(commands);
  // 全画面で、映像の左右を縦にスワイプして、明るさと音量を変える (#1598)。端末の設定で入れたときだけ受け付ける。
  // 押したときに出した操作バーは、縦のスワイプと決めたら隠し直す。
  const [swipeControls] = useSwipeControlsSetting();
  const [volumeAvailable] = useState(volumeChangeable);
  const [brightness, setBrightness] = useState(1);
  const brightnessRef = useRef(brightness);
  brightnessRef.current = brightness;
  useSwipeGestures(shell.playerBoxRef, {
    enabled: swipeControls && shell.fullscreen.active,
    controlsVisible: shell.controls.visible,
    volumeAvailable,
    onSwipeStart: shell.controls.hide,
    onStep: (band, steps) => {
      if (band === 'volume') {
        shell.changeVolume(steps * swipeVolumeStep);

        return;
      }
      const next = stepBrightness(brightnessRef.current, steps);
      brightnessRef.current = next;
      setBrightness(next);
      shell.showFlash(t('player.flash.brightness', { percent: Math.round(next * 100) }));
    },
  });
  // 滑らせている間は、シークバーのつまみを動かしている間と同じく扱う。滑らせずに離したら、長押しのメニューを開く。
  const scrub = useSlideSeek(shell.playerBoxRef, {
    ...options.slideSeek,
    onSlideStart: shell.controls.hide,
    onLongPress: contextMenu.openAt,
  });
  const doubleTap = useDoubleTap((side) => {
    if (side !== 'center') {
      options.onDoubleTap?.(side);
    }
  });
  const onDoubleTap = options.onDoubleTap === undefined ? undefined : doubleTap;
  // boxRef は、キャプチャや全画面が使う playerBoxRef と、マウスの操作の両方に渡す。
  const boxRef = useMergedRef(shell.playerBoxRef, mouseRef);

  return { sleep, contextMenu, scrub, brightness, boxRef, onDoubleTap };
}

// PlayerBox は、プレイヤーを包む要素。映像の上の、マウスとタッチの操作を受け付ける。
// clickToPlay は、マウスで映像を押したときに再生と一時停止を切り替えるか。doubleClickFullscreen は、ダブルクリックで全画面を
// 切り替えるか。ライブ視聴では、データ放送を出している間、ダブルクリックをデータ放送の操作に譲る。
export function PlayerBox({
  shell,
  inputs,
  clickToPlay = true,
  doubleClickFullscreen = true,
  children,
}: {
  shell: PlayerShell;
  inputs: PlayerInputs;
  clickToPlay?: boolean;
  doubleClickFullscreen?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      ref={inputs.boxRef}
      // 映像を右クリックすると、操作のメニューを出す (#1140)。
      onContextMenu={inputs.contextMenu.onContextMenu}
      className={playerControlsClasses.box}
      data-controls-visible={shell.controls.visible}
      {...shell.controls.containerProps}
      // マウスで映像を押すと、再生と一時停止を切り替える。タッチでは、押すと操作バーを出し隠しするので切り替えない。
      // 操作バーや、映像に重ねたボタンを押したときは、そのボタンの操作だけにする。
      onClick={
        clickToPlay
          ? (event) => {
              const pointer =
                event.nativeEvent instanceof PointerEvent ? event.nativeEvent.pointerType : 'mouse';
              if (pointer === 'mouse' && onVideoSurface(event.target)) {
                shell.togglePlay();
              }
            }
          : undefined
      }
      // マウスで映像をダブルクリックすると、全画面を切り替える (#452)。1 回目と 2 回目のクリックで、再生と一時停止は元に戻る。
      onDoubleClick={(event) => {
        if (doubleClickFullscreen && onVideoSurface(event.target)) {
          shell.fullscreen.toggle();
        }
      }}
      onPointerUp={inputs.onDoubleTap}
    >
      {inputs.contextMenu.menu}
      {children}
    </div>
  );
}

// PlayerFeedback は、映像に重ねる、操作の結果と読み込みの様子。flash は、出す操作の結果 (早回しの速さなどに置き換えたもの)。
// notice は、画質の自動の切り替えなどの通知。scrub は、滑らせている間の行き先の表示。receiving は、行き先の表示の後に重ねる、
// 受信の様子 (データ放送を受信している間など)。loadingHidden は、エラーや切断を出している間、読み込み中の表示を出さないため。
export function PlayerFeedback({
  shell,
  flash,
  notice,
  scrub,
  receiving,
  loadingHidden,
}: {
  shell: PlayerShell;
  flash: string | null;
  notice?: ReactNode;
  scrub: { seconds: number; label: string; previewUrl: string | null | undefined } | null;
  receiving?: ReactNode;
  loadingHidden: boolean;
}) {
  return (
    <>
      {flash !== null && <PlayerFlash>{flash}</PlayerFlash>}
      {notice}
      {scrub !== null && (
        <ScrubBubble seconds={scrub.seconds} label={scrub.label} previewUrl={scrub.previewUrl} />
      )}
      {receiving}
      {shell.autoplay.blocked && <BigPlayButton onPlay={shell.autoplay.play} />}
      {shell.videoLoading && !shell.autoplay.blocked && !loadingHidden && <PlayerLoading />}
    </>
  );
}

// PlayerErrorOverlay は、全画面と別のウィンドウで、再生のエラーを映像に重ねて出す (#435)。ページに出ているときは出さない。
export function PlayerErrorOverlay({
  shell,
  error,
  onRetry,
}: {
  shell: PlayerShell;
  error: string | null;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (!shell.overlaid || error === null) {
    return null;
  }

  return <PlayerAlert title={error} action={{ label: t('player.retry'), onClick: onRetry }} />;
}

// PlayerToolButtons は、操作バーの右端に並べる、キャプチャ、ショートカットの一覧、別のウィンドウ、全画面のボタン。
// captureOptional は、狭い画面でキャプチャのボタンを外すか。dataBroadcast は、ショートカットの一覧にデータ放送の操作を出すか。
export function PlayerToolButtons({
  shell,
  commands,
  capture,
  captureOptional = true,
  dataBroadcast = false,
}: {
  shell: PlayerShell;
  commands: PlayerCommands;
  capture: Capture;
  captureOptional?: boolean;
  dataBroadcast?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <>
      {/* 幅 375px ほどのスマートフォンでは、15 秒のシークのボタン (#1572) と再生位置を収めるため、キャプチャのボタンも外す。 */}
      <CaptureButton capture={capture} optional={captureOptional ? 'compact' : false} />
      <CommandButton
        commands={commands}
        command="showShortcuts"
        label={t('player.shortcuts.title')}
        icon={Keyboard}
        optional
      />
      <ShortcutsHelp
        opened={shell.shortcutsOpen}
        onClose={() => shell.setShortcutsOpen(false)}
        commands={commands}
        target={shell.playerBoxRef}
        dataBroadcast={dataBroadcast}
      />
      <PipButton pip={shell.pip} />
      <FullscreenButton active={shell.fullscreen.active} commands={commands} />
    </>
  );
}

// PlayerFrame は、視聴の画面のプレイヤーの欄。使えない経路の通知と、別のウィンドウに移せるプレイヤーを並べる。
export function PlayerFrame({
  shell,
  route,
  children,
}: {
  shell: PlayerShell;
  route: PlaybackRoute;
  children: ReactNode;
}) {
  return (
    <Stack gap="xs">
      <PlaybackUnavailable route={route} />
      <div ref={shell.frameRef}>
        <PipHost
          container={shell.pip.container}
          placeholder={<PipPlaceholder onBack={shell.pip.close} />}
        >
          {children}
        </PipHost>
      </div>
    </Stack>
  );
}

// usePlayerPresence は、プレイヤーの今の状態 (#811) を、タブの題名、OS のメディア操作、ほかの端末 (#459、設計書 7.7) に通知する。
// remote は、ほかの端末に通知する状態。null の間は、ほかの端末には通知しない。ほかの端末から、別のチャンネルや録画を
// 開かせられる (キャスト、#802)。
export function usePlayerPresence(
  player: PlayerSnapshot,
  commands: PlayerCommands,
  remote: RemotePlayerSnapshot | null,
): PlayerEvents {
  const events = usePlayerEvents(player);
  // 小さなプレイヤーで描いている間は、開いている別の画面のタイトルを変えない (#1731)。
  const mini = useWatchMini();
  useDocumentTitle(documentTitle(player), !mini);
  const [autoPictureInPicture] = useAutoPictureInPicture();
  // 設定で選んでいれば、タブを切り替えたときに、別のウィンドウに出す (#492)。
  useMediaSession(player, commands, autoPictureInPicture);
  const openRemote = useRemoteOpen();
  useRemotePlayer(remote, events, commands, undefined, openRemote);

  return events;
}
