import { Alert, Anchor, Group, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { CalendarX2 } from 'lucide-react';
import { Suspense, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import type { ChangeChannel } from '../../changes/dataChanges';
import { EmptyState, ErrorBoundary, PageLoader, zonedFormat } from '../../futon';
import { channelWeekColumns, channelWeekDays, type ChannelDay } from '../../guide/channelWeek';
import { GuideGrid } from '../../guide/GuideGrid';
import { loadExtendedHours } from '../../guide/hourNotation';
import { broadcastDay, dateOf } from '../../guide/layout';
import { ProgramDetail } from '../../guide/ProgramDetail';
import { ServiceLabel } from '../../guide/ServiceLabel';
import { isUnauthenticated } from '../../relay/environment';
import type { channelServiceIdGuideQuery } from './__generated__/channelServiceIdGuideQuery.graphql';

type ChannelSearch = {
  // program は、詳細を開いている番組の ID。
  program?: string;
};

export const Route = createFileRoute('/guide/channel/$serviceId')({
  validateSearch: (search: Record<string, unknown>): ChannelSearch =>
    typeof search['program'] === 'string' && search['program'] !== ''
      ? { program: search['program'] }
      : {},
  component: ChannelGuidePage,
  staticData: { fluid: true },
});

// channelGuideChannels は、変わったら番組表を取り直すデータの種類。番組表の画面と同じにする。
const channelGuideChannels: readonly ChangeChannel[] = ['EPG', 'RESERVATIONS', 'VIEWER_SETTINGS'];

const dayFormat = zonedFormat({ month: 'numeric', day: 'numeric', weekday: 'short' });

// ChannelGuidePage は、1 つのチャンネルの 8 日分を、日付を列にして並べる番組表 (#482、設計書 10.3)。
function ChannelGuidePage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/guide" size="sm">
        {t('guide.channel.back')}
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
          <ChannelGuide />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function ChannelGuide() {
  const { t } = useTranslation();
  const { serviceId } = Route.useParams();
  const search = Route.useSearch();
  const navigate = useNavigate({ from: '/guide/channel/$serviceId' });
  const [now] = useState(Date.now);
  const today = broadcastDay(now);
  const changes = useDataChanges(channelGuideChannels);
  const days = useMemo(() => {
    let to = today.from;
    for (let i = 0; i < channelWeekDays; i++) {
      to = broadcastDay(to).to;
    }

    return { from: today.from, to };
  }, [today.from]);
  const data = useLazyLoadQuery<channelServiceIdGuideQuery>(
    graphql`
      query channelServiceIdGuideQuery($from: Time!, $to: Time!, $services: [ID!]) {
        guide(from: $from, to: $to, services: $services) {
          service {
            id
            name
            logoUrl
            channelType {
              type
            }
          }
          programs {
            id
            startAt
            endAt
            endUndecided
            name
            genres {
              lv1
              lv2
            }
            reservation {
              id
              enabled
            }
            watchReserved
          }
        }
      }
    `,
    {
      from: new Date(days.from).toISOString(),
      to: new Date(days.to).toISOString(),
      services: [serviceId],
    },
    { fetchKey: changes, fetchPolicy: changes === 0 ? 'store-or-network' : 'store-and-network' },
  );
  const row = data.guide[0];
  const columns = useMemo<ChannelDay[]>(
    () =>
      row === undefined
        ? []
        : channelWeekColumns(
            now,
            row.service.id,
            (from) => dayFormat.format(from),
            row.programs.map((program) => ({
              id: program.id,
              startAt: Date.parse(program.startAt),
              endAt: Date.parse(program.endAt),
              endUndecided: program.endUndecided,
              name: program.name,
              genre: program.genres[0],
              // サブチャンネルでも、1 つのチャンネルだけを出すので、番組名を出す。
              shared: false,
              reserved: program.reservation?.enabled === true,
              watchReserved: program.watchReserved,
            })),
          ),
    [row, now],
  );
  const [extendedHours] = useState(loadExtendedHours);
  if (row === undefined) {
    return <Alert color="amber">{t('guide.channel.notFound')}</Alert>;
  }
  const type = row.service.channelType.type;

  return (
    <>
      <Group gap="xs" align="baseline">
        <Title order={2}>
          <ServiceLabel name={row.service.name} logoUrl={row.service.logoUrl ?? null} />
        </Title>
        <Text c="dimmed" size="sm">
          {t('guide.channel.title')}
        </Text>
      </Group>
      {columns.every((column) => column.programs.length === 0) ? (
        <EmptyState icon={CalendarX2} title={t('guide.empty')} hint={t('guide.emptyHint')} />
      ) : (
        <GuideGrid
          window={today}
          columns={columns}
          // 今日の列だけがいまの時刻を含むので、線は引かない。
          now={0}
          scrollTime={now}
          extendedHours={extendedHours}
          // 日付の見出しを押すと、その日の番組表を開く。
          renderHeader={(column) => {
            const day = columns.find((candidate) => candidate.id === column.id);

            return (
              <Anchor
                size="sm"
                fw={700}
                c="inherit"
                underline="hover"
                renderRoot={(props) => (
                  <Link
                    {...props}
                    to="/guide"
                    search={{ date: dateOf(day?.from ?? today.from), type }}
                  />
                )}
                style={{ display: 'block', lineHeight: '36px', paddingInline: 6 }}
              >
                {column.name}
              </Anchor>
            );
          }}
          onSelect={(program) => void navigate({ search: { program } })}
        />
      )}
      <ProgramDetail programId={search.program} onClose={() => void navigate({ search: {} })} />
    </>
  );
}
