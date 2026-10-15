import {
  Alert,
  Anchor,
  Box,
  Button,
  Card,
  Group,
  MultiSelect,
  SimpleGrid,
  Stack,
  TextInput,
  Title,
} from '@mantine/core';
import { Clapperboard, Info, ListFilter, Search, SearchX, Tag } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { EmptyState, ErrorBoundary, PageLoader } from '../../futon';
import { RecordingCard } from '../../recordings/RecordingCard';
import { recordingStates, stateKey } from '../../recordings/states';
import { isUnauthenticated } from '../../relay/environment';
import type {
  recordingsListQuery,
  recordingsListQuery$data,
  recordingsListQuery$variables,
} from './__generated__/recordingsListQuery.graphql';
import type { recordingsTagsQuery } from './__generated__/recordingsTagsQuery.graphql';

export const Route = createFileRoute('/recordings/')({
  component: RecordingsPage,
});

// pageSize は、1 回に取る録画の数。
const pageSize = 48;

const query = graphql`
  query recordingsListQuery($first: Int!, $after: String, $filter: RecordingFilter) {
    recordings(first: $first, after: $after, filter: $filter) {
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
          tags {
            id
            name
          }
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
`;

type Recording = recordingsListQuery$data['recordings']['edges'][number]['node'];
type Filter = NonNullable<recordingsListQuery$variables['filter']>;

// RecordingsPage は、サムネイル付きの録画のグリッドの画面 (設計書 10.3)。タグと条件で絞り込める。
function RecordingsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('recordings.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Recordings />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// Recordings は、絞り込みのフォーム。結果は、絞り込みを決めてから RecordingGrid で読む。
function Recordings() {
  const { t } = useTranslation();
  const tags = useLazyLoadQuery<recordingsTagsQuery>(
    graphql`
      query recordingsTagsQuery {
        tags {
          id
          name
          recordings
        }
      }
    `,
    {},
    { fetchPolicy: 'store-and-network' },
  );
  const [keyword, setKeyword] = useState('');
  const [tagIds, setTagIds] = useState<string[]>([]);
  const [states, setStates] = useState<string[]>([]);
  // filter は、結果に適用している絞り込み。最初は絞り込まずに、新しい録画から出す。
  const [filter, setFilter] = useState<Filter>({});

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setFilter({
      ...(keyword.trim() !== '' ? { keyword: keyword.trim() } : {}),
      ...(tagIds.length > 0 ? { tagIds } : {}),
      ...(states.length > 0 ? { states: states as Filter['states'] } : {}),
    });
  };

  return (
    <Stack>
      <Card withBorder component="form" onSubmit={submit}>
        <Group align="flex-end" wrap="wrap">
          <TextInput
            label={t('recordings.keyword')}
            leftSection={<Search size={16} aria-hidden />}
            value={keyword}
            onChange={(e) => setKeyword(e.currentTarget.value)}
            style={{ flex: '1 1 200px' }}
          />
          <MultiSelect
            label={t('recordings.tags')}
            leftSection={<Tag size={16} aria-hidden />}
            data={tags.tags.map((tag) => ({
              value: tag.id,
              label: `${tag.name}（${tag.recordings}）`,
            }))}
            value={tagIds}
            onChange={setTagIds}
            searchable
            clearable
            style={{ flex: '1 1 200px' }}
          />
          <MultiSelect
            label={t('recordings.states')}
            leftSection={<Info size={16} aria-hidden />}
            data={recordingStates.map((state) => ({ value: state, label: t(stateKey(state)) }))}
            value={states}
            onChange={setStates}
            clearable
            style={{ flex: '1 1 200px' }}
          />
          <Button type="submit" leftSection={<ListFilter size={16} aria-hidden />}>
            {t('recordings.filter')}
          </Button>
        </Group>
      </Card>
      <Suspense fallback={<PageLoader />}>
        <RecordingGrid key={JSON.stringify(filter)} filter={filter} />
      </Suspense>
    </Stack>
  );
}

// RecordingGrid は、絞り込んだ録画のグリッド。続きは、同じ絞り込みで、カーソルから読む。
function RecordingGrid({ filter }: { filter: Filter }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const data = useLazyLoadQuery<recordingsListQuery>(
    query,
    { first: pageSize, after: null, filter },
    { fetchPolicy: 'store-and-network' },
  );
  // more は、続きの読み込みで足した録画と、その次のカーソル。
  const [more, setMore] = useState<{ recordings: Recording[]; next: string | null } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const recordings = [
    ...data.recordings.edges.map((edge) => edge.node),
    ...(more?.recordings ?? []),
  ];
  const next =
    more !== null
      ? more.next
      : data.recordings.pageInfo.hasNextPage
        ? (data.recordings.pageInfo.endCursor ?? null)
        : null;

  const loadMore = async () => {
    setLoading(true);
    setError(false);
    try {
      const page = await fetchQuery<recordingsListQuery>(environment, query, {
        first: pageSize,
        after: next,
        filter,
      }).toPromise();
      if (page !== undefined) {
        setMore({
          recordings: [
            ...(more?.recordings ?? []),
            ...page.recordings.edges.map((edge) => edge.node),
          ],
          next: page.recordings.pageInfo.hasNextPage
            ? (page.recordings.pageInfo.endCursor ?? null)
            : null,
        });
      }
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Stack>
      {/* 録画がまだないときと、絞り込んで 0 件になったときで、次にすることが違うので、見出しと手がかりを分ける。 */}
      {recordings.length === 0 &&
        (Object.keys(filter).length === 0 ? (
          <EmptyState
            icon={Clapperboard}
            title={t('recordings.empty')}
            hint={t('recordings.emptyHint')}
          />
        ) : (
          <EmptyState
            icon={SearchX}
            title={t('recordings.noMatch')}
            hint={t('recordings.noMatchHint')}
          />
        ))}
      {recordings.length > 0 && (
        <SimpleGrid cols={{ base: 2, md: 3, lg: 4 }} spacing="sm">
          {recordings.map((recording) => (
            <RecordingCard key={recording.id} recording={recording} />
          ))}
        </SimpleGrid>
      )}
      {error && (
        <Alert color="red" role="alert">
          {t('recordings.error')}
        </Alert>
      )}
      {next !== null && (
        <Box>
          <Button variant="default" loading={loading} onClick={() => void loadMore()}>
            {t('recordings.more')}
          </Button>
        </Box>
      )}
    </Stack>
  );
}
