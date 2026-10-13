import { ActionIcon, Popover, Slider, Stack, Tooltip } from '@mantine/core';
import {
  Maximize,
  Minimize,
  Pause,
  Play,
  Settings,
  Volume1,
  Volume2,
  VolumeX,
  type LucideIcon,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import classes from './PlayerControls.module.css';
import { controlsAttribute } from './useControlsVisibility';
import type { VideoState } from './useVideoState';

export { classes as playerControlsClasses };

type Props = {
  visible: boolean;
  // seek は、録画のシークバー。ライブにはない。
  seek?: ReactNode;
  // start は、左に並べる操作 (再生と音量)。
  start: ReactNode;
  // status は、真ん中に出す、再生位置やライブの印。
  status: ReactNode;
  // end は、右に並べる操作 (字幕、実況、設定、キャプチャ、全画面など)。
  end: ReactNode;
};

// PlayerControls は、映像の下端に重ねる操作バー。操作がなくなると隠れる (useControlsVisibility)。
export function PlayerControls({ visible, seek, start, status, end }: Props) {
  return (
    <div className={classes.bar} data-visible={visible} {...{ [controlsAttribute]: '' }}>
      {seek}
      <div className={classes.row}>
        <div className={classes.group}>{start}</div>
        <div className={classes.status}>{status}</div>
        <div className={classes.group}>{end}</div>
      </div>
    </div>
  );
}

type ButtonProps = {
  label: string;
  icon: LucideIcon;
  onClick?: () => void;
  // active は、切り替えを入れているかどうか。切り替えでなければ渡さない。
  active?: boolean;
  // optional は、狭い画面で外してよいかどうか。
  optional?: boolean;
  // notice は、名前の代わりに、しばらく出しておく知らせ (失敗したことなど)。
  notice?: string | null;
};

// ControlButton は、操作バーのアイコンのボタン。名前は、ツールチップと読み上げで示す。
// 別のウィンドウに移っても出せるよう、ツールチップはプレイヤーの中に描く。
export function ControlButton({
  label,
  icon: Icon,
  onClick,
  active,
  optional = false,
  notice = null,
}: ButtonProps) {
  return (
    <Tooltip
      label={notice ?? label}
      opened={notice === null ? undefined : true}
      withinPortal={false}
    >
      <ActionIcon
        variant="transparent"
        size="lg"
        aria-label={label}
        aria-pressed={active}
        data-active={active}
        className={`${classes.button} ${optional ? classes.optional : ''}`}
        onClick={onClick}
      >
        <Icon size={20} aria-hidden />
      </ActionIcon>
    </Tooltip>
  );
}

// PlayButton は、再生と一時停止を切り替える。
export function PlayButton({ video }: { video: VideoState }) {
  const { t } = useTranslation();

  return (
    <ControlButton
      label={video.paused ? t('player.play') : t('player.pause')}
      icon={video.paused ? Play : Pause}
      onClick={video.togglePlay}
    />
  );
}

// VolumeControl は、ミュートのボタンと、ホバーで広がる音量のスライダー。
export function VolumeControl({ video }: { video: VideoState }) {
  const { t } = useTranslation();
  // ミュートしている間に目盛りが残っていると、音が出るように見えるので、0 として見せる。
  const shown = video.muted ? 0 : video.volume;
  const icon = shown < 0.01 ? VolumeX : shown < 0.5 ? Volume1 : Volume2;

  return (
    <div className={classes.volume}>
      <ControlButton
        label={video.muted ? t('player.unmute') : t('player.mute')}
        icon={icon}
        onClick={video.toggleMute}
      />
      <div className={classes.volumeSlider}>
        <Slider
          aria-label={t('player.volume')}
          thumbLabel={t('player.volume')}
          min={0}
          max={1}
          step={0.01}
          size={4}
          color="white"
          label={null}
          value={shown}
          onChange={video.setVolume}
        />
      </div>
    </div>
  );
}

// LiveStatus は、ライブで再生していることの印。
export function LiveStatus() {
  const { t } = useTranslation();

  return <span className={classes.live}>{t('player.live')}</span>;
}

// SettingsButton は、画質や音声、再生速度のメニューを開くボタン。
export function SettingsButton({ children }: { children: ReactNode }) {
  const { t } = useTranslation();

  return (
    <Popover position="top-end" withinPortal={false} shadow="md" width={240}>
      <Popover.Target>
        <Tooltip label={t('player.settings')} withinPortal={false}>
          <ActionIcon
            variant="transparent"
            size="lg"
            aria-label={t('player.settings')}
            className={classes.button}
          >
            <Settings size={20} aria-hidden />
          </ActionIcon>
        </Tooltip>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">{children}</Stack>
      </Popover.Dropdown>
    </Popover>
  );
}

// FullscreenButton は、プレイヤーを全画面にする。
export function FullscreenButton({ active, onToggle }: { active: boolean; onToggle: () => void }) {
  const { t } = useTranslation();

  return (
    <ControlButton
      label={active ? t('player.exitFullscreen') : t('player.fullscreen')}
      icon={active ? Minimize : Maximize}
      onClick={onToggle}
    />
  );
}
