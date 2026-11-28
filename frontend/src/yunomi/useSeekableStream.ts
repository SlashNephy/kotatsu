import { useEffect, useRef, useState } from 'react';

import {
  advanceTarget,
  isBuffered,
  parseStreamStart,
  playbackPosition,
  skipJump,
} from './position';
import type { HandoverLoaded } from './TsPlayer';
import type { PlayerMedia } from './playerMedia';

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
  // framePosition は、映ったコマの時刻 t に当たる再生位置。読み直していて、まだ位置を信じられないときは null。
  // CM を自動で飛ばす判定 (#1512) に、コマごとの位置を渡すのに使う。映ったコマごとに呼ぶ。
  framePosition: (t: number) => number | null;
  // settled は、流し始めた位置を引き終えたかどうか。引き終えるまでは、コマの位置を信じない (#1512)。
  settled: boolean;
  // onLoaded は、最初のフレームが出たときに呼ぶ。そのときの video の時刻を、位置の基準にする。
  // 裏で読み込んで切り替えたときは、TsPlayer が渡す時刻を基準にする。
  onLoaded: (handover?: HandoverLoaded) => void;
  // handoverTarget は、裏で読み込んだ映像を、切り替える前に進める先の video の時刻 (TsPlayer の handoverTarget)。
  handoverTarget: (base: number) => number | null;
};

// positionEpsilon は、求めた位置に届いたかを比較するときの、計算の丸めの誤差 (秒)。
const positionEpsilon = 0.001;

// useSeekableStream は、streamUrl に start=秒 を付けて流す配信の、再生位置を追う。
// 配信は、指定した位置の手前の I フレームから流し、実際に流し始めた位置を X-Kotatsu-Start で返す (HEAD でも返す)。
// ブラウザの中で変換した映像の時刻は 0 から始まるとは限らず、途切れた区間で飛ぶので、最初のフレームの時刻を基準にし、飛んだ分は数えない。
// 配信は求めた位置の手前の I フレームから流れるので、求めた位置以上のコマが映るまでは、位置を動かさない (#1512)。
// enabled が false の間は、HEAD を送らず、位置も動かさない。
export function useSeekableStream(
  streamUrl: string,
  media: PlayerMedia | null,
  initial: number,
  enabled = true,
): SeekableStream {
  // offset は、今の読み込みを始めた位置。loads は、読み込みを始め直した回数で、同じ位置へ読み直したときも URL を変える。
  const [offset, setOffset] = useState(initial);
  const [loads, setLoads] = useState(0);
  // streamStart は、配信が実際に流し始めた位置。分かるまでは offset を使う。
  // at が null なのは、HEAD に答えたが位置を返さなかったか、HEAD に失敗したとき。そのときは、求めた位置から数える。
  const [streamStart, setStreamStart] = useState<{ key: string; at: number | null } | null>(null);
  const streamKey = `${offset}:${loads}`;
  const settled = streamStart?.key === streamKey;
  const startedAt = settled && streamStart.at !== null ? streamStart.at : offset;
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
        setStreamStart({ key: streamKey, at });
      })
      .catch(() => {
        // 読み込みを始め直して止めたのでなければ、位置が分からないまま、求めた位置から数える。
        if (!controller.signal.aborted) {
          setStreamStart({ key: streamKey, at: null });
        }
      });

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
  // settleBase は、読み込んだ映像の最初のコマが出たら、その時刻を基準にする。mediaTime は、コマごとの通知の、そのコマの時刻。
  // timeupdate を待つと、その間に進んだ分だけ基準が後ろへずれ、位置を少なく数える。
  const settleBase = (current: PlayerMedia, mediaTime: number | null) => {
    if (
      phase.current !== 'awaitingBase' ||
      current.readyState < HTMLMediaElement.HAVE_CURRENT_DATA
    ) {
      return;
    }
    base.current = mediaTime ?? current.currentTime;
    lastVideoTime.current = base.current;
    phase.current = 'playing';
  };
  // reached は、求めた位置以上のコマが映ったと確認した読み込み (streamKey)。
  const reached = useRef<string | null>(null);
  // reachedStart は、今の読み込みで、求めた位置以上のコマが映ったかを返す。まだなら、映っているコマの位置を信じない。
  // 最初のコマが求めた位置より手前なら、その位置へ currentTime を進め、手前のコマを再生する時間を省く。読み込んである範囲の
  // 外へ進めると、変換のプレイヤーが配信を読み直すので、届くのを待ってから進める。
  const reachedStart = (current: PlayerMedia, time: number) => {
    // 裏で読み込んでいる間に映っているのは、前の読み込みの映像で、その位置は確認済み。
    if (reached.current === streamKey || handoverFrom.current !== null) {
      return true;
    }
    if (!settled || phase.current !== 'playing') {
      return false;
    }
    const target = advanceTarget(base.current, startedAt, offset);
    if (!current.seeking && time >= target - positionEpsilon) {
      reached.current = streamKey;

      return true;
    }
    // シークできない経路 (この端末でデコードする経路) は、流れて届くのを待つ。
    if (
      current.seekMode !== 'none' &&
      !current.seeking &&
      current.currentTime < target &&
      isBuffered(current.buffered, target)
    ) {
      current.seek(target);
      // 進めた分を、途切れた区間を越えたとみなさない。
      lastVideoTime.current = target;
    }

    return false;
  };
  useEffect(() => {
    if (media === null || !enabled) {
      return;
    }
    const update = () => {
      settleBase(media, null);
      // 読み直した先の手前のコマ (I フレームから目標まで) の間は、位置を動かさない (#1512)。
      if (phase.current !== 'playing' || !reachedStart(media, media.currentTime)) {
        return;
      }
      const next = converted(media.currentTime);
      positionRef.current = next;
      setPosition(next);
    };
    media.addEventListener('timeupdate', update);

    return () => media.removeEventListener('timeupdate', update);
    // converted と reachedStart は、startedAt と streamKey と settled で決まる。
  }, [startedAt, streamKey, settled, media, enabled]);

  return {
    url: `${start}#${loads}`,
    position,
    positionRef,
    restartAt: (seconds, handover = false) => {
      // 映っている映像の位置を信じられるなら、裏で読み込んでいる間も、その映像で位置を進める。
      const tracking =
        handover &&
        phase.current === 'playing' &&
        (handoverFrom.current !== null || reached.current === streamKey);
      // 裏で読み込んでいる間に、もう一度通知が届いたら、映っている映像の起点のまま続ける。
      handoverFrom.current = tracking ? (handoverFrom.current ?? startedAt) : null;
      phase.current = tracking ? 'playing' : 'loading';
      positionRef.current = seconds;
      setPosition(seconds);
      setOffset(seconds);
      setLoads((n) => n + 1);
    },
    currentPosition: () =>
      media === null || phase.current !== 'playing'
        ? positionRef.current
        : converted(media.currentTime),
    positionOf: (t) => playbackPosition(startedAt, base.current, t),
    framePosition: (t) => {
      if (media === null) {
        return null;
      }
      settleBase(media, t);

      return phase.current === 'playing' && reachedStart(media, t) ? converted(t) : null;
    },
    settled,
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
      // 新しい読み込みが流し始めた位置が分かるまでは、進める先が分からない。
      handoverFrom.current === null || !settled || media === null
        ? null
        : advanceTarget(first, startedAt, converted(media.currentTime)),
  };
}
