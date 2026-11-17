import { Alert, Anchor, Group, SegmentedControl, Select, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';

import { periods } from '../anime/library';
import { libraryStatuses, type LibraryStatus } from '../anime/status';
import { DramaLibrary } from '../drama/DramaLibrary';
import { dramaFiltersOf, parseDramaSearch, type DramaSearch } from '../drama/library';
import { ErrorBoundary, PageLoader, useDocumentTitle } from '../futon';
import { isUnauthenticated } from '../relay/environment';

export const Route = createFileRoute('/drama')({
  // 既定の値 (見ている、絞り込まない) は、URL に載せない。
  validateSearch: (search: Record<string, unknown>): DramaSearch => parseDramaSearch(search),
  component: DramaPage,
});

const weekdays = [1, 2, 3, 4, 5, 6, 0] as const;

// DramaPage は、ドラマの視聴管理の画面 (設計書 3.9、10.3、#1652)。見ている作品を、作品ごとに、次の話と次の放送と、
// まだ見ていない録画と並べる。
function DramaPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('drama.library.title')]);
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const filters = dramaFiltersOf(search);
  // update は、絞り込みを変える。既定の値は、URL から外す。
  const update = (changes: Partial<DramaSearch>) =>
    void navigate({
      search: (prev) =>
        Object.fromEntries(
          Object.entries({ ...prev, ...changes }).filter(([, value]) => value !== undefined),
        ) as DramaSearch,
    });

  return (
    <Stack gap="sm">
      <Title order={2}>{t('drama.library.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('drama.library.description')}
      </Text>
      <Group gap="sm" align="flex-end">
        <SegmentedControl
          size="xs"
          aria-label={t('anime.statusFilter')}
          value={filters.status}
          onChange={(value) =>
            update({ status: value === 'WATCHING' ? undefined : (value as LibraryStatus) })
          }
          data={libraryStatuses.map((value) => ({
            value,
            label: t(`drama.library.status.${value}`),
          }))}
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
            <Alert color="amber">{t('drama.library.loadError')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <DramaLibrary filters={filters} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}
