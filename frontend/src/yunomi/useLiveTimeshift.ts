import { useEffect, useRef, useState } from 'react';

import { useNow } from '../futon';
import { broadcastTime, recordingPosition, type Gap } from './position';
import { useHlsStream } from './useHlsStream';
import { useSeekableStream, type SeekableStream } from './useSeekableStream';

// TimeshiftBuffer は、サービスのタイムシフトのバッファの、再生できる範囲 (設計書 5.7)。
export type TimeshiftBuffer = {
  from: string;
  gaps: readonly Gap[];
  streamUrl: string;
  hlsUrl: string;
};

// TimeshiftMode は、バッファを再生する経路。convert はブラウザの中で変換する TS、hls は kyusu の仮想 HLS (設計書 7.3)。
// null なら、バッファを再生できない。
export type TimeshiftMode = 'convert' | 'hls' | null;

// liveThresholdSeconds は、ライブの端からこれより近い位置へシークしたら、バッファでなくライブに戻る秒。
export const liveThresholdSeconds = 5;
// resumeBehindMs は、ライブを一時停止してから、これより長く止めていたら、再開するときに止めた位置からバッファで続ける時間。
export const resumeBehindMs = 3_000;

export type LiveTimeshift = {
  // available は、バッファへシークできるかどうか。
  available: boolean;
  // behind は、ライブでなく、バッファを再生しているかどうか。
  behind: boolean;
  // stream は、ブラウザの中で変換するバッファの配信。convert の経路で behind の間だけ使う。
  stream: SeekableStream;
  // frame は、今の起点にしているバッファ。hls の経路で、仮想 HLS の URL を作るのに使う。
  frame: TimeshiftBuffer | null;
  // hlsStart と hlsLoads は、hls の経路で、仮想 HLS を読み始める位置と、読み直した回数。読み直すたびに URL を変える。
  hlsStart: number;
  hlsLoads: number;
  // positionRef は、描画を待たずに読む今の再生位置。
  positionRef: { current: number };
  // currentPosition は、video の時刻から求め直した今の再生位置。
  currentPosition: () => number;
  // edge は、ライブの端の位置 (バッファの始まりからの秒)。
  edge: number;
  // position は、今の再生位置。ライブなら edge。
  position: number;
  // seekTo は、位置へ移る。ライブの端に近ければ、ライブに戻る。
  seekTo: (seconds: number) => void;
  // restart は、位置からバッファを読み直す。ライブの端に近くても、ライブに戻らない。
  restart: (seconds: number) => void;
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
// mode が null (バッファを再生できない経路) なら、シークさせない。kyusu の経路 (hls) では、仮想 HLS の時刻をそのまま位置にする。
export function useLiveTimeshift(
  buffer: TimeshiftBuffer | null,
  video: HTMLVideoElement | null,
  mode: TimeshiftMode,
): LiveTimeshift {
  // pinned は、バッファを再生している間の、起点にするバッファ。ライブなら null。
  const [pinned, setPinned] = useState<TimeshiftBuffer | null>(null);
  const available = mode !== null && buffer !== null;
  // 経路を替えてバッファを再生できなくなったら、ライブに戻ったものとして扱う。
  const behind = available && pinned !== null;
  const frame = behind ? pinned : buffer;
  const stream = useSeekableStream(frame?.streamUrl ?? '', video, 0, behind && mode === 'convert');
  const hlsBehind = behind && mode === 'hls';
  const hls = useHlsStream(video, hlsBehind);
  const now = useNow(1000);
  const positionAt = (at: number) =>
    frame === null ? 0 : recordingPosition(frame.from, frame.gaps, at);
  const clockAt = (seconds: number) =>
    frame === null ? Date.now() : broadcastTime(frame.from, frame.gaps, seconds);
  const edge = positionAt(now);
  // restart は、今の起点での位置 seconds から、バッファを読み直す。読み直すたびに、起点を最新のバッファに移す。
  // 古いチャンクを破棄すると、配信は残ったチャンクから流すので、破棄した範囲の途切れた区間を、起点のバッファと数え違えるためである。
  const restart = (seconds: number) => {
    if (!available || buffer === null) {
      return;
    }
    const at = clockAt(seconds);
    setPinned(buffer);
    const start = Math.max(recordingPosition(buffer.from, buffer.gaps, at), 0);
    if (mode === 'hls') {
      hls.restartAt(start);

      return;
    }
    stream.restartAt(start);
  };
  const seekTo = (seconds: number) => {
    if (!available) {
      return;
    }
    if (seconds >= positionAt(Date.now()) - liveThresholdSeconds) {
      setPinned(null);

      return;
    }
    restart(seconds);
  };
  // ライブを一時停止して、しばらくしてから再開したら、止めた位置からバッファで続ける。止めた時間がライブの端に近くても、
  // ライブに戻さない。
  const restartRef = useRef(restart);
  restartRef.current = restart;
  const positionAtRef = useRef(positionAt);
  positionAtRef.current = positionAt;
  const live = !behind && available;
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
        restartRef.current(positionAtRef.current(pausedAt));
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

  const positionRef = hlsBehind ? hls.positionRef : stream.positionRef;

  return {
    available,
    behind,
    stream,
    frame,
    hlsStart: hls.start,
    hlsLoads: hls.loads,
    positionRef,
    currentPosition: hlsBehind ? hls.currentPosition : stream.currentPosition,
    edge,
    position: behind ? (hlsBehind ? hls.position : stream.position) : edge,
    seekTo,
    restart,
    goLive: () => setPinned(null),
    positionAt,
    clockAt,
  };
}
