// 映像の明るさ (#1598、設計書 7.5)。ブラウザから OS の明るさは変えられないので、映像と字幕に CSS の
// filter: brightness() を掛けて代わりにする。キャプチャは video から写すので、明るさは焼き込まない。

// minBrightness と maxBrightness は、変えられる明るさの範囲。上げすぎると白く飛び、下げすぎると見えない。
export const minBrightness = 0.5;
export const maxBrightness = 1.5;
// brightnessStep は、スワイプ 1 段で変える明るさ。
export const brightnessStep = 0.1;

// stepBrightness は、明るさ brightness を steps 段変えた値を返す。範囲に収め、浮動小数の誤差を 1% で丸める。
export function stepBrightness(brightness: number, steps: number): number {
  const next = Math.min(
    maxBrightness,
    Math.max(minBrightness, brightness + steps * brightnessStep),
  );

  return Math.round(next * 100) / 100;
}

// brightnessStyle は、映像と字幕を包む要素に掛ける CSS の filter を返す。そのままの明るさなら空。
export function brightnessStyle(brightness: number): { filter?: string } {
  return brightness === 1 ? {} : { filter: `brightness(${brightness})` };
}
