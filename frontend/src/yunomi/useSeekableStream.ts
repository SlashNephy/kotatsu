import { useEffect, useRef, useState } from 'react';

import { advanceTarget, parseStreamStart, playbackPosition, skipJump } from './position';
import type { HandoverLoaded } from './TsPlayer';

// SeekableStream は、位置を指定して読み直す TS の配信 (録画やタイムシフトのバッファ) の、再生位置の追い方 (設計書 7.2)。
export type SeekableStream = {
  // url は、今の読み込みの URL。読み直すたびに、fragment を変える。
  url: string;
  // position は、再生位置 (起点からの秒)。timeupdate で更新する。
  position: number;
  // positionRef は、描画を待たずに読む再生位置。
  positionRef: { current: number };
  // restartAt は、指定の位置から読み直す。handover なら、裏で読み込んでいる間も、映っている映像で位置を進める (#1728)。
  restartAt: (seconds: number, handover?: boolean) => void;
  // currentPosition は、video の時刻から求め直した今の再生位置。timeupdate より細かく要るときに使う。
  currentPosition: () => number;
  // positionOf は、video の時刻 t に当たる再生位置。読み込んだ範囲の終わりを位置に直すのに使う。
  positionOf: (t: number) => number;
  // onLoaded は、最初のフレームが出たときに呼ぶ。そのときの video の時刻を、位置の基準にする。
  // 裏で読み込んで切り替えたときは、TsPlayer が渡す時刻を基準にする。
  onLoaded: (handover?: HandoverLoaded) => void;
  // handoverTarget は、裏で読み込んだ映像を、切り替える前に進める先の video の時刻 (TsPlayer の handoverTarget)。
  handoverTarget: (base: number) => number | null;
};

// useSeekableStream は、streamUrl に start=秒 を付けて流す配信の、再生位置を追う。
// 配信は、指定した位置の手前の I フレームから流し、実際に流し始めた位置を X-Kotatsu-Start で返す (HEAD でも返す)。
// ブラウザの中で変換した映像の時刻は 0 から始まるとは限らず、途切れた区間で飛ぶので、最初のフレームの時刻を基準にし、飛んだ分は数えない。
// enabled が false の間は、HEAD を送らず、位置も動かさない。
export function useSeekableStream(
  streamUrl: string,
  video: HTMLVideoElement | null,
  initial: number,
  enabled = true,
): SeekableStream {
  // offset は、今の読み込みを始めた位置。loads は、読み込みを始め直した回数で、同じ位置へ読み直したときも URL を変える。
  const [offset, setOffset] = useState(initial);
  const [loads, setLoads] = useState(0);
  // streamStart は、配信が実際に流し始めた位置。分かるまでは offset を使う。
  const [streamStart, setStreamStart] = useState<{ key: string; at: number } | null>(null);
  const streamKey = `${offset}:${loads}`;
  const settled = streamStart?.key === streamKey;
  const startedAt = settled ? streamStart.at : offset;
  const separator = streamUrl.includes('?') ? '&' : '?';
  const start = `${streamUrl}${separator}start=${offset.toFixed(1)}`;
  useEffect(() => {
    if (!enabled) {
      return;
    }
    const controller = new AbortController();
    fetch(start, { method: 'HEAD', credentials: 'same-origin', signal: controller.signal })
      .then((response) => {
        const at = response.ok ? parseStreamStart(response.headers.get('X-Kotatsu-Start')) : null;
        if (at !== null) {
          setStreamStart({ key: streamKey, at });
        }
      })
      .catch(() => {});

    return () => controller.abort();
  }, [enabled, start, streamKey]);
  const base = useRef(0);
  const lastVideoTime = useRef(0);
  // phase は、読み込みの段階。読み直している間に、前の映像の時刻で位置を動かさないよう、基準を決めるまで位置を更新しない。
  const phase = useRef<'loading' | 'awaitingBase' | 'playing'>('loading');
  const [position, setPosition] = useState(initial);
  const positionRef = useRef(initial);
  // handoverFrom は、裏で読み込んでいる間 (#1728) の、映っている映像の読み込みが流し始めた位置。
  // 新しい読み込みの起点は、切り替えるまで使わない。
  const handoverFrom = useRef<number | null>(null);
  const converted = (current: number) => {
    base.current = skipJump(base.current, lastVideoTime.current, current);
    lastVideoTime.current = current;

    return playbackPosition(handoverFrom.current ?? startedAt, base.current, current);
  };
  useEffect(() => {
    if (video === null || !enabled) {
      return;
    }
    const update = () => {
      if (phase.current === 'loading') {
        return;
      }
      if (phase.current === 'awaitingBase') {
        if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) {
          return;
        }
        base.current = video.currentTime;
        lastVideoTime.current = video.currentTime;
        phase.current = 'playing';
      }
      const next = converted(video.currentTime);
      positionRef.current = next;
      setPosition(next);
    };
    video.addEventListener('timeupdate', update);

    return () => video.removeEventListener('timeupdate', update);
    // converted は startedAt で決まる。
  }, [startedAt, video, enabled]);

  return {
    url: `${start}#${loads}`,
    position,
    positionRef,
    restartAt: (seconds, handover = false) => {
      // 映っている映像の位置を信じられるなら、裏で読み込んでいる間も、その映像で位置を進める。
      const tracking = handover && phase.current === 'playing';
      // 裏で読み込んでいる間に、もう一度知らせが届いたら、映っている映像の起点のまま続ける。
      handoverFrom.current = tracking ? (handoverFrom.current ?? startedAt) : null;
      phase.current = tracking ? 'playing' : 'loading';
      positionRef.current = seconds;
      setPosition(seconds);
      setOffset(seconds);
      setLoads((n) => n + 1);
    },
    currentPosition: () =>
      video === null || phase.current !== 'playing'
        ? positionRef.current
        : converted(video.currentTime),
    positionOf: (t) => playbackPosition(startedAt, base.current, t),
    onLoaded: (handover) => {
      const switched = handover !== undefined && handoverFrom.current !== null;
      handoverFrom.current = null;
      if (switched) {
        base.current = handover.base;
        // 切り替える前に進めた分を、途切れた区間を越えたとみなさない。
        lastVideoTime.current = handover.current;
        phase.current = 'playing';

        return;
      }
      phase.current = 'awaitingBase';
    },
    handoverTarget: (first) =>
      handoverFrom.current === null || !settled || video === null
        ? null
        : advanceTarget(first, startedAt, converted(video.currentTime)),
  };
}
