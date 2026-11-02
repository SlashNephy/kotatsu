import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Image,
  Menu,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { CalendarCheck, ListChecks, ListPlus, Play, Search } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { graphql, useMutation } from 'react-relay';

import { destinationNames, type AnimeRecordDestinationName } from '../animeRecords/destinations';
import { useDataChanges } from '../changes/DataChanges';
import { EmptyState, ProgramTitle, useNow, zonedFormat } from '../futon';
import { broadcastDay, dateOf } from '../guide/layout';
import { ServiceLabel } from '../guide/ServiceLabel';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { RecordAnimeButton } from './RecordAnimeButton';
import type {
  AnimeLibraryQuery,
  AnimeLibraryQuery$data,
} from './__generated__/AnimeLibraryQuery.graphql';
import type { AnimeLibraryCreateRuleMutation } from './__generated__/AnimeLibraryCreateRuleMutation.graphql';
import type { AnimeLibraryReserveMutation } from './__generated__/AnimeLibraryReserveMutation.graphql';
import { nextBroadcastOf, periodOf, sortWorks, type Period } from './library';
import { matchesFilters, type LibraryFilters } from './status';
import { searchURL, type StreamingService } from './streaming';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// maxRecordedEpisodes は、カードに並べる、録画のある話の数の上限。残りは数だけを示す。
const maxRecordedEpisodes = 6;

type Work = AnimeLibraryQuery$data['animeLibrary']['works'][number];
type RecordedEpisode = Work['recordedEpisodes'][number];
type WorkBroadcast = Work['broadcasts'][number];

// periodColors は、放送の時期のバッジの色。「それより後」と「未定」は、バッジを出さない。
const periodColors: Partial<Record<Period, string>> = {
  today: 'red',
  yesterday: 'orange',
  tomorrow: 'blue',
  ended: 'gray',
};

