import { Alert, Stack, Text } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';

import { Dashboard, type DashboardRange } from '../home/Dashboard';
import { useTranslation } from 'react-i18next';

import { authApi } from '../auth/api';
import { ErrorBoundary } from '../futon';
import { isUnauthenticated } from '../relay/environment';

export const Route = createFileRoute('/')({
  component: Home,
});

// upcomingDays は、ホームに出す、これからの予約と予約の競合の期間 (日)。
const upcomingDays = 7;

function Home() {
  const { t } = useTranslation();
  // 時刻は、待たない親で決める。待つ子で決めると、待つたびに決め直して、読み込みが終わらなくなる。
  const [range] = useState<DashboardRange>(() => {
    const now = Date.now();

    return {
      now,
      from: new Date(now).toISOString(),
      // 放送中の番組は、今に重なる番組を引けば足りる。
      nowEnd: new Date(now + 1000).toISOString(),
      to: new Date(now + upcomingDays * 24 * 60 * 60 * 1000).toISOString(),
    };
  });

  return (
    <Stack>
      <ErrorBoundary
        fallback={(error) =>
          // オフラインでも、Service Worker が外枠を返すので、ここまでは表示される。
          !navigator.onLine ? (
            <Alert color="amber">{t('viewer.offline')}</Alert>
          ) : isUnauthenticated(error) ? (
            <SignedOut />
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Dashboard range={range} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// SignedOut は、ログインしていないときに、初回セットアップかログインの画面に移る (設計書 9.2)。
function SignedOut() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  useEffect(() => {
    authApi.setupStatus().then(
      // 戻るボタンで、ここに戻ってまた移る、を繰り返さないよう、履歴を置き換える。
      (status) => navigate({ to: status.required ? '/setup' : '/login', replace: true }),
      () => navigate({ to: '/login', replace: true }),
    );
  }, [navigate]);

  return <Text c="dimmed">{t('viewer.signedOut')}</Text>;
}
