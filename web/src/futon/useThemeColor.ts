import { useComputedColorScheme } from '@mantine/core';
import { useEffect } from 'react';

import { kogecha, mikan } from './theme';

// テーマごとの theme-color。ライトはみかん色、ダークは背景の焦げ茶にする。
const themeColors = {
  light: mikan[6],
  dark: kogecha[7],
} as const;

// useThemeColor は、<meta name="theme-color"> を、表示中のテーマに合わせて書き換える。
// Android にインストールした PWA は、ナビゲーションの帯をこの色で塗る。マニフェストの theme_color は 1 色しか書けず、
// ダークテーマで帯だけがみかん色に浮くので、ページの側から変える (#425)。アプリ内でテーマを切り替えたときも追従する。
export function useThemeColor(): void {
  const colorScheme = useComputedColorScheme('light');

  useEffect(() => {
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', themeColors[colorScheme]);
  }, [colorScheme]);
}
