import { useState } from 'react';

// 早回し (#1572、設計書 7.5)。録画とタイムシフトの再生で、操作バーの「15 秒進む」を押している間だけ、速く再生する。

// fastForwardFactor は、早回しの間に、選んでいる再生速度に掛ける倍率。
export const fastForwardFactor = 2;

// fastForwardEdgeMargin は、伸び続けている再生 (録画中の追っかけ再生、放送中のタイムシフト) で、
// 早回しを効かせない、今の端までの秒。端に追いつくと、まだ届いていない所を読もうとして止まるので、手前で元の速さに戻す。
export const fastForwardEdgeMargin = 10;

type Edge = {
  // growing は、録画中や放送中で、再生できる長さが伸び続けているかどうか。
  growing: boolean;
  position: number;
  duration: number;
};

// canFastForward は、今の位置で早回しを効かせてよいかを返す。伸び続けている再生では、端の手前だけにする。
export function canFastForward({ growing, position, duration }: Edge): boolean {
  return !growing || duration - position > fastForwardEdgeMargin;
}

// useFastForward は、早回しの状態と、video の要素に当てる再生速度を返す。
// rate は、設定で選んだ再生速度。早回しを止めたら、この速さに戻す。
export function useFastForward(rate: number, edge: Edge) {
  const [held, setHeld] = useState(false);
  const active = held && canFastForward(edge);

  return {
    // active は、早回しで再生しているかどうか。押していても、端の手前では false になる。
    active,
    playbackRate: active ? rate * fastForwardFactor : rate,
    // setHeld は、操作バーのボタンを押しているかどうかを知らせる。
    setHeld,
  };
}
