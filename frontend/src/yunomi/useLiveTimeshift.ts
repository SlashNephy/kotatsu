import { useEffect, useRef, useState } from 'react';

import { useNow } from '../futon';
import { broadcastTime, recordingPosition, type Gap } from './position';
import { useSeekableStream, type SeekableStream } from './useSeekableStream';

// TimeshiftBuffer は、サービスのタイムシフトのバッファの、再生できる範囲 (設計書 5.7)。
export type TimeshiftBuffer = { from: string; gaps: readonly Gap[]; streamUrl: string };

// liveThresholdSeconds は、ライブの端からこれより近い位置へシークしたら、バッファでなくライブに戻る秒。
export const liveThresholdSeconds = 5;
// resumeBehindMs は、ライブを一時停止してから、これより長く止めていたら、再開するときに止めた位置からバッファで続ける時間。
export const resumeBehindMs = 3_000;

export type LiveTimeshift = {
  // available は、バッファへシークできるかどうか。
  available: boolean;
  // behind は、ライブでなく、バッファを再生しているかどうか。
  behind: boolean;
  // stream は、バッファの配信。behind の間だけ使う。
  stream: SeekableStream;
  // edge は、ライブの端の位置 (バッファの始まりからの秒)。
  edge: number;
  // position は、今の再生位置。ライブなら edge。
  position: number;
  // seekTo は、位置へ移る。ライブの端に近ければ、ライブに戻る。
  seekTo: (seconds: number) => void;
  // goLive は、ライブに戻る。
  goLive: () => void;
  // positionAt は、放送時刻 (Unix ミリ秒) に当たる位置。
  positionAt: (at: number) => number;
  // clockAt は、位置に当たる放送時刻 (Unix ミリ秒)。
  clockAt: (seconds: number) => number;
};

// useLiveTimeshift は、ライブ視聴から、タイムシフトのバッファへシークできるようにする (設計書 5.7)。
// YouTube のライブ配信と同じく、シークバーはバッファの始まりからライブの端までで、過去へシークするとバッファの配信に切り替える。
// バッファへ移ったときのバッファの始まりを起点に持ち続け、古いチャンクが破棄されても、位置をずらさない。
// enabled が false (バッファを再生できない経路) なら、シークさせない。
export function useLiveTimeshift(
  buffer: TimeshiftBuffer | null,
  video: HTMLVideoElement | null,
  enabled: boolean,
): LiveTimeshift {
  // pinned は、バッファを再生している間の、起点にするバッファ。ライブなら null。
  const [pinned, setPinned] = useState<TimeshiftBuffer | null>(null);
  const frame = pinned ?? buffer;
  const stream = useSeekableStream(frame?.streamUrl ?? '', video, 0, pinned !== null);
  const now = useNow(1000);
  const available = enabled && buffer !== null;
  const positionAt = (at: number) =>
    frame === null ? 0 : recordingPosition(frame.from, frame.gaps, at);
  const edge = positionAt(now);
  const seekTo = (seconds: number) => {
    if (!available || frame === null) {
      return;
    }
    if (seconds >= positionAt(Date.now()) - liveThresholdSeconds) {
      setPinned(null);

      return;
    }
    setPinned(frame);
    stream.restartAt(Math.max(seconds, 0));
  };
  // ライブを一時停止して、しばらくしてから再開したら、止めた位置からバッファで続ける。
  const seekToRef = useRef(seekTo);
  seekToRef.current = seekTo;
  const positionAtRef = useRef(positionAt);
  positionAtRef.current = positionAt;
  const live = pinned === null && available;
  useEffect(() => {
    if (video === null || !live) {
      return;
    }
    let pausedAt: number | null = null;
    const pause = () => {
      if (!video.ended) {
        pausedAt = Date.now();
      }
    };
    const play = () => {
      if (pausedAt !== null && Date.now() - pausedAt > resumeBehindMs) {
        seekToRef.current(positionAtRef.current(pausedAt));
      }
      pausedAt = null;
    };
    video.addEventListener('pause', pause);
    video.addEventListener('play', play);

    return () => {
      video.removeEventListener('pause', pause);
      video.removeEventListener('play', play);
    };
  }, [video, live]);

  return {
    available,
    behind: pinned !== null,
    stream,
    edge,
    position: pinned === null ? edge : stream.position,
    seekTo,
    goLive: () => setPinned(null),
    positionAt,
    clockAt: (seconds) =>
      frame === null ? Date.now() : broadcastTime(frame.from, frame.gaps, seconds),
  };
}
