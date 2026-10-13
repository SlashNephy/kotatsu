import { createTheme, type MantineColorsTuple } from '@mantine/core';

// futon は、Mantine に重ねる Kotatsu 用のテーマと部品 (設計書 10.1、10.2)。

// みかん色。ライトテーマのアクセントにする。
const mikan: MantineColorsTuple = [
  '#fff3e6',
  '#ffe4cc',
  '#ffc799',
  '#ffa862',
  '#ff8e35',
  '#ff7d18',
  '#ff7407',
  '#e36200',
  '#ca5600',
  '#b04800',
];

// 琥珀色。警告に使う。赤は、録画の失敗のように問題が起きたときだけ使う。
const amber: MantineColorsTuple = [
  '#fff8e1',
  '#ffefcc',
  '#ffdd9b',
  '#ffca64',
  '#ffba38',
  '#ffb01b',
  '#ffab09',
  '#e39500',
  '#ca8400',
  '#af7100',
];

// 焦げ茶。ダークテーマ (夜のこたつ) の背景と面に使う。dark[7] が背景、dark[6] が面になる。
const kogecha: MantineColorsTuple = [
  '#e9e1da',
  '#c9bcb0',
  '#a3948a',
  '#7d6e64',
  '#5c4f47',
  '#43382f',
  '#352c25',
  '#2a221c',
  '#211a15',
  '#18130f',
];

export const headingFontFamily = "'Zen Maru Gothic', system-ui, sans-serif";

export const theme = createTheme({
  primaryColor: 'mikan',
  primaryShade: { light: 6, dark: 5 },
  colors: { mikan, amber, dark: kogecha },
  // 角丸は大きめにする。
  defaultRadius: 'lg',
  // OS で動きを減らす設定にしていれば、Mantine の部品の動きを止める (設計書 10.2)。
  respectReducedMotion: true,
  // 本文はシステムフォント、見出しは同梱した丸ゴシック体にする。
  fontFamily: 'system-ui, -apple-system, "Hiragino Sans", "Noto Sans JP", sans-serif',
  headings: { fontFamily: headingFontFamily, fontWeight: '700' },
  other: {
    // 生成り色。ライトテーマの背景に使う。
    kinari: '#fbf7ef',
  },
});

export const cssVariablesResolver = () => ({
  variables: {},
  light: {
    '--mantine-color-body': '#fbf7ef',
    // 押せない部品の色。Mantine の既定は青みのある灰色で、生成りの背景から浮くので、生成りに寄せた暖かい灰色にする。
    '--mantine-color-disabled': '#efe8dd',
    '--mantine-color-disabled-color': '#a89c8f',
    '--mantine-color-disabled-border': '#ddd3c5',
  },
  dark: {
    '--mantine-color-body': kogecha[7],
    // Mantine の既定では、押せないボタンの背景が dark[6] で、カードの面と同じ色になり、ボタンに見えない。一段明るくする。
    '--mantine-color-disabled': kogecha[5],
  },
});
