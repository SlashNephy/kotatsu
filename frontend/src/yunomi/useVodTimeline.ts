import type { PlayerState } from '@mpeg2toh264/player';
import { type RefObject, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useStreamsDrains } from '../changes/DataChanges';
import type { PlayerMedia } from './playerMedia';
import { canRetryImmediately, endedEarly, skipBrokenMedia } from './reconnect';
import type { HandoverLoaded } from './TsPlayer';
import { type HlsStream, useHlsStream } from './useHlsStream';
import { type ResolvedStream, type SeekableStream, useSeekableStream } from './useSeekableStream';

// 録画、タイムシフトのバッファ、ライブラリのファイルのように、位置を指定して読み直せる再生元の、位置とシークと
// 読み直しをまとめる (#2244)。変換の経路は start=秒 を付けた配信 (設計書 7.2)、kyusu の経路は仮想 HLS (設計書 7.3) を流す。

// VodTimelineOptions は、再生元ごとに変わる、位置とシークの扱い。
export type VodTimelineOptions = {
  // streamUrl は、変換の経路で流す配信。kyusu の経路では使わない。
  streamUrl: string;
  media: PlayerMedia | null;
  mediaRef: RefObject<PlayerMedia | null>;
  // convert と hls は、今の経路がブラウザの中での変換か、kyusu の仮想 HLS か。どちらでもなければ、位置を動かさない。
  convert: boolean;
  hls: boolean;
  // native は、変換の経路で、映像の要素が自分でシークするか。端末に保存した録画は、ファイルを Range で読むので、
  // start=秒 を付けて読み直さずに、kyusu の経路と同じく映像の時刻を移す (設計書 7.8)。
  native?: boolean;
  // local は、端末に保存した録画か。tenban を通らないので、止まる前の通知で読み直さず、読み終えたのを切れたとみなさない。
  local?: boolean;
  // handover は、tenban が止まる前の通知で、裏で読み込んでから切り替えるか (#1085)。既定は true。
  // 裏で読み込めないプレイヤー (この端末でデコードする経路) では false にし、すぐ読み直す。
  handover?: boolean;
  // resolve は、変換の経路で読み直すときに、start=秒 を付けた配信の代わりに読む URL を決める。端末に保存した録画を、
  // この端末でデコードする経路で、ファイルを切り出して読ませるのに使う (#2166)。
  resolve?: (seconds: number) => Promise<ResolvedStream>;
  // initial は、最初に読み始める位置 (秒)。
  initial: number;
  // duration は、再生できる長さ (秒)。途中で切れたかを、読み込んだ範囲の終わりと比較して見分けるのに使う。
  duration: number;
  // clampSeek は、移れる範囲に収めた位置を返す。既定は 0 から duration まで。
  clampSeek?: (seconds: number) => number;
  // restartFrom は、読み直す位置を、映像が入る位置に寄せる。録画では、終わりの直前を避ける (#1576)。既定はそのまま。
  restartFrom?: (seconds: number) => number;
  // endKnown は、長さが決まっていて、途中で切れたかを見分けられるか。ライブラリのファイルでインデックスを作っている間は、
  // 長さが見込みなので false にする。
  endKnown?: boolean;
};

// VodTimeline は、useVodTimeline が返す、位置とシークと読み直しの口。
export type VodTimeline = {
  stream: SeekableStream;
  hlsStream: HlsStream;
  // position と positionRef と currentPosition は、今の経路の再生位置 (秒)。
  position: number;
  positionRef: { current: number };
  currentPosition: () => number;
  // restartAt は、指定の位置から読み直す。kyusu の経路では、hls.js を作り直す。
  restartAt: (seconds: number) => void;
  // jumpTo は、指定の位置に移り、実際に移った先を返す。kyusu の経路は、プレイリストの中を移るだけで、読み直さない。
  jumpTo: (target: number) => number;
  // retry は、今の位置から読み直す。エラーの通知も消す。
  retry: () => void;
  // error は、続けて切れて、利用者に任せることにしたときのエラーの文。
  error: string | null;
  // handover は、次の読み直しで、今の映像を流したまま裏で読み込んで切り替えるかどうか (#1085)。TsPlayer に渡す。
  handover: boolean;
  // onError は、配信が切れたときに呼ぶ。通知を出さずに今の位置から読み直し、続けて切れるならエラーにする (#1083)。
  onError: () => void;
  // onConvertState は、変換の配信が、終わりに届く前に終わったら、途中で切れたとみなして読み直す。
  onConvertState: (state: PlayerState) => void;
  // onConvertLoaded と onHlsLoaded は、最初のコマが出たときに呼ぶ。
  onConvertLoaded: (handover?: HandoverLoaded) => void;
  onHlsLoaded: () => void;
};

