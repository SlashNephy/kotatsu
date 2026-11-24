import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { ListChecks, Play } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { Broadcast } from '../anime/AnimeLibrary';
import type { Period } from '../anime/library';
import { useDataChanges } from '../changes/DataChanges';
import { EmptyState, useNow } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type {
  DramaLibraryQuery,
  DramaLibraryQuery$data,
} from './__generated__/DramaLibraryQuery.graphql';
import type { DramaLibraryImportMutation } from './__generated__/DramaLibraryImportMutation.graphql';
import type { DramaLibraryReserveMutation } from './__generated__/DramaLibraryReserveMutation.graphql';
import { episodeLabel } from './DramaEpisode';
import { DramaWatchedButton, DramaWatchStatusMenu, watchStatusOf } from './DramaWatchControls';
import { dramaPeriodOf, matchesDramaFilters, type DramaFilters } from './library';

// maxRecordedEpisodes は、カードに並べる、録画のある話の数の上限。残りは数だけを示す。
const maxRecordedEpisodes = 6;

type Work = DramaLibraryQuery$data['dramaLibrary']['works'][number];
type RecordedEpisode = Work['recordedEpisodes'][number];

// periodColors は、放送の時期のバッジの色。「それより後」と「未定」は、バッジを出さない (アニメの画面と同じ)。
const periodColors: Partial<Record<Period, string>> = {
  today: 'red',
  yesterday: 'orange',
  tomorrow: 'blue',
  ended: 'gray',
};

