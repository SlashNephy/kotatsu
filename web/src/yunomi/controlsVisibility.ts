// プレイヤーの操作バーを、いつ隠すか。nebula のプレイヤーと揃える。

// controlsHideDelayMs は、操作がなくなってから、操作バーを隠すまでのミリ秒。
export const controlsHideDelayMs = 2500;

// shouldForceControlsVisible は、操作がなくても、操作バーを出し続けるかどうか。
// 止めている間は、次の操作を選ぶ場面なので隠さない。キーボードで操作バーにフォーカスを移している間も、操作している先が
// 見えなくならないよう隠さない。focusWithin は、キーボードでのフォーカス (:focus-visible) のときだけ true にする。
export function shouldForceControlsVisible({
  playing,
  focusWithin,
}: {
  playing: boolean;
  focusWithin: boolean;
}): boolean {
  return !playing || focusWithin;
}
