import {
  Alert,
  Anchor,
  Card,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import type { liveServicesQuery } from './__generated__/liveServicesQuery.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';

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
      <SimpleGrid cols={{ base: 1, xs: 2, md: 3, lg: 4 }} spacing="sm">
        {data.guide
          .filter((row) => row.service.channelType.type === type)
          .map((row) => (
            <Card
              key={row.service.id}
              withBorder
              padding="sm"
              renderRoot={(props) => (
                <Link {...props} to="/live/$serviceId" params={{ serviceId: row.service.id }} />
              )}
            >
              <Text fw={700} size="sm" truncate>
                <ServiceLabel name={row.service.name} logoUrl={row.service.logoUrl} />
              </Text>
              <Text size="xs" c="dimmed" lineClamp={2}>
                {row.programs[0]?.name ?? t('live.noProgram')}
              </Text>
            </Card>
          ))}
      </SimpleGrid>
    </Stack>
  );
}
