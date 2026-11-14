import { Alert, Anchor } from '@mantine/core';
import { Link, useRouterState } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { ErrorBoundary } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { LineupBannerQuery } from './__generated__/LineupBannerQuery.graphql';
import { recentLineupAlerts } from './lineupAlerts';

// systemPath は、チャンネルの変更を確かめるシステムの状態の画面。開くと、お知らせを既読にする。
const systemPath = '/admin/system';

// LineupBanner は、衛星の局の顔ぶれがチューナーサーバーと食い違ったこと (チャンネルの変更、#1519) を、画面の上の 1 行で知らせる。
// 知らせは管理者にだけ届くので、ほかのユーザーには出ない。システムの状態の画面を開くまで、出しておく長さの間だけ出す。
export function LineupBanner() {
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  if (pathname === systemPath) {
    // システムの状態の画面には、同じ知らせを出し、開いたら既読にする。
    return null;
  }

  // 読めなくても、本文は出す。画面を移ったら、読み直す。
  return (
    <ErrorBoundary resetKey={pathname} fallback={() => null}>
      <Suspense fallback={null}>
        <LineupBannerContent pathname={pathname} />
      </Suspense>
    </ErrorBoundary>
  );
}

function LineupBannerContent({ pathname }: { pathname: string }) {
  const { t } = useTranslation();
  // 画面を移るたびに読み直し、システムの状態の画面で既読にしたものを消す。
  const data = useLazyLoadQuery<LineupBannerQuery>(
    graphql`
      query LineupBannerQuery {
        notifications(first: 5, unreadOnly: true, kinds: [LINEUP_CHANGED]) {
          edges {
            node {
              id
              message
              occurredAt
            }
          }
        }
      }
    `,
    {},
    { fetchKey: pathname, fetchPolicy: 'store-and-network' },
  );
  const [now] = useState(Date.now);
  const [latest] = recentLineupAlerts(
    data.notifications.edges.map(({ node }) => node),
    now,
  );
  if (latest === undefined) {
    return null;
  }

  return (
    <Alert color="amber" mb="sm" role="status">
      {latest.message}{' '}
      <Anchor component={Link} to={systemPath} size="sm">
        {t('notifications.lineup.open')}
      </Anchor>
    </Alert>
  );
}
