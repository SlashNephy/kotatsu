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

// shouldHoldControls は、操作がなくなって隠す時間になっても、操作バーを出したままにするかどうか。
// マウスが操作バーの上にある間、操作バーの中を押したまま動かしている間 (シークバーを掴んでいる間)、
// 設定や選択肢を開いている間は、使っている最中なので隠さない。
export function shouldHoldControls({
  pointerOverControls,
  pressingControls,
  menuOpened,
}: {
  pointerOverControls: boolean;
  pressingControls: boolean;
  menuOpened: boolean;
}): boolean {
  return pointerOverControls || pressingControls || menuOpened;
}