// useVodTimeline は、位置を指定して読み直せる再生元の、位置とシークと読み直しを用意する。
export function useVodTimeline({
  streamUrl,
  media,
  mediaRef,
  convert,
  hls,
  native = false,
  local = false,
  handover: canHandover = true,
  resolve,
  initial,
  duration,
  clampSeek = (seconds) => Math.min(Math.max(seconds, 0), duration),
  restartFrom = (seconds) => seconds,
  endKnown = true,
}: VodTimelineOptions): VodTimeline {
  const { t } = useTranslation();
  // seeksItself は、映像の要素が自分でシークする経路か。位置は映像の時刻をそのまま使い、読み直すときは読み始める位置を渡す。
  const seeksItself = hls || (convert && native);
  const stream = useSeekableStream(streamUrl, media, initial, convert && !native, resolve);
  const hlsStream = useHlsStream(media, seeksItself, initial);
  const { position, positionRef, currentPosition } = seeksItself ? hlsStream : stream;
  const [error, setError] = useState<string | null>(null);
  // lastAutoRetry は、切れたときに、通知を出さずに今の位置から読み直した時刻 (#1083)。
  const lastAutoRetry = useRef<number | null>(null);
  // lastBrokenSkip は、壊れた所を最後に飛び越えた先の位置。
  const lastBrokenSkip = useRef<number | null>(null);
  // handover は、tenban が止まる前の通知で読み直すときだけ使い、シークなどでは、すぐ読み直す。
  const [handover, setHandover] = useState(false);

  const restartAt = (target: number) => {
    const seconds = restartFrom(target);
    setHandover(false);
    // 画質を変えると、変換と kyusu の経路が入れ替わることがある。入れ替わった先の経路も、この位置から読み始める。
    hlsStream.restartAt(seconds);
    stream.restartAt(seconds);
  };
  // tenban が止まる前に通知してきたら、変換の経路では、今の位置から、新しい接続を張ってから切り替える (#1085)。
  // kyusu の経路は、セグメントごとに読み、次のセグメントは止まらない tenban に届くので、つなぎ直さない (設計書 11.4、#1727)。
  const drains = useStreamsDrains();
  const seenDrains = useRef(drains);
  useEffect(() => {
    if (drains === seenDrains.current) {
      return;
    }
    seenDrains.current = drains;
    // エラーで止まっている間は、「やり直す」に任せる。端末に保存した録画は、tenban を通らない。
    if (convert && !native && !local && error === null) {
      if (!canHandover) {
        restartAt(positionRef.current);

        return;
      }
      // 映っている映像の位置を信じられるなら、裏で読み込んでいる間も、その映像で位置を進める (#1728)。
      stream.restartAt(restartFrom(positionRef.current), true);
      setHandover(true);
    }
    // 読み直しの位置は ref から読むので、通知が届いたときだけ動かす。
  }, [drains, convert, native, local, canHandover, error]);
  const retry = () => {
    setError(null);
    restartAt(positionRef.current);
  };
  const jumpTo = (target: number) => {
    const seconds = clampSeek(target);
    const current = mediaRef.current;
    if (seeksItself && current !== null) {
      current.seek(seconds);
      hlsStream.seekTo(seconds);

      return seconds;
    }
    restartAt(seconds);

    return seconds;
  };
  const onError = () => {
    // 放送の受信で壊れた音声のフレームなどで、映像の要素が復号に失敗して止まったなら、壊れた所を飛び越えて読み直す (#1131)。
    // 同じ位置から読み直すと、同じフレームでまた止まる。
    if (mediaRef.current?.error != null) {
      const next = skipBrokenMedia(positionRef.current, lastBrokenSkip.current);
      if (next !== null) {
        lastBrokenSkip.current = next;
        restartAt(clampSeek(next));

        return;
      }
    }
    const now = Date.now();
    if (!canRetryImmediately(lastAutoRetry.current, now)) {
      setError(t('player.error'));

      return;
    }
    lastAutoRetry.current = now;
    restartAt(positionRef.current);
  };
  const onConvertState = (state: PlayerState) => {
    const current = mediaRef.current;
    // 端末に保存した録画は、ファイルを読み終えても切れたのではないので、見分けない。
    if (state !== 'completed' || current === null || !endKnown || native || local) {
      return;
    }
    const { buffered } = current;
    const end =
      buffered.length === 0
        ? positionRef.current
        : stream.positionOf(buffered.end(buffered.length - 1));
    if (endedEarly(end, duration)) {
      onError();
    }
  };

  return {
    stream,
    hlsStream,
    position,
    positionRef,
    currentPosition,
    restartAt,
    jumpTo,
    retry,
    error,
    handover,
    onError,
    onConvertState,
    onConvertLoaded: (handed) => {
      setError(null);
      // 映像の要素が自分でシークする経路は、読み直したら、読み始める位置へ移る。この端末でデコードする経路は、
      // シークできないので、頭から流す。
      if (native) {
        const current = mediaRef.current;
        if (hlsStream.start > 0 && current !== null && current.seekMode !== 'none') {
          current.seek(hlsStream.start);
        }

        return;
      }
      // 最初のコマが出たときの時刻を、位置の基準にする。裏で読み込んで切り替えたら、TsPlayer が渡す時刻にする (#1728)。
      stream.onLoaded(handed);
    },
    onHlsLoaded: () => setError(null),
  };
}
