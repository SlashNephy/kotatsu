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
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { useDataChanges } from '../changes/DataChanges';
import type { ChangeChannel } from '../changes/dataChanges';
import { RecordingCard } from '../recordings/RecordingCard';
import type { DashboardQuery } from './__generated__/DashboardQuery.graphql';
import classes from './Dashboard.module.css';
import { nowPlaying, pinnedNowPlaying } from './nowPlaying';
import { broadcastDay, dateOf } from '../guide/layout';
import { nextWakeAt, ProgramTitle, toHalfWidth, useWakeAt } from '../futon';
import { NowPlayingCard } from '../guide/NowPlayingCard';
import { PinnableCard } from '../guide/PinnableCard';
import { ServiceLabel } from '../guide/ServiceLabel';
import { zonedFormat } from '../futon';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// maxUpcoming は、ホームに出すこれからの予約の数。
// 予約は時間帯に重なるものが返り、録画中の予約も含むので、除いても足りるよう多めに取る。
const maxUpcoming = 5;
const upcomingFetch = 20;

// recentRecordings は、ホームのために読む最近の録画の数。並べる列の数によって、出す数を CSS で絞る (recentGrid)。
const recentRecordings = 6;

export type DashboardRange = { now: number; nowEnd: string; from: string; to: string };

// homeChannels は、ホームに関わるデータの種類。変わったら、読み直す (#866)。
const homeChannels: readonly ChangeChannel[] = [
  'EPG',
  'RESERVATIONS',
  'ASSIGNMENTS',
  'RESERVATION_CONFLICTS',
  'TUNERS',
  'RECORDINGS',
];

// maxWaitMs は、時刻で読み直すまでに待つ最大の時間。放送中の番組がないサービスで、番組が始まるのに気づくのに使う。
const maxWaitMs = 10 * 60_000;

// pinnedTab は、ピン留めしたサービスを出すタブの値。チャンネル種別とぶつからないよう、小文字にする (ライブの一覧と同じ)。
const pinnedTab = 'pinned';

// Dashboard は、ホームの中身。縮退と予約の競合の警告、これからの予約、最近の録画、放送中の番組を出す (設計書 10.3)。
// range は、待たない親で決めた時刻。待つ間に決め直すと、読み込みが終わらなくなる。
// onExpire は、放送中の番組が終わったり、予約が始まったりして、range を決め直す時刻になったら呼ぶ。
export function Dashboard({ range, onExpire }: { range: DashboardRange; onExpire: () => void }) {
  const { t } = useTranslation();
  // 予約やチューナーの状態などが、ほかの端末や neko で変わったら読み直す (#866)。
  const changes = useDataChanges(homeChannels);
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
        pinnedServices {
          id
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
          gaps {
            from
            to
          }
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
              gaps {
                from
                to
              }
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
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
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
  // pinned は、ピン留めしたサービスの ID を、ピン留めした順に並べたもの (#485)。
  // ピン留めしたサービスに放送中の番組があれば、種類の前に「ピン留め」のタブを置き、そこから出す。
  const pinned = data.pinnedServices.map((service) => service.id);
  const pinnedSet = new Set(pinned);
  const pinnedPlaying = pinnedNowPlaying(data.guide, pinned, range.now);
  const tabs = [
    ...(pinnedPlaying.length > 0
      ? [{ type: pinnedTab, displayName: t('pin.tab'), playing: pinnedPlaying }]
      : []),
    ...playingByType,
  ];
  const [selectedType, setSelectedType] = useState<string | null>(() =>
    pinnedPlaying.length > 0 ? pinnedTab : null,
  );
  // ピン留めのタブがなくなったら、最初の種類に戻す。後でまたピン留めのタブができても、ピン留めのタブへは戻さない。
  const fallBack = selectedType === pinnedTab && pinnedPlaying.length === 0;
  useEffect(() => {
    if (fallBack) {
      setSelectedType(null);
    }
  }, [fallBack]);
  // 放送中の番組が終わる時刻か、これからの予約が始まる時刻のうち、最も早いものに読み直す。
  useWakeAt(
    nextWakeAt(
      [
        ...data.guide.flatMap((row) => row.programs.map((program) => program.endAt)),
        ...upcoming.map(({ node }) => node.startAt),
      ],
      range.now,
      maxWaitMs,
    ),
    onExpire,
  );
  const current = tabs.find((tab) => tab.type === selectedType) ?? playingByType[0];
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
          {tabs.length > 1 && (
            <SegmentedControl
              className={classes.types}
              size="xs"
              value={current?.type ?? ''}
              onChange={setSelectedType}
              data={tabs.map((tab) => ({ value: tab.type, label: tab.displayName }))}
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
          <SimpleGrid cols={{ base: 1, xs: 2, md: 3, lg: 4 }} spacing="xs">
            {playing.map((item) => (
              <PinnableCard
                key={item.serviceId}
                serviceId={item.serviceId}
                serviceName={item.serviceName}
                pinned={pinnedSet.has(item.serviceId)}
              >
                <NowPlayingCard
                  serviceId={item.serviceId}
                  serviceName={item.serviceName}
                  serviceLogoUrl={item.serviceLogoUrl}
                  program={{ name: item.programName, startAt: item.startAt, endAt: item.endAt }}
                />
              </PinnableCard>
            ))}
          </SimpleGrid>
        )}
      </Stack>
    </Stack>
  );
}
