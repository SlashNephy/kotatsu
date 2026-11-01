import { Alert, Anchor, Badge, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { CalendarCheck, ListChecks, ListPlus, RefreshCw, Star } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { destinationNames, type AnimeRecordDestinationName } from '../animeRecords/destinations';
import { useDataChanges } from '../changes/DataChanges';
import { EmptyState, ProgramTitle, zonedFormat } from '../futon';
import { broadcastDay, dateOf } from '../guide/layout';
import { ServiceLabel } from '../guide/ServiceLabel';
import type { AnimeWishlistListCreateRuleMutation } from './__generated__/AnimeWishlistListCreateRuleMutation.graphql';
import type { AnimeWishlistListQuery } from './__generated__/AnimeWishlistListQuery.graphql';
import type { AnimeWishlistListRefreshMutation } from './__generated__/AnimeWishlistListRefreshMutation.graphql';
import type { AnimeWishlistListReserveMutation } from './__generated__/AnimeWishlistListReserveMutation.graphql';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// refreshDelayMs は、取り込み直しを頼んでから、一覧を読み直すまでの時間。取り込みはジョブで行うので、終わるのを待たない。
const refreshDelayMs = 3_000;

// AnimeWishlistList は、ログイン中のユーザーが Annict と AniList で「見たい」「見てる」にした作品の、
// これからの放送の候補 (設計書 3.8、10.3、#928)。候補を出すだけで、ルールは、予約できるユーザーが押したときだけ作る。
// 視聴者は、自分の候補を見られるが、ルールも予約も作れない (設計書 9.7)。
export function AnimeWishlistList() {
  const { t } = useTranslation();
  // 予約やルールが変わったら (作ったルールの照合で予約ができたときを含む)、読み直す。
  const changes = useDataChanges(['RESERVATIONS', 'RULES']);
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<AnimeWishlistListQuery>(
    graphql`
      query AnimeWishlistListQuery {
        viewer {
          role
        }
        animeWishlist {
          enabled
          imports {
            destination
            importedAt
            error
            entryCount
          }
          works {
            tid
            title
            reservedBroadcasts
            covered
            sources {
              destination
              workId
              status
              url
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
  const [createRule, creatingRule] = useMutation<AnimeWishlistListCreateRuleMutation>(graphql`
    mutation AnimeWishlistListCreateRuleMutation($tid: Int!) {
      createAnimeWishlistRule(tid: $tid) {
        id
        name
      }
    }
  `);
  const [reserve, reserving] = useMutation<AnimeWishlistListReserveMutation>(graphql`
    mutation AnimeWishlistListReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [refresh, refreshing] = useMutation<AnimeWishlistListRefreshMutation>(graphql`
    mutation AnimeWishlistListRefreshMutation {
      refreshAnimeWishlist
    }
  `);
  const [failed, setFailed] = useState<'rule' | 'reserve' | 'refresh' | null>(null);
  const [created, setCreated] = useState<{ id: string; name: string } | null>(null);
  const [refreshed, setRefreshed] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  // 取り込み直しを頼んだら、少し待ってから読み直す。
  useEffect(() => {
    if (!refreshed) {
      return undefined;
    }
    const timer = setTimeout(() => setFetchKey((key) => key + 1), refreshDelayMs);

    return () => clearTimeout(timer);
  }, [refreshed]);

  const wishlist = data.animeWishlist;
  const canEdit = data.viewer.role !== 'VIEWER';

  if (!wishlist.enabled) {
    return <EmptyState icon={ListChecks} title={t('reservations.wishlist.disabled')} />;
  }
  if (wishlist.imports.length === 0) {
    return (
      <EmptyState
        icon={ListChecks}
        title={t('reservations.wishlist.notConnected')}
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
        setCreated(response.createAnimeWishlistRule);
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
  const onRefresh = () => {
    setFailed(null);
    setRefreshed(false);
    refresh({
      variables: {},
      onCompleted: (_, errors) => {
        if (errors != null && errors.length > 0) {
          setFailed('refresh');

          return;
        }
        setRefreshed(true);
      },
      onError: () => setFailed('refresh'),
    });
  };
  const pending = creatingRule || reserving;

  return (
    <Stack gap="xs">
      <Text size="xs" c="dimmed">
        {t('reservations.wishlist.description')}
      </Text>
      <Group gap="xs" justify="space-between" wrap="wrap">
        <Stack gap={0}>
          {wishlist.imports.map((imported) => {
            const name =
              destinationNames[imported.destination.toLowerCase() as AnimeRecordDestinationName];

            return (
              <Text
                key={imported.destination}
                size="xs"
                c={imported.error == null ? 'dimmed' : 'red'}
              >
                {imported.error != null
                  ? t('reservations.wishlist.importFailed', {
                      destination: name,
                      reason: imported.error,
                    })
                  : imported.importedAt == null
                    ? t('reservations.wishlist.importPending', { destination: name })
                    : t('reservations.wishlist.imported', {
                        destination: name,
                        count: imported.entryCount,
                        date: dateTimeFormat.format(Date.parse(imported.importedAt)),
                      })}
              </Text>
            );
          })}
        </Stack>
        <Button
          size="xs"
          variant="default"
          leftSection={<RefreshCw size={14} aria-hidden />}
          loading={refreshing}
          onClick={onRefresh}
        >
          {t('reservations.wishlist.refresh')}
        </Button>
      </Group>
      {refreshed && (
        <Alert color="blue" role="status" withCloseButton onClose={() => setRefreshed(false)}>
          {t('reservations.wishlist.refreshStarted')}
        </Alert>
      )}
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
      {wishlist.works.length === 0 ? (
        <EmptyState icon={ListChecks} title={t('reservations.wishlist.empty')} />
      ) : (
        wishlist.works.map((work) => (
          <Card key={work.tid} component="section" withBorder padding="xs" aria-label={work.title}>
            <Stack gap="xs">
              <Group gap="sm" wrap="nowrap" justify="space-between" align="flex-start">
                <Stack gap={4} style={{ minWidth: 0 }}>
                  <Group gap="xs">
                    <Title order={3} size="h5">
                      {work.title}
                    </Title>
                    {/* 予約のある作品は、録る放送が決まっているので、予約済みと示し、ルールを作らせない (#928)。 */}
                    {work.covered && (
                      <Badge
                        size="sm"
                        color="teal"
                        variant="light"
                        tt="none"
                        leftSection={<CalendarCheck size={12} aria-hidden />}
                      >
                        {t('reservations.wishlist.covered')}
                      </Badge>
                    )}
                  </Group>
                  <Group gap={4}>
                    {work.sources.map((source) => (
                      <Badge
                        key={`${source.destination}:${source.workId}`}
                        component="a"
                        href={source.url}
                        target="_blank"
                        rel="noreferrer"
                        variant="light"
                        size="sm"
                        tt="none"
                        style={{ cursor: 'pointer' }}
                      >
                        {t('reservations.wishlist.source', {
                          destination:
                            destinationNames[
                              source.destination.toLowerCase() as AnimeRecordDestinationName
                            ],
                          status:
                            source.status === 'WATCHING'
                              ? t('reservations.wishlist.status.WATCHING')
                              : t('reservations.wishlist.status.WANNA_WATCH'),
                        })}
                      </Badge>
                    ))}
                  </Group>
                  {work.reservedBroadcasts > 0 && (
                    <Text size="xs" c="dimmed">
                      {t('reservations.wishlist.reserved', { count: work.reservedBroadcasts })}
                    </Text>
                  )}
                </Stack>
                {canEdit && !work.covered && (
                  <Button
                    size="xs"
                    flex="none"
                    leftSection={<ListPlus size={14} aria-hidden />}
                    loading={busy === `rule:${work.tid}`}
                    disabled={pending && busy !== `rule:${work.tid}`}
                    onClick={() => onCreateRule(work.tid)}
                  >
                    {t('reservations.wishlist.createRule')}
                  </Button>
                )}
              </Group>
              <Stack gap={4}>
                {work.broadcasts.map(({ preferred, program }) => {
                  const start = Date.parse(program.startAt);

                  return (
                    <Group key={program.id} gap="sm" wrap="nowrap" justify="space-between">
                      <Stack gap={0} style={{ minWidth: 0 }}>
                        <Text size="sm" truncate>
                          <Anchor
                            inherit
                            renderRoot={(props) => (
                              <Link
                                {...props}
                                to="/guide"
                                search={{
                                  date: dateOf(broadcastDay(start).from),
                                  type: program.service.channelType.type,
                                  program: program.id,
                                }}
                              />
                            )}
                          >
                            <ProgramTitle name={program.name} />
                          </Anchor>
                        </Text>
                        <Group gap="xs">
                          <Text size="xs" c="dimmed">
                            {t('range', {
                              from: dateTimeFormat.format(start),
                              to: timeFormat.format(Date.parse(program.endAt)),
                            })}
                          </Text>
                          <Text size="xs">
                            <ServiceLabel
                              name={program.service.name}
                              logoUrl={program.service.logoUrl}
                            />
                          </Text>
                          {preferred && (
                            <Badge
                              size="xs"
                              color="teal"
                              variant="light"
                              tt="none"
                              leftSection={<Star size={10} aria-hidden />}
                              title={t('reservations.wishlist.preferredHelp')}
                            >
                              {t('reservations.wishlist.preferred')}
                            </Badge>
                          )}
                        </Group>
                      </Stack>
                      {canEdit && (
                        <Button
                          size="xs"
                          variant="default"
                          flex="none"
                          loading={busy === program.id}
                          disabled={pending && busy !== program.id}
                          onClick={() => onReserve(program.id)}
                        >
                          {t('reservations.wishlist.reserve')}
                        </Button>
                      )}
                    </Group>
                  );
                })}
              </Stack>
            </Stack>
          </Card>
        ))
      )}
    </Stack>
  );
}
