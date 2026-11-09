import { Alert, Anchor, Stack, Title } from '@mantine/core';
import { Tv } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { startTransition, Suspense, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import type { ChangeChannel } from '../../changes/dataChanges';
import {
  EmptyState,
  ErrorBoundary,
  nextWakeAt,
  PageLoader,
  useDocumentTitle,
  useWakeAt,
} from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import type { liveServicesQuery } from './__generated__/liveServicesQuery.graphql';
import { NowPlayingGrid } from '../../guide/NowPlayingGrid';
import { NowPlayingTabs } from '../../guide/NowPlayingTabs';
import { nowPlayingTabs, useNowPlayingTabs } from '../../guide/useNowPlayingTabs';

export const Route = createFileRoute('/live/')({
  component: LivePage,
});

// LivePage は、ライブ視聴のチャンネルの一覧 (設計書 10.3)。
function LivePage() {
  const { t } = useTranslation();
  useDocumentTitle([t('live.title')]);
  // 放送中の番組を出すので、画面を開いた時刻から 1 分間の時間帯を引く。
  // 読み込みの間に子は描き直されて state を失うので、時間帯は、待たない親で決める。
  const [range, setRange] = useState(() => liveRange(Date.now()));
  // 放送中の番組が終わったら、時刻を決め直して読み直す (#866)。読み直す間も一覧を出したままにする。
  const refresh = useCallback(() => startTransition(() => setRange(liveRange(Date.now()))), []);

  return (
    <Stack gap="sm">
      <Title order={2}>{t('live.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Services range={range} onExpire={refresh} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

type LiveRange = { now: number; from: string; to: string };

// liveRange は、now を基準にした、ライブの一覧で読む時間帯。
function liveRange(now: number): LiveRange {
  return { now, from: new Date(now).toISOString(), to: new Date(now + 60_000).toISOString() };
}

// liveChannels は、ライブの一覧に関わるデータの種類。変わったら、読み直す (#866)。
const liveChannels: readonly ChangeChannel[] = ['EPG'];

// maxWaitMs は、時刻で読み直すまでに待つ最大の時間。放送中の番組がないサービスで、番組が始まるのに気づくのに使う。
const maxWaitMs = 10 * 60_000;

function Services({ range, onExpire }: { range: LiveRange; onExpire: () => void }) {
  const { t } = useTranslation();
  const changes = useDataChanges(liveChannels);
  const data = useLazyLoadQuery<liveServicesQuery>(
    graphql`
      query liveServicesQuery($from: Time!, $to: Time!) {
        channelTypes {
          type
          displayName
        }
        pinnedServices {
          id
        }
        guide(from: $from, to: $to) {
          service {
            id
            name
            logoUrl
            channelType {
              type
            }
            jikkyo {
              id
              force
            }
          }
          programs {
            name
            startAt
            endAt
            sharedFrom {
              id
            }
          }
        }
      }
    `,
    { from: range.from, to: range.to },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  // 放送中の番組のうち、最も早く終わる時刻に読み直す。
  useWakeAt(
    nextWakeAt(
      data.guide.flatMap((row) => row.programs.map((program) => program.endAt)),
      range.now,
      maxWaitMs,
    ),
    onExpire,
  );
  // pinned は、ピン留めしたサービスの ID を、ピン留めした順に並べたもの (#485)。
  const pinned = data.pinnedServices.map((service) => service.id);
  const tabs = useNowPlayingTabs(
    nowPlayingTabs(data.channelTypes, data.guide, pinned, {
      pinned: t('pin.tab'),
      force: t('live.forceTab'),
    }),
  );

  return (
    <Stack>
      <NowPlayingTabs state={tabs} style={{ alignSelf: 'flex-start' }} />
      {tabs.tabs.length === 0 && (
        <EmptyState icon={Tv} title={t('live.empty')} hint={t('live.emptyHint')} />
      )}
      <NowPlayingGrid rows={tabs.rows} pinned={pinned} />
    </Stack>
  );
}
