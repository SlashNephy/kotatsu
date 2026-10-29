import { Alert, Anchor, Badge, Button, Group, Paper, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { History, Play } from 'lucide-react';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import {
  EmptyState,
  ErrorBoundary,
  PageLoader,
  ProgramTitle,
  useDocumentTitle,
  zonedFormat,
} from '../../futon';
import { ServiceLabel } from '../../guide/ServiceLabel';
import { isUnauthenticated } from '../../relay/environment';
import type { ServiceIdTimeshiftQuery } from './__generated__/ServiceIdTimeshiftQuery.graphql';

export const Route = createFileRoute('/timeshift/$serviceId')({
  component: TimeshiftPage,
});

const startFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const endFormat = zonedFormat({ hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });

// TimeshiftPage は、サービスのタイムシフトのバッファにある番組の一覧 (設計書 5.7)。押すと、その番組を頭から再生する。
function TimeshiftPage() {
  const { t } = useTranslation();
  const { serviceId } = Route.useParams();

  return (
    <Stack gap="sm">
      <Anchor
        size="sm"
        renderRoot={(props) => <Link {...props} to="/live/$serviceId" params={{ serviceId }} />}
      >
        {t('timeshift.backToLive')}
      </Anchor>
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
          <ProgramList serviceId={serviceId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function ProgramList({ serviceId }: { serviceId: string }) {
  const { t } = useTranslation();
  const { node } = useLazyLoadQuery<ServiceIdTimeshiftQuery>(
    graphql`
      query ServiceIdTimeshiftQuery($id: ID!) {
        node(id: $id) {
          __typename
          ... on Service {
            name
            logoUrl
            timeshiftPrograms {
              name
              startAt
              endAt
              playableFrom
            }
          }
        }
      }
    `,
    { id: serviceId },
    // バッファは刻々と伸び、古い番組は破棄されるので、開くたびに読み直す。
    { fetchPolicy: 'store-and-network' },
  );
  useDocumentTitle([t('timeshift.title')]);
  if (node?.__typename !== 'Service') {
    return <Alert color="amber">{t('live.notFound')}</Alert>;
  }
  // 新しい番組を上に並べる。
  const programs = [...node.timeshiftPrograms].reverse();

  return (
    <Stack gap="sm">
      <Stack gap={4}>
        <Title order={2} size="h3">
          {t('timeshift.title')}
        </Title>
        <Text size="sm" fw={500}>
          <ServiceLabel name={node.name} logoUrl={node.logoUrl} />
        </Text>
      </Stack>
      {programs.length === 0 ? (
        <EmptyState icon={History} title={t('timeshift.empty')} />
      ) : (
        programs.map((program) => (
          <Paper key={program.startAt} withBorder p="sm">
            <Group justify="space-between" wrap="nowrap" gap="sm">
              <Stack gap={2} style={{ minWidth: 0 }}>
                <Text size="sm" c="dimmed">
                  {t('range', {
                    from: startFormat.format(Date.parse(program.startAt)),
                    to:
                      program.endAt == null
                        ? t('timeshift.onAir')
                        : endFormat.format(Date.parse(program.endAt)),
                  })}
                </Text>
                <Group gap="xs" wrap="wrap">
                  <Text fw={500}>
                    {program.name === '' ? (
                      t('timeshift.untitled')
                    ) : (
                      <ProgramTitle name={program.name} />
                    )}
                  </Text>
                  {/* 番組の頭のチャンクがないときは、残っているところから再生する。 */}
                  {program.playableFrom !== program.startAt && (
                    <Badge size="sm" variant="light" color="gray">
                      {t('timeshift.partial')}
                    </Badge>
                  )}
                </Group>
              </Stack>
              <Button
                size="compact-sm"
                variant="light"
                leftSection={<Play size={14} aria-hidden />}
                renderRoot={(props) => (
                  <Link
                    {...props}
                    to="/timeshift/$serviceId/play"
                    params={{ serviceId }}
                    search={{ program: program.startAt }}
                  />
                )}
              >
                {t('timeshift.play')}
              </Button>
            </Group>
          </Paper>
        ))
      )}
    </Stack>
  );
}
