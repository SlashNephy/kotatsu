import { ScrollArea, Stack, Text } from '@mantine/core';
import { useEffect, useRef, useState, type PointerEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';

import { readVideoStats, type VideoStats } from './PlayerStats';
import { languageName } from './QualitySelect';
import {
  StreamMeter,
  audioFormatKey,
  codecName,
  continuesFrames,
  formatBitrate,
  frameRate,
  historySeconds,
  type EncodedStream,
  type FrameCount,
  type HlsSegment,
  type SourcePicture,
  type TransportCounts,
  type TsPlayerReading,
} from './streamMeter';
import classes from './StreamInfo.module.css';

// useStreamMeter は、再生している経路から届く様子を溜める StreamMeter を作る (#828)。
// routeKey (画質の名前) が変わったら、前の経路の様子を捨てる。
export function useStreamMeter(routeKey: string | null) {
  const [meter] = useState(() => new StreamMeter());
  useEffect(() => {
    meter.reset();
  }, [meter, routeKey]);

  return {
    meter,
    onReading: (reading: TsPlayerReading) => meter.feedTs(Date.now(), reading),
    onSegment: (segment: HlsSegment) => meter.feedSegment(Date.now(), segment),
  };
}

// StreamAudio は、EPG から読んだ番組の音声 (GraphQL の ProgramAudio)。
export type StreamAudio = {
  readonly componentType: number;
  readonly languages: ReadonlyArray<string>;
  readonly samplingRate: number;
};

// Snapshot は、1 秒ごとに読む、右の列に出す様子。
type Snapshot = {
  rate: number | null;
  history: (number | null)[];
  transport: TransportCounts | null;
  source: SourcePicture | null;
  encoded: EncodedStream | null;
  video: VideoStats | null;
  fps: number | null;
};

// snapshotIntervalMs は、様子を読み直す間隔。
const snapshotIntervalMs = 1_000;
// fpsWindow は、フレームレートを平均する、読み直しの回数。
const fpsWindow = 5;

// pictureLabel は、映像の大きさを、1440×1080i のように書く。走査の方式が分からなければ、大きさだけにする。
function pictureLabel(width: number, height: number, interlaced: boolean | null): string {
  const scan = interlaced === null ? '' : interlaced ? 'i' : 'p';

  return `${width}×${height}${scan}`;
}

// audioLabel は、番組の音声の 1 つを、「ステレオ 48 kHz 日本語」のように書く。
function audioLabel(t: TFunction, audio: StreamAudio): string {
  const key = audioFormatKey(audio.componentType);
  const parts = [
    key === null
      ? t('player.streamInfo.audioFormat.other', { type: audio.componentType })
      : t(`player.streamInfo.audioFormat.${key}`),
    t('player.streamInfo.samplingRate', { khz: audio.samplingRate / 1_000 }),
    ...audio.languages.map((code) => languageName(t, code)),
  ];

  return parts.join(' ');
}

type Props = {
  meter: StreamMeter;
  video: HTMLVideoElement | null;
  // quality は、今の経路を表す画質の名前。
  quality: string | null;
  // kind は、再生している経路。ブラウザ内での変換 (convert) か、kyusu の HLS (kyusu) か。
  kind: 'convert' | 'kyusu' | null;
  // audios は、EPG から読んだ番組の音声。分からなければ null。
  audios: ReadonlyArray<StreamAudio> | null;
  // average は、ブラウザ内での変換で、届いた TS から測らずに出す平均のビットレート (bps)。録画とタイムシフトのバッファの
  // 再生では、プレイヤーが変換よりずっと先まで読むので、読んだバイト数からは測れない。分からなければ null。
  // 渡さなければ (ライブ)、届いた TS から測る。
  average?: number | null;
  // transport は、プレイヤーが数えた数の代わりに出す、TS のパケットの異常の数。録画では、録画しながらサーバーが数えた数を渡す。
  // プレイヤーは、録画のファイルを読み始めたところでも、ドロップとして数えることがある。
  transport?: TransportCounts | null;
  // height は、枠の高さ。CSS の長さで渡す。
  height?: string | number;
};

// StreamInfo は、右の列の「情報」(#828)。届いた映像のビットレートの推移と、放送の形式、再生の様子を出す。
// ブラウザ内での変換では、届いた TS 全体のビットレートと、TS のパケットの異常の数を出す。kyusu の経路では、
// エンコードした後のセグメントのビットレートと、hls.js が推定した帯域を出す。
export function StreamInfo({
  meter,
  video,
  quality,
  kind,
  audios,
  average,
  transport: recordedTransport,
  height = 240,
}: Props) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  useEffect(() => {
    const frames: FrameCount[] = [];
    const update = () => {
      const now = Date.now();
      const quality = video?.getVideoPlaybackQuality?.();
      if (video !== null && quality !== undefined && !video.paused) {
        const next = { frames: quality.totalVideoFrames, time: video.currentTime };
        const previous = frames.at(-1);
        // シークや読み直しを挟んだら、その前の点を捨てる。
        if (
          previous !== undefined &&
          !continuesFrames(previous, next, snapshotIntervalMs / 1_000, video.playbackRate)
        ) {
          frames.length = 0;
        }
        frames.push(next);
        if (frames.length > fpsWindow) {
          frames.shift();
        }
      }
      const first = frames[0];
      const last = frames.at(-1);
      setSnapshot({
        rate: meter.bitrate.rate(now),
        history: meter.bitrate.history(now),
        transport: meter.transport,
        source: meter.source,
        encoded: meter.encoded,
        video: video === null ? null : readVideoStats(video),
        fps: first === undefined || last === undefined ? null : frameRate(first, last),
      });
    };
    update();
    const timer = setInterval(update, snapshotIntervalMs);

    return () => clearInterval(timer);
  }, [meter, video]);

  const stats = snapshot?.video ?? null;
  const encoded = snapshot?.encoded ?? null;
  const source = snapshot?.source ?? null;
  const transport =
    recordedTransport !== undefined ? recordedTransport : (snapshot?.transport ?? null);
  // averaged は、測った値でなく平均を出すかどうか。
  const averaged = kind === 'convert' && average !== undefined;
  const rate = averaged ? average : (snapshot?.rate ?? null);
  const fps = snapshot?.fps ?? null;
  // 再生している映像の大きさは、プレイリストに書いてあればそれを、なければ video から読む。
  const size =
    encoded !== null && encoded.width > 0
      ? pictureLabel(encoded.width, encoded.height, null)
      : stats !== null && stats.width > 0
        ? pictureLabel(stats.width, stats.height, null)
        : null;
  const codecs =
    encoded === null
      ? []
      : [encoded.videoCodec, encoded.audioCodec].flatMap((codec) =>
          codec === null ? [] : [codecName(codec)],
        );
  const playing = size === null ? '-' : [size, ...codecs].join(' ');

  return (
    <ScrollArea h={height} type="auto">
      <Stack gap="sm" p="sm">
        <div>
          <Text size="xs" c="dimmed">
            {kind === 'kyusu'
              ? t('player.streamInfo.bitrateSegment')
              : averaged
                ? t('player.streamInfo.bitrateAverage')
                : t('player.streamInfo.bitrateTs')}
          </Text>
          <Text size="xl" fw={600} className={classes.value}>
            {rate === null ? '-' : formatBitrate(rate)}
          </Text>
          {!averaged && <Sparkline values={snapshot?.history ?? []} />}
        </div>
        <dl className={classes.list}>
          <dt>{t('player.stats.quality')}</dt>
          <dd>{quality ?? '-'}</dd>
          {kind === 'convert' && (
            <>
              <dt>{t('player.streamInfo.source')}</dt>
              <dd>
                {source === null
                  ? '-'
                  : pictureLabel(source.width, source.height, source.interlaced)}
              </dd>
            </>
          )}
          <dt>{t('player.streamInfo.playing')}</dt>
          <dd>{playing}</dd>
          <dt>{t('player.streamInfo.frameRate')}</dt>
          <dd>{fps === null ? '-' : `${fps.toFixed(2)} fps`}</dd>
          <dt>{t('player.streamInfo.audio')}</dt>
          <dd>
            {audios === null || audios.length === 0
              ? '-'
              : audios.map((audio, i) => <div key={i}>{audioLabel(t, audio)}</div>)}
          </dd>
          {kind === 'kyusu' && (
            <>
              <dt>{t('player.streamInfo.bandwidth')}</dt>
              <dd>{encoded?.bandwidth == null ? '-' : formatBitrate(encoded.bandwidth)}</dd>
            </>
          )}
          {(kind === 'convert' || recordedTransport !== undefined) && (
            <>
              <dt>{t('player.streamInfo.transport')}</dt>
              <dd>
                {transport === null
                  ? '-'
                  : t('player.streamInfo.transportCounts', {
                      dropped: transport.dropped,
                      errors: transport.errors,
                      scrambled: transport.scrambled,
                    })}
              </dd>
            </>
          )}
          <dt>{t('player.stats.droppedFrames')}</dt>
          <dd>
            {stats?.dropped == null || stats.total == null
              ? '-'
              : `${stats.dropped} / ${stats.total}`}
          </dd>
          <dt>{t('player.stats.buffered')}</dt>
          <dd>
            {stats === null
              ? '-'
              : t('player.stats.seconds', { seconds: stats.buffered.toFixed(1) })}
          </dd>
          <dt>{t('player.stats.rate')}</dt>
          <dd>{stats === null ? '-' : `${stats.rate}×`}</dd>
        </dl>
      </Stack>
    </ScrollArea>
  );
}

