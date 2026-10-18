// 字幕の見た目 (#491)。字幕を描く captions.ts と、プレイヤーの部品の両方が使う。

// CaptionStyle は、字幕の見た目 (#491)。stroke なら、放送の指定によらず黒で縁取る。
// backgroundOpacity は、背景を、放送の指定する色に代えて、この不透明度の黒で塗る。null なら、放送の指定どおり。
export type CaptionStyle = { stroke: boolean; backgroundOpacity: number | null };

export const defaultCaptionStyle: CaptionStyle = { stroke: false, backgroundOpacity: null };

// rendererColor は、字幕の見た目を、aribb24.js の描画器の色の設定 (null なら放送の指定どおり) にする。
export function rendererColor(style: CaptionStyle): {
  stroke: string | null;
  background: string | null;
} {
  return {
    stroke: style.stroke ? 'black' : null,
    background:
      style.backgroundOpacity === null ? null : `rgba(0, 0, 0, ${style.backgroundOpacity})`,
  };
}