// AnimeLibrary は、ログイン中のユーザーが Annict と AniList で「見たい」「見てる」にした作品を、作品ごとに、次の話、次の放送、
// まだ見ていない録画と並べる (視聴管理の画面。設計書 3.8、10.3、#1231)。ルールを作るのと予約は、予約の画面の「見たいリスト」タブと
// 同じ処理を使い、視聴者には出さない (設計書 9.7)。
export function AnimeLibrary({
  filters,
  streaming,
}: {
  filters: LibraryFilters;
  streaming: readonly StreamingService[];
}) {
  const { t } = useTranslation();
  // 予約やルール、録画、番組表が変わったら、読み直す。
  const changes = useDataChanges(['RESERVATIONS', 'RULES', 'RECORDINGS', 'EPG']);
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<AnimeLibraryQuery>(
    graphql`
      query AnimeLibraryQuery {
        viewer {
          role
        }
        animeLibrary {
          enabled
          imports {
            destination
            importedAt
            error
            entryCount
          }
          works {
            id
            tid
            title
            imageUrl
            status
            progress
            episodesCount
            seasonYear
            season
            sources {
              destination
              workId
              status
              url
            }
            nextEpisode {
              number
              numberText
              title
            }
            broadcasts {
              countStart
              countEnd
              rerun
              last
              program {
                id
                name
                startAt
                endAt
                reservation {
                  enabled
                }
                service {
                  name
                  logoUrl
                  channelType {
                    type
                  }
                }
              }
            }
            recentBroadcasts {
              startAt
              rerun
              last
            }
            recordedEpisodes {
              countStart
              countEnd
              subtitle
              recordingCount
              watched
              recording {
                id
                thumbnailUrl
              }
            }
          }
        }
        # ルールを作れる作品 (これから 1 週間に、予約していない放送がある作品) と、予約済みかどうか (#928)。
        animeWishlist {
          works {
            tid
            covered
          }
        }
      }
    `,
    {},
    { fetchKey: `${changes}:${fetchKey}`, fetchPolicy: 'store-and-network' },
  );
  const [createRule, creatingRule] = useMutation<AnimeLibraryCreateRuleMutation>(graphql`
    mutation AnimeLibraryCreateRuleMutation($tid: Int!) {
      createAnimeWishlistRule(tid: $tid) {
        id
        name
      }
    }
  `);
  const [reserve, reserving] = useMutation<AnimeLibraryReserveMutation>(graphql`
    mutation AnimeLibraryReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [failed, setFailed] = useState<'rule' | 'reserve' | null>(null);
  const [created, setCreated] = useState<{ id: string; name: string } | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const now = useNow(60_000);

  const library = data.animeLibrary;
  const canEdit = data.viewer.role !== 'VIEWER';
  if (!library.enabled) {
    return <EmptyState icon={ListChecks} title={t('anime.disabled')} />;
  }
  if (library.imports.length === 0) {
    return (
      <EmptyState
        icon={ListChecks}
        title={t('anime.notConnected')}
        hint={
          <Anchor component={Link} to="/settings/account" size="sm">
            {t('anime.connect')}
          </Anchor>
        }
      />
    );
  }
  // 「見たいリスト」タブの候補にある作品だけ、ルールを作れる (候補にない作品は、サーバーが断る)。
  const candidates = new Map(data.animeWishlist.works.map((work) => [work.tid, work.covered]));
  const works = sortWorks(
    library.works.filter((work) => matchesFilters(work, filters, now)),
    filters.excludeReruns,
  );

  const run = (
    key: string,
    kind: 'rule' | 'reserve',
    start: (done: (ok: boolean) => void) => void,
  ) => {
    setFailed(null);
    setCreated(null);
    setBusy(key);
    start((ok) => {
      setBusy(null);
      if (!ok) {
        setFailed(kind);

        return;
      }
      setFetchKey((value) => value + 1);
    });
  };
  const onCreateRule = (tid: number) =>
    run(`rule:${tid}`, 'rule', (done) =>
      createRule({
        variables: { tid },
        onCompleted: (response, errors) => {
          const ok = errors == null || errors.length === 0;
          if (ok) {
            setCreated(response.createAnimeWishlistRule);
          }
          done(ok);
        },
        onError: () => done(false),
      }),
    );
  const onReserve = (programId: string) =>
    run(programId, 'reserve', (done) =>
      reserve({
        variables: { program: programId },
        onCompleted: (_, errors) => done(errors == null || errors.length === 0),
        onError: () => done(false),
      }),
    );
  const pending = creatingRule || reserving;

  return (
    <Stack gap="sm">
      <Stack gap={0}>
        {library.imports.map((imported) => {
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
      {works.length === 0 ? (
        <EmptyState icon={ListChecks} title={t('anime.empty')} />
      ) : (
        <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }} spacing="sm">
          {works.map((work) => {
            const tid = work.tid;
            const covered = tid == null ? undefined : candidates.get(tid);

            return (
              <WorkCard
                key={work.id}
                work={work}
                now={now}
                excludeReruns={filters.excludeReruns}
                streaming={streaming}
                canEdit={canEdit}
                pending={pending}
                busy={busy}
                // 候補にない作品 (これから 1 週間に予約していない放送がない作品) は、ルールを作れない。
                ruleTid={canEdit && tid != null && covered === false ? tid : null}
                covered={covered === true}
                onCreateRule={onCreateRule}
                onReserve={onReserve}
              />
            );
          })}
        </SimpleGrid>
      )}
    </Stack>
  );
}

function WorkCard({
  work,
  now,
  excludeReruns,
  streaming,
  canEdit,
  pending,
  busy,
  ruleTid,
  covered,
  onCreateRule,
  onReserve,
}: {
  work: Work;
  now: number;
  excludeReruns: boolean;
  streaming: readonly StreamingService[];
  canEdit: boolean;
  pending: boolean;
  busy: string | null;
  ruleTid: number | null;
  covered: boolean;
  onCreateRule: (tid: number) => void;
  onReserve: (programId: string) => void;
}) {
  const { t } = useTranslation();
  const next = work.nextEpisode;
  // 次の放送は、再放送を除く絞り込みのときは、本放送から選ぶ。
  const broadcast = nextBroadcastOf(work, excludeReruns);
  const period = periodOf(work, now, excludeReruns);
  const periodColor = periodColors[period];
  // 見終えた話数が分かれば、まだ見ていない話の録画だけを並べる。分からなければ、見たかを示さずにすべて並べる。
  const recorded = work.recordedEpisodes.filter((episode) => episode.watched !== true);
  const shown = recorded.slice(0, maxRecordedEpisodes);

  // 作品の画像がなければ (読めなければ)、その作品の最も新しい録画の代表画像を出す (#1235)。
  const thumbnail = work.recordedEpisodes.findLast(
    (episode) => episode.recording.thumbnailUrl != null,
  )?.recording.thumbnailUrl;
  // 作品の画像も代わりの代表画像も読めなければ、画像の枠を出さない。
  const [imageFailed, setImageFailed] = useState(false);
  const image = imageFailed ? undefined : (work.imageUrl ?? thumbnail);

  return (
    <Card component="section" withBorder padding="sm" aria-label={work.title}>
      {image != null && (
        <Card.Section mb="xs">
          <Image
            src={image}
            fallbackSrc={thumbnail ?? undefined}
            h={140}
            fit="cover"
            alt=""
            loading="lazy"
            onError={thumbnail == null ? () => setImageFailed(true) : undefined}
          />
        </Card.Section>
      )}
      <Stack gap="xs">
        <Stack gap={4}>
          <Group gap="xs" wrap="nowrap" justify="space-between" align="flex-start">
            <Group gap={4} style={{ minWidth: 0 }}>
              <Title order={3} size="h5">
                {work.title}
              </Title>
              {periodColor !== undefined && (
                <Badge size="sm" color={periodColor} variant="light" tt="none">
                  {t(`anime.period.${period}`)}
                </Badge>
              )}
              {broadcast?.rerun === true && (
                <Badge size="sm" color="gray" variant="light" tt="none">
                  {t('anime.rerun')}
                </Badge>
              )}
            </Group>
            {covered && (
              <Badge
                size="sm"
                color="teal"
                variant="light"
                tt="none"
                flex="none"
                leftSection={<CalendarCheck size={12} aria-hidden />}
              >
                {t('reservations.wishlist.covered')}
              </Badge>
            )}
          </Group>
          <Group gap={4}>
            {streaming.length > 0 && <StreamingMenu title={work.title} services={streaming} />}
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
          <Text size="xs" c="dimmed">
            {[
              work.seasonYear != null &&
                (work.season != null
                  ? t('anime.season', {
                      year: work.seasonYear,
                      season: seasonLabel(t, work.season),
                    })
                  : t('anime.year', { year: work.seasonYear })),
              // まだ見ていない作品は、見終えた話数の代わりに全話数を出す。
              work.progress != null && work.progress > 0
                ? work.episodesCount != null
                  ? t('anime.progressOf', { progress: work.progress, total: work.episodesCount })
                  : t('anime.progress', { progress: work.progress })
                : work.episodesCount != null && t('anime.total', { total: work.episodesCount }),
            ]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        </Stack>
        <Text size="sm">
          {next == null
            ? t('anime.noNext')
            : t('anime.next', {
                episode:
                  next.numberText ??
                  (next.number != null ? t('anime.episode', { number: next.number }) : ''),
                title: next.title != null ? `「${next.title}」` : '',
              })}
        </Text>
        {broadcast == null ? (
          <Text size="xs" c="dimmed">
            {t('anime.noBroadcast')}
          </Text>
        ) : (
          <Broadcast
            broadcast={broadcast}
            now={now}
            canEdit={canEdit}
            pending={pending}
            busy={busy}
            onReserve={onReserve}
          />
        )}
        {work.tid != null && next?.number != null && (
          <Group gap="xs">
            {/* 次の話が進んだら、記録を頼んだ表示を消して、次の話のボタンにする。 */}
            <RecordAnimeButton
              key={next.number}
              input={{ tid: work.tid, episode: next.number, title: work.title }}
              label={t('anime.record.next', {
                episode: t('anime.episode', { number: next.number }),
              })}
            />
          </Group>
        )}
        {recorded.length > 0 && (
          <Stack gap={4}>
            <Text size="xs" c="dimmed">
              {recorded.some((episode) => episode.watched === false)
                ? t('anime.unwatched')
                : t('anime.recorded')}
            </Text>
            <Group gap={4}>
              {shown.map((episode) => (
                <RecordedEpisodeButton
                  key={`${episode.countStart}-${episode.countEnd}`}
                  episode={episode}
                />
              ))}
              {recorded.length > shown.length && (
                <Text size="xs" c="dimmed">
                  {t('anime.moreRecorded', { count: recorded.length - shown.length })}
                </Text>
              )}
            </Group>
          </Stack>
        )}
        {ruleTid != null && (
          <Group justify="flex-end">
            <Button
              size="xs"
              variant="default"
              leftSection={<ListPlus size={14} aria-hidden />}
              loading={busy === `rule:${ruleTid}`}
              disabled={pending && busy !== `rule:${ruleTid}`}
              onClick={() => onCreateRule(ruleTid)}
            >
              {t('reservations.wishlist.createRule')}
            </Button>
          </Group>
        )}
      </Stack>
    </Card>
  );
}

function Broadcast({
  broadcast,
  now,
  canEdit,
  pending,
  busy,
  onReserve,
}: {
  broadcast: WorkBroadcast;
  now: number;
  canEdit: boolean;
  pending: boolean;
  busy: string | null;
  onReserve: (programId: string) => void;
}) {
  const { t } = useTranslation();
  const { program } = broadcast;
  const start = Date.parse(program.startAt);
  // 無効にした予約も予約として扱う。「予約する」は、無効にした予約を有効にしないので、出さない。
  const reservation = program.reservation;

  return (
    <Group gap="sm" wrap="nowrap" justify="space-between">
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
            {dateTimeFormat.format(start)}
          </Text>
          <Text size="xs">
            <ServiceLabel name={program.service.name} logoUrl={program.service.logoUrl} />
          </Text>
          <Text size="xs" c={start <= now ? 'red' : 'dimmed'}>
            {relativeTime(t, start, now)}
          </Text>
          {reservation != null && (
            <Badge
              size="xs"
              color={reservation.enabled ? 'teal' : 'gray'}
              variant="light"
              tt="none"
            >
              {reservation.enabled ? t('anime.reserved') : t('anime.reservationDisabled')}
            </Badge>
          )}
        </Group>
      </Stack>
      {canEdit && reservation == null && (
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
}

function RecordedEpisodeButton({ episode }: { episode: RecordedEpisode }) {
  const { t } = useTranslation();
  const label =
    episode.countStart === episode.countEnd
      ? t('anime.episode', { number: episode.countStart })
      : t('anime.episodeRange', { from: episode.countStart, to: episode.countEnd });

  return (
    <Button
      size="compact-xs"
      variant={episode.watched === false ? 'light' : 'default'}
      leftSection={<Play size={12} aria-hidden />}
      title={episode.subtitle ?? undefined}
      renderRoot={(props) => (
        <Link
          {...props}
          to="/recordings/$recordingId/play"
          params={{ recordingId: episode.recording.id }}
        />
      )}
    >
      {episode.recordingCount > 1
        ? t('anime.recordingCount', { label, count: episode.recordingCount })
        : label}
    </Button>
  );
}

// seasonLabel は、放送のシーズンの名前を返す。
export function seasonLabel(t: TFunction, season: NonNullable<Work['season']>): string {
  switch (season) {
    case 'WINTER':
      return t('anime.seasons.WINTER');
    case 'SPRING':
      return t('anime.seasons.SPRING');
    case 'SUMMER':
      return t('anime.seasons.SUMMER');
    default:
      return t('anime.seasons.AUTUMN');
  }
}

// relativeTime は、放送が始まるまでの時間を、分、時間、日で返す。放送中なら、放送中と返す。
function relativeTime(t: TFunction, start: number, now: number): string {
  const minutes = Math.ceil((start - now) / 60_000);
  if (minutes <= 0) {
    return t('anime.onAir');
  }
  if (minutes < 60) {
    return t('anime.inMinutes', { count: minutes });
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return t('anime.inHours', { count: hours });
  }

  return t('anime.inDays', { count: Math.floor(hours / 24) });
}

// StreamingMenu は、配信サービスで作品名を探すリンクを並べたメニュー (#1235)。どのサービスを出すかは、端末ごとに選ぶ。
function StreamingMenu({
  title,
  services,
}: {
  title: string;
  services: readonly StreamingService[];
}) {
  const { t } = useTranslation();

  return (
    <Menu position="bottom-start" withinPortal>
      <Menu.Target>
        <Badge
          component="button"
          type="button"
          variant="default"
          size="sm"
          tt="none"
          leftSection={<Search size={10} aria-hidden />}
          style={{ cursor: 'pointer' }}
        >
          {t('anime.streaming.search')}
        </Badge>
      </Menu.Target>
      <Menu.Dropdown>
        {services.map((service) => (
          <Menu.Item
            key={service}
            component="a"
            href={searchURL(service, title)}
            target="_blank"
            rel="noreferrer"
          >
            {t(`anime.streaming.services.${service}`)}
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  );
}