// sparklineWidth と sparklineHeight は、グラフの座標の大きさ。表示の幅は、枠に合わせて伸ばす。
const sparklineWidth = 240;
const sparklineHeight = 48;

// Sparkline は、直近の 1 秒ごとのビットレートの線。カーソルを乗せた位置の値を出す。値のない秒は、線を切る。
function Sparkline({ values }: { values: readonly (number | null)[] }) {
  const { t } = useTranslation();
  const [hover, setHover] = useState<number | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);
  const known = values.filter((value): value is number => value !== null);
  // 上に少し余白を残し、線が上端に貼り付かないようにする。
  const max = known.length === 0 ? 1 : Math.max(...known) * 1.15;
  const x = (i: number) => (i / Math.max(historySeconds - 1, 1)) * sparklineWidth;
  const y = (value: number) => sparklineHeight - (value / max) * sparklineHeight;
  let path = '';
  let drawing = false;
  values.forEach((value, i) => {
    if (value === null) {
      drawing = false;

      return;
    }
    path += `${drawing ? 'L' : 'M'}${x(i).toFixed(1)},${y(value).toFixed(1)}`;
    drawing = true;
  });
  const onMove = (event: PointerEvent<SVGSVGElement>) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (rect === undefined || rect.width === 0) {
      return;
    }
    const ratio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
    setHover(Math.round(ratio * (historySeconds - 1)));
  };
  const hovered = hover === null ? null : (values[hover] ?? null);

  return (
    <div className={classes.sparkline}>
      <svg
        ref={svgRef}
        viewBox={`0 0 ${sparklineWidth} ${sparklineHeight}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={t('player.streamInfo.bitrateHistory', { seconds: historySeconds })}
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
      >
        <line
          className={classes.baseline}
          x1={0}
          x2={sparklineWidth}
          y1={sparklineHeight}
          y2={sparklineHeight}
        />
        <path className={classes.line} d={path} />
        {hover !== null && (
          <line
            className={classes.crosshair}
            x1={x(hover)}
            x2={x(hover)}
            y1={0}
            y2={sparklineHeight}
          />
        )}
      </svg>
      <Text size="xs" c="dimmed" className={classes.caption}>
        {hover === null
          ? t('player.streamInfo.bitrateHistory', { seconds: historySeconds })
          : `${
              hover === historySeconds - 1
                ? t('player.streamInfo.now')
                : t('player.streamInfo.secondsAgo', { seconds: historySeconds - 1 - hover })
            }: ${hovered === null ? '-' : formatBitrate(hovered)}`}
      </Text>
    </div>
  );
}
