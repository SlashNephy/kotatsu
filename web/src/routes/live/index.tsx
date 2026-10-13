import { Alert, Anchor, SegmentedControl, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import type { liveServicesQuery } from './__generated__/liveServicesQuery.graphql';
import { NowPlayingCard } from '../../guide/NowPlayingCard';
import { sharesWithListed } from '../../guide/sharedEvent';

export const Route = createFileRoute('/live/')({
  component: LivePage,
});

// LivePage は、ライブ視聴のチャンネルの一覧 (設計書 10.3)。
function LivePage() {
  const { t } = useTranslation();
  // 放送中の番組を出すので、画面を開いた時刻から 1 分間の時間帯を引く。
  // 読み込みの間に子は描き直されて state を失うので、時間帯は、待たない親で決める。
  const [range] = useState(() => {
    const now = Date.now();

    return { from: new Date(now).toISOString(), to: new Date(now + 60_000).toISOString() };
  });

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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Services range={range} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Services({ range }: { range: { from: string; to: string } }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<liveServicesQuery>(
    graphql`
      query liveServicesQuery($from: Time!, $to: Time!) {
        channelTypes {
          type
          displayName
        }
        guide(from: $from, to: $to) {
          service {
            id
            name
            logoUrl
            channelType {
              type
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
    range,
  );
  const types = data.channelTypes.filter((type) =>
    data.guide.some((row) => row.service.channelType.type === type.type),
  );
  const [type, setType] = useState(types[0]?.type ?? '');
  // サブチャンネルがメインチャンネルと同じ番組を流しているときは、メインチャンネルだけを出す (ホームの放送中の番組と同じ)。
  const listed = new Set(data.guide.map((row) => row.service.id));
  const rows = data.guide.filter((row) => !sharesWithListed(row.programs[0], listed));

  return (
    <Stack>
      {types.length > 1 && (
        <SegmentedControl
          size="xs"
          value={type}
          onChange={setType}
          data={types.map((channelType) => ({
            value: channelType.type,
            label: channelType.displayName,
          }))}
          style={{ alignSelf: 'flex-start' }}
        />
      )}
      {types.length === 0 && <Alert color="gray">{t('live.empty')}</Alert>}
      <SimpleGrid cols={{ base: 1, xs: 2, md: 3, lg: 4 }} spacing="xs">
        {rows
          .filter((row) => row.service.channelType.type === type)
          .map((row) => (
            <NowPlayingCard
              key={row.service.id}
              serviceId={row.service.id}
              serviceName={row.service.name}
              serviceLogoUrl={row.service.logoUrl ?? null}
              program={row.programs[0] ?? null}
            />
          ))}
      </SimpleGrid>
    </Stack>
  );
}
