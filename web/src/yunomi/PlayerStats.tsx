import { Switch } from '@mantine/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import classes from './PlayerControls.module.css';

// VideoStats は、統計情報に出す、映像の再生の様子 (#499)。
export type VideoStats = {
  width: number;
  height: number;
  // dropped と total は、捨てたフレームと、描いたフレームの数。数えられないブラウザでは null。
  dropped: number | null;
  total: number | null;
  // buffered は、今の位置から先に読み込んである秒数。
  buffered: number;
  rate: number;
};

// readVideoStats は、video から、統計情報を読む。
export function readVideoStats(
  video: Pick<
    HTMLVideoElement,
    'videoWidth' | 'videoHeight' | 'currentTime' | 'buffered' | 'playbackRate'
  > &
    Partial<Pick<HTMLVideoElement, 'getVideoPlaybackQuality'>>,
): VideoStats {
  let buffered = 0;
  for (let i = 0; i < video.buffered.length; i++) {
    if (
      video.buffered.start(i) <= video.currentTime &&
      video.currentTime <= video.buffered.end(i)
    ) {
      buffered = video.buffered.end(i) - video.currentTime;
    }
  }
  const quality = video.getVideoPlaybackQuality?.();

  return {
    width: video.videoWidth,
    height: video.videoHeight,
    dropped: quality?.droppedVideoFrames ?? null,
    total: quality?.totalVideoFrames ?? null,
    buffered,
    rate: video.playbackRate,
  };
}

// statsIntervalMs は、統計情報を読み直す間隔。
const statsIntervalMs = 1000;

// PlayerStats は、映像の上に、再生の統計情報 (経路と画質、解像度、捨てたフレーム、バッファ、再生速度) を重ねる (#499)。
export function PlayerStats({
  video,
  quality,
}: {
  video: HTMLVideoElement | null;
  // quality は、今の経路を表す画質の名前。
  quality: string | null;
}) {
  const { t } = useTranslation();
  const [stats, setStats] = useState<VideoStats | null>(null);
  useEffect(() => {
    if (video === null) {
      setStats(null);

      return;
    }
    const update = () => setStats(readVideoStats(video));
    update();
    const timer = setInterval(update, statsIntervalMs);

    return () => clearInterval(timer);
  }, [video]);

  return (
    <dl className={classes.stats} aria-label={t('player.stats.title')}>
      <dt>{t('player.stats.quality')}</dt>
      <dd>{quality ?? '-'}</dd>
      <dt>{t('player.stats.resolution')}</dt>
      <dd>{stats === null || stats.width === 0 ? '-' : `${stats.width}×${stats.height}`}</dd>
      <dt>{t('player.stats.droppedFrames')}</dt>
      <dd>
        {stats?.dropped == null || stats.total == null ? '-' : `${stats.dropped} / ${stats.total}`}
      </dd>
      <dt>{t('player.stats.buffered')}</dt>
      <dd>
        {stats === null ? '-' : t('player.stats.seconds', { seconds: stats.buffered.toFixed(1) })}
      </dd>
      <dt>{t('player.stats.rate')}</dt>
      <dd>{stats === null ? '-' : `${stats.rate}×`}</dd>
    </dl>
  );
}

// StatsToggle は、設定のメニューに置く、統計情報を出すかの切り替え。
export function StatsToggle({
  shown,
  onChange,
}: {
  shown: boolean;
  onChange: (shown: boolean) => void;
}) {
  const { t } = useTranslation();

  return (
    <Switch
      size="xs"
      label={t('player.stats.toggle')}
      checked={shown}
      onChange={(event) => onChange(event.currentTarget.checked)}
    />
  );
}
