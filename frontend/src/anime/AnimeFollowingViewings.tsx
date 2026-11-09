import { Anchor, Avatar, Badge, Collapse, Group, Stack, Text, UnstyledButton } from '@mantine/core';
import {
  Check,
  ChevronDown,
  Circle,
  Pause,
  Play,
  Repeat,
  Square,
  type LucideIcon,
} from 'lucide-react';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { destinationNames, type AnimeRecordDestinationName } from '../animeRecords/destinations';
import { useDataChanges } from '../changes/DataChanges';
import { ErrorBoundary } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { AnimeFollowingViewingsQuery } from './__generated__/AnimeFollowingViewingsQuery.graphql';
import type { AnimeFollowingViewingsRefreshMutation } from './__generated__/AnimeFollowingViewingsRefreshMutation.graphql';
import { followingCounts, followingDetail, type FollowingStatus } from './following';

// maxAvatars は、折りたたんだときに並べるアバターの数の上限。残りは数だけを示す。
const maxAvatars = 5;

// statusStyles は、状態ごとのアイコンと色。
const statusStyles: Record<FollowingStatus, { icon: LucideIcon; color: string }> = {
  WATCHED: { icon: Check, color: 'teal' },
  WATCHING: { icon: Play, color: 'blue' },
  REPEATING: { icon: Repeat, color: 'blue' },
  ON_HOLD: { icon: Pause, color: 'yellow' },
  STOP_WATCHING: { icon: Square, color: 'red' },
  WANNA_WATCH: { icon: Circle, color: 'gray' },
};

function StatusIcon({ status }: { status: FollowingStatus }) {
  const { icon: Icon, color } = statusStyles[status];

  return <Icon size={12} color={`var(--mantine-color-${color}-filled)`} aria-hidden />;
}

// AnimeFollowingViewings は、Annict と AniList でフォローしている人の、作品の視聴状況を出す (設計書 3.8、#1372)。
// 詳細の本体を待たせないよう、別に読み込む。読み込めなくても、詳細は出し続ける。
export function AnimeFollowingViewings({ tid }: { tid: number }) {
  return (
    <ErrorBoundary fallback={() => null}>
      <Suspense fallback={null}>
        <Section tid={tid} />
      </Suspense>
    </ErrorBoundary>
  );
}

function Section({ tid }: { tid: number }) {
  const { t } = useTranslation();
  const [opened, setOpened] = useState(false);
  // ジョブが終わったら読み直す。
  const changes = useDataChanges(['ANIME_FOLLOWING']);
  const { animeFollowingViewings: following } = useLazyLoadQuery<AnimeFollowingViewingsQuery>(
    graphql`
      query AnimeFollowingViewingsQuery($tid: Int!) {
        animeFollowingViewings(tid: $tid) {
          enabled
          needsRefresh
          failed
          viewings {
            destination
            username
            name
            avatarUrl
            profileUrl
            status
            progress
            episodesCount
            score
            notes
          }
        }
      }
    `,
    { tid },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const [refresh] = useMutation<AnimeFollowingViewingsRefreshMutation>(graphql`
    mutation AnimeFollowingViewingsRefreshMutation($tid: Int!) {
      refreshAnimeFollowingViewings(tid: $tid)
    }
  `);

  // 問い合わせる時期が来ていたら、作品ごとに 1 度だけ取得を頼む。失敗しても、詳細の操作は妨げない。
  const requested = useRef<number | null>(null);
  useEffect(() => {
    if (following.needsRefresh && requested.current !== tid) {
      requested.current = tid;
      refresh({ variables: { tid }, onError: () => {} });
    }
  }, [following.needsRefresh, refresh, tid]);

  if (!following.enabled) {
    return null;
  }
  const viewings = following.viewings;
  if (viewings.length === 0) {
    if (following.failed) {
      return (
        <Text size="xs" c="dimmed">
          {t('anime.following.failed')}
        </Text>
      );
    }
    if (following.needsRefresh) {
      return (
        <Text size="xs" c="dimmed">
          {t('anime.following.loading')}
        </Text>
      );
    }

    // フォローしている人が誰も状態を付けていなければ、詳細を静かに保つ。
    return null;
  }

  return (
    <Stack gap={6} data-testid="anime-following">
      <UnstyledButton onClick={() => setOpened((value) => !value)} aria-expanded={opened}>
        <Stack gap={4}>
          <Group gap={4} wrap="nowrap">
            <Text size="sm" fw={600}>
              {t('anime.following.title')}
            </Text>
            <ChevronDown
              size={14}
              aria-hidden
              style={{ transform: opened ? 'rotate(180deg)' : undefined }}
            />
          </Group>
          <Group gap="sm" wrap="wrap">
            <Avatar.Group spacing="xs">
              {viewings.slice(0, maxAvatars).map((viewing) => (
                <Avatar
                  key={`${viewing.destination}/${viewing.username}`}
                  src={viewing.avatarUrl}
                  alt=""
                  size={28}
                  radius="xl"
                  name={viewing.name}
                  color="initials"
                />
              ))}
              {viewings.length > maxAvatars && (
                <Avatar size={28} radius="xl">
                  {`+${viewings.length - maxAvatars}`}
                </Avatar>
              )}
            </Avatar.Group>
            {followingCounts(viewings).map(({ status, count }) => (
              <Group key={status} gap={4} wrap="nowrap">
                <StatusIcon status={status} />
                <Text size="xs">
                  {t(`anime.following.status.${status}`)} {count}
                </Text>
              </Group>
            ))}
          </Group>
        </Stack>
      </UnstyledButton>
      <Collapse expanded={opened}>
        <Stack gap="xs">
          {viewings.map((viewing) => {
            const detail = followingDetail(t, viewing);
            const status = viewing.status as FollowingStatus;

            return (
              <Group
                key={`${viewing.destination}/${viewing.username}`}
                gap="xs"
                wrap="nowrap"
                align="flex-start"
              >
                <Avatar
                  src={viewing.avatarUrl}
                  alt=""
                  size={32}
                  radius="xl"
                  name={viewing.name}
                  color="initials"
                />
                <Stack gap={0} style={{ minWidth: 0 }}>
                  <Group gap={6} wrap="wrap">
                    <Anchor
                      size="sm"
                      href={viewing.profileUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {viewing.name}
                    </Anchor>
                    <Badge size="xs" variant="light" color="gray">
                      {
                        destinationNames[
                          viewing.destination.toLowerCase() as AnimeRecordDestinationName
                        ]
                      }
                    </Badge>
                  </Group>
                  <Group gap={4} wrap="wrap">
                    <StatusIcon status={status} />
                    <Text size="xs">{t(`anime.following.status.${status}`)}</Text>
                    {detail !== '' && (
                      <Text size="xs" c="dimmed">
                        {detail}
                      </Text>
                    )}
                  </Group>
                  {viewing.notes != null && (
                    <Text size="xs" c="dimmed" style={{ whiteSpace: 'pre-wrap' }}>
                      {viewing.notes}
                    </Text>
                  )}
                </Stack>
              </Group>
            );
          })}
        </Stack>
      </Collapse>
    </Stack>
  );
}
