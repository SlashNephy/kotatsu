import { Alert, Anchor, Badge, Button, Group, Paper, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { CircleDot, History, Play } from 'lucide-react';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
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
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { ServiceIdTimeshiftPromoteMutation } from './__generated__/ServiceIdTimeshiftPromoteMutation.graphql';
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
// 予約を作れるユーザーは、番組を録画に昇格できる。
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
  const { viewer, node } = useLazyLoadQuery<ServiceIdTimeshiftQuery>(
    graphql`
      query ServiceIdTimeshiftQuery($id: ID!) {
        viewer {
          role
        }
        node(id: $id) {
          __typename
          ... on Service {
            name
            logoUrl
            timeshiftPrograms {
              name
              startAt
              endAt
              fromStart
              promoting
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
  // 録画への昇格は、予約を作れるユーザーだけができる。
  const canPromote = viewer.role !== 'VIEWER';

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
                  {/* 番組の頭から録れていないときは、残っているところから再生する。 */}
                  {!program.fromStart && (
                    <Badge size="sm" variant="light" color="gray">
                      {t('timeshift.partial')}
                    </Badge>
                  )}
                </Group>
              </Stack>
              <Group gap="xs" wrap="nowrap">
                {canPromote && (
                  <PromoteButton
                    serviceId={serviceId}
                    startAt={program.startAt}
                    promoting={program.promoting}
                  />
                )}
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
            </Group>
          </Paper>
        ))
      )}
    </Stack>
  );
}

// PromoteButton は、番組を録画に昇格するボタン (設計書 5.7)。放送中の番組は番組予約を作り、放送の終わった番組は
// バッファから録画に書き出す。昇格したら、ボタンの代わりに、予約したか、書き出していることを出す。
function PromoteButton({
  serviceId,
  startAt,
  promoting,
}: {
  serviceId: string;
  startAt: string;
  promoting: boolean;
}) {
  const { t } = useTranslation();
  const [commit, inFlight] = useMutation<ServiceIdTimeshiftPromoteMutation>(graphql`
    mutation ServiceIdTimeshiftPromoteMutation($serviceId: ID!, $startAt: Time!) {
      promoteTimeshiftProgram(serviceId: $serviceId, startAt: $startAt) {
        recording {
          id
        }
        reservation {
          id
        }
      }
    }
  `);
  const [result, setResult] = useState<'promoting' | 'reserved' | 'error' | null>(
    promoting ? 'promoting' : null,
  );
  const shown = promoting && result === null ? 'promoting' : result;
  if (shown === 'promoting') {
    return (
      <Badge
        size="sm"
        variant="light"
        color="red"
        leftSection={<CircleDot size={10} aria-hidden />}
      >
        {t('timeshift.promoting')}
      </Badge>
    );
  }
  if (shown === 'reserved') {
    return (
      <Anchor size="sm" renderRoot={(props) => <Link {...props} to="/reservations" />}>
        {t('timeshift.reserved')}
      </Anchor>
    );
  }
  const promote = () => {
    setResult(null);
    commit({
      variables: { serviceId, startAt },
      onCompleted: (response, errors) => {
        if (errors != null && errors.length > 0) {
          setResult('error');

          return;
        }
        setResult(response.promoteTimeshiftProgram.reservation != null ? 'reserved' : 'promoting');
      },
      // 同じ番組をほかで昇格していれば、書き出している途中として出す。
      onError: (error) =>
        setResult(graphQLErrorDetails(error).code === 'CONFLICT' ? 'promoting' : 'error'),
    });
  };

  return (
    <Stack gap={2} align="flex-end">
      <Button
        size="compact-sm"
        variant="default"
        leftSection={<CircleDot size={14} aria-hidden />}
        loading={inFlight}
        onClick={promote}
      >
        {t('timeshift.promote')}
      </Button>
      {shown === 'error' && (
        <Text size="xs" c="red" role="alert">
          {t('timeshift.promoteError')}
        </Text>
      )}
    </Stack>
  );
}