// DramaLibrary は、ログイン中のユーザーが「見ている」「見たい」にしたドラマの作品を、作品ごとに、次に見る話、次の放送、
// まだ見ていない録画と並べる (ドラマの視聴管理の画面。設計書 3.9、10.3、#1652)。予約はアニメの画面と同じく、視聴者には出さない。
export function DramaLibrary({ filters }: { filters: DramaFilters }) {
  const { t } = useTranslation();
  // 予約や録画、番組表が変わったときと、Simkl のウォッチリストを取り込んで作品が変わったとき (#1649) に、読み直す。
  const changes = useDataChanges(['RESERVATIONS', 'RECORDINGS', 'EPG', 'DRAMA_WATCHLIST']);
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<DramaLibraryQuery>(
    graphql`
      query DramaLibraryQuery {
        viewer {
          role
        }
        dramaLibrary {
          enabled
          works {
            id
            status
            series {
              tmdbId
              name
              originalName
              url
            }
            lastWatched {
              seasonNumber
              episodeNumber
              name
            }
            nextEpisode {
              seasonNumber
              episodeNumber
              name
              url
            }
            nextBroadcast {
              seasonNumber
              episodeNumber
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
            recentBroadcast {
              startAt
              last
            }
            recordedEpisodes {
              seasonNumber
              episodeNumber
              name
              recordingCount
              watched
              recording {
                id
              }
            }
          }
        }
      }
    `,
    {},
    { fetchKey: `${changes}:${fetchKey}`, fetchPolicy: 'store-and-network' },
  );
  const [reserve, reserving] = useMutation<DramaLibraryReserveMutation>(graphql`
    mutation DramaLibraryReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [requestImport] = useMutation<DramaLibraryImportMutation>(graphql`
    mutation DramaLibraryImportMutation {
      importDramaWatchlist
    }
  `);
  // 画面を開いたら、Simkl と連携していれば、ウォッチリストの取り込みを 1 度だけ頼む (#1649)。サーバーは、前に問い合わせてから
  // 30 分たっていなければ取り込まない (Simkl は、タイマーで問い合わせ続けず、利用者の操作で取り込むよう求めている)。
  // 取り込めなくても、画面の操作は妨げない。
  const importRequested = useRef(false);
  const enabled = data.dramaLibrary.enabled;
  useEffect(() => {
    if (enabled && !importRequested.current) {
      importRequested.current = true;
      requestImport({ variables: {}, onError: () => {} });
    }
  }, [enabled, requestImport]);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const now = useNow(60_000);
  const reload = () => setFetchKey((value) => value + 1);

  const library = data.dramaLibrary;
  const canEdit = data.viewer.role !== 'VIEWER';
  if (!library.enabled) {
    return <EmptyState icon={ListChecks} title={t('drama.library.disabled')} />;
  }
  const works = library.works.filter((work) => matchesDramaFilters(work, filters, now));
  const onReserve = (programId: string) => {
    setFailed(false);
    setBusy(programId);
    const done = (ok: boolean) => {
      setBusy(null);
      if (ok) {
        reload();
      } else {
        setFailed(true);
      }
    };
    reserve({
      variables: { program: programId },
      onCompleted: (_, errors) => done(errors == null || errors.length === 0),
      onError: () => done(false),
    });
  };

  return (
    <Stack gap="sm">
      {failed && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setFailed(false)}>
          {t('reservations.wishlist.error.reserve')}
        </Alert>
      )}
      {works.length === 0 ? (
        <EmptyState
          icon={ListChecks}
          title={t('drama.library.empty')}
          hint={library.works.length === 0 ? t('drama.library.emptyHint') : undefined}
        />
      ) : (
        <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }} spacing="sm">
          {works.map((work) => (
            <WorkCard
              key={work.id}
              work={work}
              now={now}
              canEdit={canEdit}
              pending={reserving}
              busy={busy}
              onReserve={onReserve}
              onChange={reload}
            />
          ))}
        </SimpleGrid>
      )}
    </Stack>
  );
}

function WorkCard({
  work,
  now,
  canEdit,
  pending,
  busy,
  onReserve,
  onChange,
}: {
  work: Work;
  now: number;
  canEdit: boolean;
  pending: boolean;
  busy: string | null;
  onReserve: (programId: string) => void;
  onChange: () => void;
}) {
  const { t } = useTranslation();
  const title = work.series.name ?? work.series.originalName ?? t('drama.unknownSeries');
  const period = dramaPeriodOf(work, now);
  const periodColor = periodColors[period];
  const broadcast = work.nextBroadcast;
  const next = work.nextEpisode;
  const last = work.lastWatched;
  // 見た話が分かれば、まだ見ていない話の録画だけを並べる。分からなければ、見たかを示さずにすべて並べる。
  const recorded = work.recordedEpisodes.filter((episode) => episode.watched !== true);
  const shown = recorded.slice(0, maxRecordedEpisodes);

  return (
    <Card component="section" withBorder padding="sm" aria-label={title}>
      <Stack gap="xs">
        <Stack gap={4}>
          <Group gap={4} style={{ minWidth: 0 }}>
            <Title order={3} size="h5">
              {title}
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
          <Group gap={4}>
            <DramaWatchStatusMenu
              seriesId={work.series.tmdbId}
              status={watchStatusOf(work.status)}
              onChange={onChange}
            />
            <Anchor size="xs" href={work.series.url} target="_blank" rel="noopener noreferrer">
              {t('drama.tmdb')}
            </Anchor>
          </Group>
          <Text size="xs" c="dimmed">
            {last == null
              ? t('drama.library.noWatched')
              : t('drama.library.lastWatched', { episode: episodeLabel(t, last) })}
          </Text>
        </Stack>
        <Text size="sm">
          {next != null
            ? t('drama.library.next', {
                episode: episodeLabel(t, next),
                title: next.name != null ? `「${next.name}」` : '',
              })
            : last != null
              ? t('drama.library.finished')
              : t('anime.noNext')}
        </Text>
        {broadcast == null ? (
          <Text size="xs" c="dimmed">
            {t('anime.noBroadcast')}
          </Text>
        ) : (
          <Stack gap={0}>
            {broadcast.seasonNumber != null && broadcast.episodeNumber != null && (
              <Text size="xs" c="dimmed">
                {episodeLabel(t, {
                  seasonNumber: broadcast.seasonNumber,
                  episodeNumber: broadcast.episodeNumber,
                })}
              </Text>
            )}
            <Broadcast
              broadcast={broadcast}
              now={now}
              canEdit={canEdit}
              pending={pending}
              busy={busy}
              onReserve={onReserve}
            />
          </Stack>
        )}
        {next != null && (
          <Group gap="xs">
            {/* 次の話が進んだら、ボタンを作り直す。 */}
            <DramaWatchedButton
              key={`${next.seasonNumber}:${next.episodeNumber}`}
              episode={{
                seriesId: work.series.tmdbId,
                seasonNumber: next.seasonNumber,
                episodeNumber: next.episodeNumber,
              }}
              watched={false}
              onChange={onChange}
              label={t('drama.library.markNext', { episode: episodeLabel(t, next) })}
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
                  key={`${episode.seasonNumber}-${episode.episodeNumber}`}
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
      </Stack>
    </Card>
  );
}

function RecordedEpisodeButton({ episode }: { episode: RecordedEpisode }) {
  const { t } = useTranslation();
  const label = episodeLabel(t, episode);

  return (
    <Button
      size="compact-xs"
      variant={episode.watched === false ? 'light' : 'default'}
      leftSection={<Play size={12} aria-hidden />}
      title={episode.name ?? undefined}
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
