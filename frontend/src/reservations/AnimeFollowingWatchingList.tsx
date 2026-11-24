import { Alert, Anchor, Avatar, Group, Stack, Text, Tooltip } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { Users } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { destinationNames, type AnimeRecordDestinationName } from '../animeRecords/destinations';
import { useDataChanges } from '../changes/DataChanges';
import { EmptyState } from '../futon';
import type { AnimeFollowingWatchingListCreateRuleMutation } from './__generated__/AnimeFollowingWatchingListCreateRuleMutation.graphql';
import type { AnimeFollowingWatchingListQuery } from './__generated__/AnimeFollowingWatchingListQuery.graphql';
import type { AnimeFollowingWatchingListReserveMutation } from './__generated__/AnimeFollowingWatchingListReserveMutation.graphql';
import { AnimeCandidateCard } from './AnimeCandidateCard';

// maxAvatars は、作品ごとに並べるアバターの数。これより多い人は、数だけを出す。
const maxAvatars = 8;

// AnimeFollowingWatchingList は、ログイン中のユーザーが Annict と AniList でフォローしている人が「見てる」にした作品の、
// これからの放送 (設計書 3.8、#1968)。見ている人の多い順に並べる。見たいリストの候補 (AnimeWishlistList) と同じく、
// ルールは、予約できるユーザーが押したときだけ作る。視聴者は、見られるが、ルールも予約も作れない (設計書 9.7)。
export function AnimeFollowingWatchingList() {
  const { t } = useTranslation();
  // 予約やルールが変わったら (作ったルールの照合で予約ができたときを含む)、読み直す。
  const changes = useDataChanges(['RESERVATIONS', 'RULES']);
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<AnimeFollowingWatchingListQuery>(
    graphql`
      query AnimeFollowingWatchingListQuery {
        viewer {
          role
        }
        animeRecordSettings {
          destinations {
            connection {
              connectedAt
            }
          }
        }
        animeFollowingWatching {
          enabled
          works {
            tid
            title
            reservedBroadcasts
            covered
            watchers {
              destination
              username
              name
              avatarUrl
              profileUrl
            }
            broadcasts {
              preferred
              program {
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
              }
            }
          }
        }
      }
    `,
    {},
    { fetchKey: `${changes}:${fetchKey}`, fetchPolicy: 'store-and-network' },
  );
  const [createRule, creatingRule] = useMutation<AnimeFollowingWatchingListCreateRuleMutation>(
    graphql`
      mutation AnimeFollowingWatchingListCreateRuleMutation($tid: Int!) {
        createAnimeFollowingWatchingRule(tid: $tid) {
          id
          name
        }
      }
    `,
  );
  const [reserve, reserving] = useMutation<AnimeFollowingWatchingListReserveMutation>(graphql`
    mutation AnimeFollowingWatchingListReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [failed, setFailed] = useState<'rule' | 'reserve' | null>(null);
  const [created, setCreated] = useState<{ id: string; name: string } | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const following = data.animeFollowingWatching;
  const canEdit = data.viewer.role !== 'VIEWER';

  if (!following.enabled) {
    return <EmptyState icon={Users} title={t('reservations.wishlist.disabled')} />;
  }
  if (
    !data.animeRecordSettings.destinations.some((destination) => destination.connection != null)
  ) {
    return (
      <EmptyState
        icon={Users}
        title={t('reservations.following.notConnected')}
        hint={
          <Anchor component={Link} to="/settings/account" size="sm">
            {t('reservations.wishlist.connect')}
          </Anchor>
        }
      />
    );
  }

  const onCreateRule = (tid: number) => {
    setFailed(null);
    setCreated(null);
    setBusy(`rule:${tid}`);
    createRule({
      variables: { tid },
      onCompleted: (response, errors) => {
        setBusy(null);
        if (errors != null && errors.length > 0) {
          setFailed('rule');

          return;
        }
        setCreated(response.createAnimeFollowingWatchingRule);
        // ルールの照合で予約ができた放送は、一覧から外れるので読み直す。
        setFetchKey((key) => key + 1);
      },
      onError: () => {
        setBusy(null);
        setFailed('rule');
      },
    });
  };
  const onReserve = (programId: string) => {
    setFailed(null);
    setBusy(programId);
    reserve({
      variables: { program: programId },
      onCompleted: (_, errors) => {
        setBusy(null);
        if (errors != null && errors.length > 0) {
          setFailed('reserve');

          return;
        }
        // 予約した放送は、一覧から外れるので読み直す。
        setFetchKey((key) => key + 1);
      },
      onError: () => {
        setBusy(null);
        setFailed('reserve');
      },
    });
  };
  const pending = creatingRule || reserving;

  return (
    <Stack gap="xs">
      <Text size="xs" c="dimmed">
        {t('reservations.following.description')}
      </Text>
      {!canEdit && (
        <Alert color="gray" role="note">
          {t('reservations.wishlist.viewerNote')}
        </Alert>
      )}
      {created !== null && (
        <Alert color="teal" role="status" withCloseButton onClose={() => setCreated(null)}>
          {t('reservations.wishlist.ruleCreated', { name: created.name })}{' '}
          <Anchor
            size="sm"
            renderRoot={(props) => (
              <Link {...props} to="/rules/$ruleId" params={{ ruleId: created.id }} />
            )}
          >
            {t('reservations.wishlist.openRule')}
          </Anchor>
        </Alert>
      )}
      {failed !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setFailed(null)}>
          {t(`reservations.wishlist.error.${failed}`)}
        </Alert>
      )}
      {following.works.length === 0 ? (
        <EmptyState icon={Users} title={t('reservations.following.empty')} />
      ) : (
        following.works.map((work) => (
          <AnimeCandidateCard
            key={work.tid}
            work={work}
            canEdit={canEdit}
            busy={busy}
            pending={pending}
            onCreateRule={onCreateRule}
            onReserve={onReserve}
          >
            <Group gap="xs" wrap="nowrap">
              <Avatar.Group spacing="xs">
                {work.watchers.slice(0, maxAvatars).map((watcher) => {
                  const destination =
                    destinationNames[
                      watcher.destination.toLowerCase() as AnimeRecordDestinationName
                    ];
                  const label = t('reservations.following.watcher', {
                    name: watcher.name,
                    destination,
                  });

                  return (
                    <Tooltip
                      key={`${watcher.destination}:${watcher.username}`}
                      label={label}
                      withArrow
                    >
                      <Avatar
                        component="a"
                        href={watcher.profileUrl}
                        target="_blank"
                        rel="noreferrer"
                        size="sm"
                        radius="xl"
                        src={watcher.avatarUrl}
                        alt={label}
                        aria-label={label}
                        name={watcher.name}
                      />
                    </Tooltip>
                  );
                })}
              </Avatar.Group>
              <Text size="xs" c="dimmed">
                {t('reservations.following.watching', { count: work.watchers.length })}
              </Text>
            </Group>
          </AnimeCandidateCard>
        ))
      )}
    </Stack>
  );
}
