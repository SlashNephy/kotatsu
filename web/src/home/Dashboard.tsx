import {
  Alert,
  Anchor,
  Badge,
  Box,
  Group,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { RecordingCard } from '../recordings/RecordingCard';
import type { DashboardQuery } from './__generated__/DashboardQuery.graphql';
import classes from './Dashboard.module.css';
import { nowPlaying } from './nowPlaying';
import { broadcastDay, dateOf } from '../guide/layout';
import { ProgramTitle, toHalfWidth } from '../futon';
import { NowPlayingCard } from '../guide/NowPlayingCard';
import { ServiceLabel } from '../guide/ServiceLabel';

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// maxUpcoming は、ホームに出すこれからの予約の数。
// 予約は時間帯に重なるものが返り、録画中の予約も含むので、除いても足りるよう多めに取る。
const maxUpcoming = 5;
const upcomingFetch = 20;

// recentRecordings は、ホームのために読む最近の録画の数。並べる列の数によって、出す数を CSS で絞る (recentGrid)。
const recentRecordings = 6;

export type DashboardRange = { now: number; nowEnd: string; from: string; to: string };

// Dashboard は、ホームの中身。縮退と予約の競合の警告、これからの予約、最近の録画、放送中の番組を出す (設計書 10.3)。
// range は、待たない親で決めた時刻。待つ間に決め直すと、読み込みが終わらなくなる。
export function Dashboard({ range }: { range: DashboardRange }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<DashboardQuery>(
    graphql`
      query DashboardQuery(
        $from: Time!
        $nowEnd: Time!
        $to: Time!
        $recordings: Int!
        $upcoming: Int!
      ) {
        tunerServerHealth {
          degraded
          degradedSince
          clockSkewed
          clockSkewMs
        }
        channelTypes {
          type
          displayName
        }
        guide(from: $from, to: $nowEnd) {
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
            name
            startAt
            endAt
            sharedFrom {
              id
            }
          }
        }
        continueWatching(first: $recordings) {
          id
          name
          startAt
          endAt
          state
          drops
          errors
          scrambled
          recordedFrom
          recordedUntil
          watchPositionMs
          thumbnailUrl
          previewUrl
          watchFrame {
            url
            x
            y
            width
            height
            sheetWidth
            sheetHeight
          }
          service {
            name
            logoUrl
          }
        }
        recordings(first: $recordings) {
          edges {
            node {
              id
              name
              startAt
              endAt
              state
              drops
              errors
              scrambled
              recordedFrom
              recordedUntil
              watchPositionMs
              thumbnailUrl
              previewUrl
              watchFrame {
                url
                x
                y
                width
                height
                sheetWidth
                sheetHeight
              }
              service {
                name
                logoUrl
              }
            }
          }
        }
        upcoming: reservations(from: $from, to: $to, enabled: true, first: $upcoming) {
          edges {
            node {
              id
              name
              startAt
              endAt
              service {
                name
                logoUrl
                channelType {
                  type
                }
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
    {
      from: range.from,
      nowEnd: range.nowEnd,
      to: range.to,
      recordings: recentRecordings,
      upcoming: upcomingFetch,
    },
  );
  // 放送中の番組を、チャンネルの種類ごとに分ける。番組のない種類は、選べないようにする。
  // これからの予約は、まだ始まっていないものだけにする。始まった予約は、録画中として最近の録画に出る。
  const upcoming = data.upcoming.edges
    .filter(({ node }) => Date.parse(node.startAt) > range.now)
    .slice(0, maxUpcoming);
  const playingByType = data.channelTypes
    .map((channelType) => ({
      ...channelType,
      playing: nowPlaying(
        data.guide.filter((row) => row.service.channelType.type === channelType.type),
        range.now,
      ),
    }))
    .filter((channelType) => channelType.playing.length > 0);
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const current =
    playingByType.find((channelType) => channelType.type === selectedType) ?? playingByType[0];
  const playing = current?.playing ?? [];
  // 100 件を超えたら、「100 件以上」と示す。
  const troubles = data.troubles.edges.length;
  const moreTroubles = data.troubles.pageInfo.hasNextPage;

  return (
    <Stack gap="xl">
      {data.tunerServerHealth.degraded && (
        <Alert color="amber" role="alert" title={t('home.degraded.title')}>
          {data.tunerServerHealth.degradedSince == null
            ? t('home.degraded.body')
            : t('home.degraded.since', {
                time: dateTimeFormat.format(Date.parse(data.tunerServerHealth.degradedSince)),
              })}
        </Alert>
      )}
      {/* チューナーサーバーの時計がずれていると、録画の開始と終了がずれる (#505)。縮退中は、時計のずれより縮退を知らせる。 */}
      {!data.tunerServerHealth.degraded &&
        data.tunerServerHealth.clockSkewed &&
        data.tunerServerHealth.clockSkewMs != null && (
          <Alert color="amber" role="alert" title={t('home.clockSkew.title')}>
            {t(
              data.tunerServerHealth.clockSkewMs > 0
                ? 'home.clockSkew.ahead'
                : 'home.clockSkew.behind',
              { seconds: Math.round(Math.abs(data.tunerServerHealth.clockSkewMs) / 1000) },
            )}
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
            {t('home.upcoming')}
          </Title>
          <Anchor component={Link} to="/reservations" size="sm">
            {t('home.allReservations')}
          </Anchor>
        </Group>
        {upcoming.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('home.noReservations')}
          </Text>
        ) : (
          <Box className={classes.upcomingList}>
            {upcoming.map(({ node }) => (
              // 押すと、番組表のその日・その種別を開き、この予約の番組の詳細を開く。
              <Box
                key={node.id}
                className={classes.upcomingRow}
                renderRoot={(props) => (
                  <Link
                    {...props}
                    to="/guide"
                    search={{
                      date: dateOf(broadcastDay(Date.parse(node.startAt)).from),
                      type: node.service.channelType.type,
                      reservation: node.id,
                    }}
                  />
                )}
              >
                <Text size="sm" c="dimmed">
                  {dateTimeFormat.format(Date.parse(node.startAt))}
                </Text>
                <Text size="sm" truncate>
                  <ProgramTitle name={node.name} />
                </Text>
                {/* バッジがない行でも、サービスを右端の列に置くよう、空の枠を残す。 */}
                <span>
                  {(node.assignment.state === 'CONFLICT' ||
                    node.assignment.state === 'PARTIAL') && (
                    <Badge size="xs" color="amber" variant="light">
                      {t(
                        node.assignment.state === 'CONFLICT'
                          ? 'reservations.state.CONFLICT'
                          : 'reservations.state.PARTIAL',
                      )}
                    </Badge>
                  )}
                </span>
                {/* サービスの列は、長い名前でも番組名を潰さないよう、幅に上限を付けて省く。全部の名前は、ホバーで見せる。 */}
                <Text
                  size="xs"
                  c="dimmed"
                  truncate
                  maw="14em"
                  title={toHalfWidth(node.service.name)}
                >
                  <ServiceLabel
                    name={node.service.name}
                    logoUrl={node.service.logoUrl}
                    logoOnlyNarrow
                  />
                </Text>
              </Box>
            ))}
          </Box>
        )}
      </Stack>

      {/* 途中まで見た録画があるときだけ、最近の録画の前に出す。 */}
      {data.continueWatching.length > 0 && (
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('home.continueWatching')}
          </Title>
          <SimpleGrid cols={{ base: 2, sm: 3, md: 4 }} spacing="xs" className={classes.recentGrid}>
            {data.continueWatching.map((recording) => (
              <RecordingCard key={recording.id} recording={recording} />
            ))}
          </SimpleGrid>
        </Stack>
      )}

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
          <SimpleGrid cols={{ base: 2, sm: 3, md: 4 }} spacing="xs" className={classes.recentGrid}>
            {data.recordings.edges.map(({ node }) => (
              <RecordingCard key={node.id} recording={node} />
            ))}
          </SimpleGrid>
        )}
      </Stack>

      <Stack gap="xs">
        <div className={classes.nowPlayingHeader}>
          <Title order={3} size="h5" className={classes.title}>
            {t('home.nowPlaying')}
          </Title>
          {playingByType.length > 1 && (
            <SegmentedControl
              className={classes.types}
              size="xs"
              value={current?.type ?? ''}
              onChange={setSelectedType}
              data={playingByType.map((channelType) => ({
                value: channelType.type,
                label: channelType.displayName,
              }))}
            />
          )}
          <Anchor component={Link} to="/guide" size="sm" className={classes.link}>
            {t('home.openGuide')}
          </Anchor>
        </div>
        {playing.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('home.nothingPlaying')}
          </Text>
        ) : (
          <SimpleGrid cols={{ base: 1, xs: 2, md: 3 }} spacing="xs">
            {playing.map((item) => (
              <NowPlayingCard
                key={item.serviceId}
                serviceId={item.serviceId}
                serviceName={item.serviceName}
                serviceLogoUrl={item.serviceLogoUrl}
                program={{ name: item.programName, startAt: item.startAt, endAt: item.endAt }}
              />
            ))}
          </SimpleGrid>
        )}
      </Stack>
    </Stack>
  );
}
