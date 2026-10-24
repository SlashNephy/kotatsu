import { Alert, Anchor, Stack, Title } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { Suspense, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { ErrorBoundary, PageLoader } from '../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../relay/environment';

// AdminPage は、管理者向けの画面の枠 (設計書 10.3)。設定に戻るリンクと見出しを出し、
// 読み込み中と、ログインしていないとき、管理者でないときの表示をまとめる。
export function AdminPage({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{title}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : graphQLErrorDetails(error).code === 'FORBIDDEN' ? (
            <Alert color="amber">{t('admin.forbidden')}</Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>{children}</Suspense>
      </ErrorBoundary>
    </Stack>
  );
}
