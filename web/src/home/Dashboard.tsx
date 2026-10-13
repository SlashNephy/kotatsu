import { Alert, Anchor, Badge, Card, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { RecordingThumbnail } from '../recordings/RecordingThumbnail';
import { stateColor, stateKey } from '../recordings/states';
import type { DashboardQuery } from './__generated__/DashboardQuery.graphql';
import { nowPlaying } from './nowPlaying';
import { ProgramMarkBadges, splitProgramMarks } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';

const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});
const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// maxNowPlaying は、ホームに出す放送中の番組の数。続きは番組表で見る。
const maxNowPlaying = 12;

export type DashboardRange = { now: number; nowEnd: string; from: string; to: string };

// Dashboard は、ホームの中身。縮退と予約の競合の警告、放送中の番組、最近の録画、これからの予約を出す (設計書 10.3)。
// range は、待たない親で決めた時刻。待つ間に決め直すと、読み込みが終わらなくなる。
export function Dashboard({ range }: { range: DashboardRange }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<DashboardQuery>(
    graphql`
      query DashboardQuery($from: Time!, $nowEnd: Time!, $to: Time!) {
        tunerServerHealth {
          degraded
          degradedSince
        }
        guide(from: $from, to: $nowEnd) {
          service {
            id
            name
            logoUrl
          }
          programs {
            id
            name
            startAt
            endAt
            sharedFrom {
              id
            }
          }
        }
        recordings(first: 6) {
          edges {
            node {
              id
              name
              state
              recordedFrom
              thumbnailUrl
              service {
                name
                logoUrl
              }
            }
          }
        }
        upcoming: reservations(from: $from, to: $to, enabled: true, first: 5) {
          edges {
            node {
              id
              name
              startAt
              endAt
              service {
                name
                logoUrl
              }
              assignment {
                state
              }
            }
          }
        }
        troubles: reservations(
          from: $from
          to: $to
          states: [CONFLICT, PARTIAL]
          enabled: true
          first: 100
        ) {
          edges {
            node {
              id
            }
          }
          pageInfo {
            hasNextPage
          }
        }
      }
    `,
    { from: range.from, nowEnd: range.nowEnd, to: range.to },
  );
  const playing = nowPlaying(data.guide, range.now, maxNowPlaying);
  // 100 件を超えたら、「100 件以上」と示す。
  const troubles = data.troubles.edges.length;
  const moreTroubles = data.troubles.pageInfo.hasNextPage;

  return (
    <Stack gap="lg">
      {data.tunerServerHealth.degraded && (
        <Alert color="amber" role="alert" title={t('home.degraded.title')}>
          {data.tunerServerHealth.degradedSince == null
            ? t('home.degraded.body')
            : t('home.degraded.since', {
                time: dateTimeFormat.format(Date.parse(data.tunerServerHealth.degradedSince)),
              })}
        </Alert>
      )}
      {troubles > 0 && (
        <Alert color="amber" role="alert" title={t('home.conflicts.title')}>
          <Group gap="xs">
            <Text size="sm">
              {t(moreTroubles ? 'home.conflicts.bodyMore' : 'home.conflicts.body', {
                count: troubles,
              })}
            </Text>
            <Anchor component={Link} to="/reservations" size="sm">
              {t('home.conflicts.link')}
            </Anchor>
          </Group>
        </Alert>
      )}

      <Stack gap="xs">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('home.nowPlaying')}
          </Title>
          <Anchor component={Link} to="/guide" size="sm">
            {t('home.openGuide')}
          </Anchor>
        </Group>
        {playing.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('home.nothingPlaying')}
          </Text>
        ) : (
          <SimpleGrid cols={{ base: 1, xs: 2, md: 3 }} spacing="xs">
            {playing.map((item) => {
              const program = splitProgramMarks(item.programName);

              return (
                <Card
                  key={item.serviceId}
                  withBorder
                  padding="xs"
                  renderRoot={(props) => (
                    <Link {...props} to="/live/$serviceId" params={{ serviceId: item.serviceId }} />
                  )}
                >
                  <Text size="xs" c="dimmed">
                    <ServiceLabel name={item.serviceName} logoUrl={item.serviceLogoUrl} />
                  </Text>
                  <Text size="sm" lineClamp={2} mt="xs">
                    {program.name}
                  </Text>
                  {/* 番組名が 1 行のカードでも、放送時間をカードの下端に揃える。 */}
                  <Group gap={4} mt="auto" pt={4}>
                    <Text size="xs" c="dimmed">
                      {timeFormat.format(Date.parse(item.startAt))} 〜{' '}
                      {timeFormat.format(Date.parse(item.endAt))}{' '}
                      {t('home.duration', {
                        minutes: Math.round(
                          (Date.parse(item.endAt) - Date.parse(item.startAt)) / 60_000,
                        ),
                      })}
                    </Text>
                    <ProgramMarkBadges marks={program.marks} />
                  </Group>
                </Card>
              );
            })}
          </SimpleGrid>
        )}
      </Stack>

      <Stack gap="xs">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('home.recentRecordings')}
          </Title>
          <Anchor component={Link} to="/recordings" size="sm">
            {t('home.allRecordings')}
          </Anchor>
        </Group>
        {data.recordings.edges.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('home.noRecordings')}
          </Text>
        ) : (
          <SimpleGrid cols={{ base: 2, sm: 3, md: 6 }} spacing="xs">
            {data.recordings.edges.map(({ node }) => (
              <Card
                key={node.id}
                withBorder
                padding={0}
                renderRoot={(props) => (
                  <Link
                    {...props}
                    to="/recordings/$recordingId"
                    params={{ recordingId: node.id }}
                  />
                )}
              >
                <RecordingThumbnail url={node.thumbnailUrl} />
                <Stack gap={2} p="xs">
                  <Text size="sm" lineClamp={2}>
                    {node.name}
                  </Text>
                  <Group gap={4}>
                    <Badge size="xs" variant="light" color={stateColor(node.state)}>
                      {t(stateKey(node.state))}
                    </Badge>
                    <Text size="xs" c="dimmed">
                      <ServiceLabel name={node.service.name} logoUrl={node.service.logoUrl} />
                    </Text>
                  </Group>
                </Stack>
              </Card>
            ))}
          </SimpleGrid>
        )}
      </Stack>

      <Stack gap="xs">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('home.upcoming')}
          </Title>
          <Anchor component={Link} to="/reservations" size="sm">
            {t('home.allReservations')}
          </Anchor>
        </Group>
        {data.upcoming.edges.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('home.noReservations')}
          </Text>
        ) : (
          <Stack gap={4}>
            {data.upcoming.edges.map(({ node }) => (
              <Group key={node.id} gap="xs" wrap="nowrap">
                <Text size="sm" c="dimmed" style={{ flexShrink: 0 }}>
                  {dateTimeFormat.format(Date.parse(node.startAt))}
                </Text>
                <Text size="sm" truncate style={{ flex: 1, minWidth: 0 }}>
                  {node.name}
                </Text>
                <Text size="xs" c="dimmed" style={{ flexShrink: 0 }}>
                  <ServiceLabel name={node.service.name} logoUrl={node.service.logoUrl} />
                </Text>
                {(node.assignment.state === 'CONFLICT' || node.assignment.state === 'PARTIAL') && (
                  <Badge size="xs" color="amber" variant="light">
                    {t(
                      node.assignment.state === 'CONFLICT'
                        ? 'reservations.state.CONFLICT'
                        : 'reservations.state.PARTIAL',
                    )}
                  </Badge>
                )}
              </Group>
            ))}
          </Stack>
        )}
      </Stack>
    </Stack>
  );
}
