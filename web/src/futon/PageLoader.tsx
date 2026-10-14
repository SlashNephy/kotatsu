import { Loader, VisuallyHidden } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import classes from './PageLoader.module.css';

// PageLoader は、画面を読み込んでいる間に出す、全画面のスピナー。Suspense の fallback に使う。
// 読み上げでは「読み込んでいます」と伝える。
export function PageLoader() {
  const { t } = useTranslation();

  return (
    <div className={classes.root} role="status">
      <Loader size="lg" />
      <VisuallyHidden>{t('viewer.loading')}</VisuallyHidden>
    </div>
  );
}
