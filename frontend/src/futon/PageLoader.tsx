import { Loader, VisuallyHidden } from '@mantine/core';
import { useLayoutEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import classes from './PageLoader.module.css';

// PageLoader は、画面を読み込んでいる間に出すスピナー。Suspense の fallback に使い、読み込んでいる部分の中に出す。
// 題名やタブより下の、画面に見えている残りの部分の、縦横の中央に出す。
// 読み上げでは「読み込んでいます」と伝える。
export function PageLoader() {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);

  // 残りの高さを CSS で決めるため、置かれた位置の、画面の上からの距離を --page-loader-top に入れる。
  // 描く前に入れて、仮の高さで描いてから跳ねるのを避ける。
  useLayoutEffect(() => {
    const element = ref.current;
    if (element === null) {
      return;
    }
    const measure = () => {
      element.style.setProperty('--page-loader-top', `${element.getBoundingClientRect().top}px`);
    };
    measure();
    window.addEventListener('resize', measure);
    return () => window.removeEventListener('resize', measure);
  }, []);

  return (
    <div ref={ref} className={classes.root} role="status">
      <Loader size="lg" />
      <VisuallyHidden>{t('viewer.loading')}</VisuallyHidden>
    </div>
  );
}
