import { useComputedColorScheme } from '@mantine/core';
import { useEffect } from 'react';

import { kogecha, mikan } from './theme';

// テーマごとの theme-color。ライトはみかん色、ダークは背景の焦げ茶にする。
const themeColors = {
  light: mikan[6],
  dark: kogecha[7],
} as const;

// useThemeColor は、<meta name="theme-color"> を、表示中のテーマに合わせて書き換える。
// Android にインストールした PWA を Chrome が edge-to-edge で描くときは、帯は透明になり、ボタンの明暗をこの色から決める。
// edge-to-edge でないときは、Chrome が帯をマニフェストの theme_color で塗り、この色は帯に効かない (#425)。
// アプリ内でテーマを切り替えたときも追従する。
export function useThemeColor(): void {
  const colorScheme = useComputedColorScheme('light');

  useEffect(() => {
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', themeColors[colorScheme]);
  }, [colorScheme]);
}
