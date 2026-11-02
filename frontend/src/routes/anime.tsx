import {
  Alert,
  Anchor,
  Checkbox,
  Group,
  MultiSelect,
  SegmentedControl,
  Select,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

import { AnimeLibrary, seasonLabel } from '../anime/AnimeLibrary';
import { currentSeason, periods, type AnimeSeason } from '../anime/library';
import {
  filtersOf,
  libraryStatuses,
  parseAnimeSearch,
  type AnimeSearch,
  type LibraryStatus,
} from '../anime/status';
import {
  loadStreamingServices,
  saveStreamingServices,
  streamingServices,
} from '../anime/streaming';
import { ErrorBoundary, PageLoader, useDocumentTitle } from '../futon';
import { isUnauthenticated } from '../relay/environment';

export const Route = createFileRoute('/anime')({
  // 既定の値 (見てる、絞り込まない) は、URL に載せない。
  validateSearch: (search: Record<string, unknown>): AnimeSearch => parseAnimeSearch(search),
  component: AnimePage,
});

// AnimePage は、視聴管理の画面 (設計書 10.3、#1231)。見たいリストの作品を、作品ごとに、次の話と次の放送と、まだ見ていない録画と並べる。
function AnimePage() {
  const { t } = useTranslation();
  useDocumentTitle([t('anime.title')]);
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const filters = filtersOf(search);
  // 作品を探すリンクを出す配信サービス (#1235)。端末ごとに覚える。
  const [streaming, setStreaming] = useState(loadStreamingServices);
  // update は、絞り込みを変える。既定の値は、URL から外す。
  const update = (changes: Partial<AnimeSearch>) =>
    void navigate({
      search: (prev) =>
        Object.fromEntries(
          Object.entries({ ...prev, ...changes }).filter(([, value]) => value !== undefined),
        ) as AnimeSearch,
    });

  return (
    <Stack gap="sm">
      <Title order={2}>{t('anime.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('anime.description')}{' '}
        <Anchor
          size="sm"
          renderRoot={(props) => <Link {...props} to="/reservations" search={{ wishlist: true }} />}
        >
          {t('anime.openWishlist')}
        </Anchor>
      </Text>
      <Group gap="sm" align="flex-end">
        <SegmentedControl
          size="xs"
          aria-label={t('anime.statusFilter')}
          value={filters.status}
          onChange={(value) =>
            update({ status: value === 'WATCHING' ? undefined : (value as LibraryStatus) })
          }
          data={libraryStatuses.map((value) => ({ value, label: t(`anime.status.${value}`) }))}
        />
        <Select
          size="xs"
          w={140}
          aria-label={t('anime.seasonFilter')}
          placeholder={t('anime.allSeasons')}
          clearable
          value={filters.season ?? null}
          onChange={(value) => update({ season: value ?? undefined })}
          data={seasonOptions(t)}
        />
        <Select
          size="xs"
          w={140}
          aria-label={t('anime.periodFilter')}
          placeholder={t('anime.allPeriods')}
          clearable
          value={filters.period ?? null}
          onChange={(value) => update({ period: periods.find((period) => period === value) })}
          data={periods.map((value) => ({ value, label: t(`anime.period.${value}`) }))}
        />
        <Select
          size="xs"
          w={120}
          aria-label={t('anime.weekdayFilter')}
          placeholder={t('anime.allWeekdays')}
          clearable
          value={filters.weekday?.toString() ?? null}
          onChange={(value) => update({ weekday: value == null ? undefined : Number(value) })}
          data={weekdays.map((day) => ({
            value: day.toString(),
            label: t(`anime.weekdays.${day}`),
          }))}
        />
        <MultiSelect
          size="xs"
          w={260}
          aria-label={t('anime.streaming.choose')}
          placeholder={streaming.length === 0 ? t('anime.streaming.choose') : undefined}
          clearable
          value={[...streaming]}
          onChange={(values) => {
            const next = streamingServices.filter((service) => values.includes(service));
            setStreaming(next);
            saveStreamingServices(next);
          }}
          data={streamingServices.map((value) => ({
            value,
            label: t(`anime.streaming.services.${value}`),
          }))}
        />
        <Checkbox
          size="xs"
          label={t('anime.excludeReruns')}
          checked={filters.excludeReruns}
          onChange={(event) =>
            update({ rerun: event.currentTarget.checked ? 'exclude' : undefined })
          }
        />
      </Group>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('anime.loadError')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <AnimeLibrary filters={filters} streaming={streaming} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

const weekdays = [1, 2, 3, 4, 5, 6, 0] as const;
const seasonNames: readonly AnimeSeason[] = ['WINTER', 'SPRING', 'SUMMER', 'AUTUMN'];

// seasonOptions は、時期の絞り込みの候補。今期と、今期から 2 年前までの時期を、新しい順に並べる。
function seasonOptions(t: TFunction): { value: string; label: string }[] {
  const now = currentSeason(Date.now());
  const options: { value: string; label: string }[] = [
    { value: 'current', label: t('anime.currentSeason') },
  ];
  let index = now.year * 4 + seasonNames.indexOf(now.season);
  for (let i = 0; i < 9; i++, index--) {
    const year = Math.floor(index / 4);
    const season = seasonNames[index % 4] ?? 'WINTER';
    options.push({
      value: `${year}-${season}`,
      label: t('anime.season', { year, season: seasonLabel(t, season) }),
    });
  }

  return options;
}
