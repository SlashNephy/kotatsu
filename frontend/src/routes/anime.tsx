import { Alert, Anchor, Group, SegmentedControl, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';

import { AnimeLibrary } from '../anime/AnimeLibrary';
import { libraryStatuses, parseLibraryStatus, type LibraryStatus } from '../anime/status';
import { ErrorBoundary, PageLoader, useDocumentTitle } from '../futon';
import { isUnauthenticated } from '../relay/environment';

type AnimeSearch = { status?: LibraryStatus };

export const Route = createFileRoute('/anime')({
  // 既定の「見てる」は、URL に載せない。
  validateSearch: (search: Record<string, unknown>): AnimeSearch => {
    const status = parseLibraryStatus(search['status']);

    return status === 'WATCHING' ? {} : { status };
  },
  component: AnimePage,
});

// AnimePage は、視聴管理の画面 (設計書 10.3、#1231)。見たいリストの作品を、作品ごとに、次の話と次の放送と、まだ見ていない録画と並べる。
function AnimePage() {
  const { t } = useTranslation();
  useDocumentTitle([t('anime.title')]);
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const status = search.status ?? 'WATCHING';

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
      <Group>
        <SegmentedControl
          size="xs"
          aria-label={t('anime.statusFilter')}
          value={status}
          onChange={(value) =>
            void navigate({
              search: value === 'WATCHING' ? {} : { status: value as LibraryStatus },
            })
          }
          data={libraryStatuses.map((value) => ({ value, label: t(`anime.status.${value}`) }))}
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
          <AnimeLibrary status={status} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}
