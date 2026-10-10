import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  MultiSelect,
  NumberInput,
  SegmentedControl,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
  UnstyledButton,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { ErrorBoundary } from '../futon';
import { genreKey } from '../guide/genres';
import { ProgramDetail } from '../guide/ProgramDetail';
import { isUnauthenticated } from '../relay/environment';
import { isEmpty, toConditions, type KeywordScope, type SearchForm } from '../search/conditions';
import { CreateRuleDialog } from '../search/CreateRuleDialog';
import type {
  searchProgramsQuery,
  searchProgramsQuery$data,
} from './__generated__/searchProgramsQuery.graphql';
import type { searchViewerQuery } from './__generated__/searchViewerQuery.graphql';

export const Route = createFileRoute('/search')({
  component: SearchPage,
});

const day = 24 * 60 * 60_000;
// pageSize は、1 回に取る件数。
const pageSize = 50;
const selectableGenres = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 15];

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

const query = graphql`
  query searchProgramsQuery(
    $conditions: ProgramConditionsInput!
    $archive: ArchiveRangeInput
    $first: Int!
    $after: String
  ) {
    searchPrograms(conditions: $conditions, archive: $archive, first: $first, after: $after) {
      edges {
        node {
          id
          name
          startAt
          archived
          service {
            name
          }
          genres {
            lv1
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

type Result = searchProgramsQuery$data['searchPrograms']['edges'][number]['node'];

// Page は、表示している検索の結果。続きは、同じ条件と、その条件で得たカーソルで読む。
type Page = {
  variables: searchProgramsQuery['variables'];
  results: Result[];
  // next は、続きのカーソル。続きがなければ null。
  next: string | null;
};

// SearchPage は、番組を検索する画面 (設計書 10.3)。過去の放送も検索でき、検索の条件からルールを作れる。
function SearchPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('search.title')}</Title>
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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Search />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Search() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const viewer = useLazyLoadQuery<searchViewerQuery>(
    graphql`
      query searchViewerQuery {
        viewer {
          role
        }
        channelTypes {
          type
          displayName
        }
      }
    `,
    {},
  );
  const [form, setForm] = useState<SearchForm>({
    keyword: '',
    scope: 'name',
    excludeKeyword: '',
    channelTypes: [],
    genres: [],
    freeOnly: false,
  });
  const [includeArchive, setIncludeArchive] = useState(false);
  const [archiveDays, setArchiveDays] = useState(30);
  const [page, setPage] = useState<Page | null>(null);
  // loading は、読み込み中の操作。新しい検索と、続きの読み込みで、ボタンの表示を分ける。
  const [loading, setLoading] = useState<'search' | 'more' | null>(null);
  const [error, setError] = useState(false);
  // request は、最後に始めた読み込みの番号。古い読み込みの応答は捨てる。
  const request = useRef(0);
  const [selected, setSelected] = useState<string | undefined>(undefined);
  const [creatingRule, setCreatingRule] = useState(false);
  const conditions = toConditions(form);

  // load は、variables で検索し、previous の後ろに足す。新しい検索では、previous を空にする。
  // 失敗したときは、表示している結果とカーソルの組を変えないので、続きを別の条件で読むことはない。
  const load = async (
    variables: searchProgramsQuery['variables'],
    previous: Result[],
    kind: 'search' | 'more',
  ) => {
    const id = ++request.current;
    setLoading(kind);
    setError(false);
    try {
      const data = await fetchQuery<searchProgramsQuery>(environment, query, variables, {
        fetchPolicy: 'network-only',
      }).toPromise();
      if (id !== request.current) {
        return;
      }
      const result = data?.searchPrograms;
      if (result === undefined) {
        throw new Error('no data');
      }
      setPage({
        variables,
        results: [...previous, ...result.edges.map((edge) => edge.node)],
        // 照合する番組の数の上限に達すると、first 件に満たなくても続きがある。
        next: result.pageInfo.hasNextPage ? (result.pageInfo.endCursor ?? null) : null,
      });
    } catch {
      if (id === request.current) {
        setError(true);
      }
    } finally {
      if (id === request.current) {
        setLoading(null);
      }
    }
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const now = Date.now();
    const variables = {
      conditions,
      archive: includeArchive
        ? { from: new Date(now - archiveDays * day).toISOString(), to: new Date(now).toISOString() }
        : null,
      first: pageSize,
      after: null,
    };
    void load(variables, [], 'search');
  };

  const update = (patch: Partial<SearchForm>) => setForm((prev) => ({ ...prev, ...patch }));
  const canCreateRule = viewer.viewer.role !== 'VIEWER';

  return (
    <Stack>
      <Card withBorder component="form" onSubmit={submit}>
        <Stack gap="sm">
          <Group align="flex-end" wrap="wrap">
            <TextInput
              label={t('search.keyword')}
              placeholder={t('search.keywordPlaceholder')}
              value={form.keyword}
              onChange={(e) => update({ keyword: e.currentTarget.value })}
              style={{ flex: '1 1 240px' }}
            />
            <SegmentedControl
              value={form.scope}
              onChange={(value) => update({ scope: value as KeywordScope })}
              data={[
                { value: 'name', label: t('search.scope.name') },
                { value: 'nameAndDescription', label: t('search.scope.nameAndDescription') },
                { value: 'all', label: t('search.scope.all') },
              ]}
            />
          </Group>
          <Group align="flex-start" wrap="wrap">
            <TextInput
              label={t('search.excludeKeyword')}
              value={form.excludeKeyword}
              onChange={(e) => update({ excludeKeyword: e.currentTarget.value })}
              style={{ flex: '1 1 200px' }}
            />
            <MultiSelect
              label={t('search.channelTypes')}
              data={viewer.channelTypes.map((type) => ({
                value: type.type,
                label: type.displayName,
              }))}
              value={form.channelTypes}
              onChange={(channelTypes) => update({ channelTypes })}
              clearable
              style={{ flex: '1 1 200px' }}
            />
            <MultiSelect
              label={t('search.genres')}
              data={selectableGenres.map((lv1) => ({
                value: String(lv1),
                label: t(genreKey(lv1)),
              }))}
              value={form.genres.map(String)}
              onChange={(genres) => update({ genres: genres.map(Number) })}
              clearable
              style={{ flex: '1 1 200px' }}
            />
          </Group>
          <Group wrap="wrap">
            <Switch
              label={t('search.freeOnly')}
              checked={form.freeOnly}
              onChange={(e) => update({ freeOnly: e.currentTarget.checked })}
            />
            <Switch
              label={t('search.includeArchive')}
              checked={includeArchive}
              onChange={(e) => setIncludeArchive(e.currentTarget.checked)}
            />
            {includeArchive && (
              <NumberInput
                aria-label={t('search.archiveDays')}
                size="xs"
                w={140}
                value={archiveDays}
                onChange={(value) => setArchiveDays(typeof value === 'number' ? value : 30)}
                min={1}
                max={3650}
                allowDecimal={false}
                prefix={t('search.archiveDaysPrefix')}
                suffix={t('search.archiveDaysSuffix')}
              />
            )}
          </Group>
          <Group justify="space-between">
            {canCreateRule ? (
              <Button
                variant="default"
                disabled={isEmpty(conditions)}
                onClick={() => setCreatingRule(true)}
              >
                {t('search.createRule')}
              </Button>
            ) : (
              <span />
            )}
            <Button type="submit" loading={loading === 'search'}>
              {t('search.submit')}
            </Button>
          </Group>
        </Stack>
      </Card>
      {error && (
        <Alert color="red" role="alert">
          {t('search.error')}
        </Alert>
      )}
      {page !== null && page.results.length === 0 && page.next === null && (
        <Alert color="gray">{t('search.empty')}</Alert>
      )}
      {page !== null && page.results.length > 0 && (
        <Stack gap={4}>
          {page.results.map((program) => (
            <UnstyledButton
              key={program.id}
              onClick={() => setSelected(program.id)}
              p="xs"
              style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}
            >
              <Group gap="xs" wrap="nowrap">
                <Text size="sm" c="dimmed" w={190} style={{ flexShrink: 0 }}>
                  {dateTimeFormat.format(Date.parse(program.startAt))}
                </Text>
                <Text size="sm" w={140} truncate style={{ flexShrink: 0 }}>
                  {program.service.name}
                </Text>
                <Text size="sm" fw={700} truncate style={{ flex: 1 }}>
                  {program.name}
                </Text>
                {program.archived && <Badge color="gray">{t('search.archived')}</Badge>}
              </Group>
            </UnstyledButton>
          ))}
        </Stack>
      )}
      {page?.next != null && (
        <Button
          variant="default"
          loading={loading === 'more'}
          // 新しい検索の最中は、前の検索の続きを読まない。
          disabled={loading === 'search'}
          onClick={() => void load({ ...page.variables, after: page.next }, page.results, 'more')}
        >
          {t('search.more')}
        </Button>
      )}
      <ProgramDetail programId={selected} onClose={() => setSelected(undefined)} />
      {creatingRule && (
        <CreateRuleDialog
          conditions={conditions}
          defaultName={form.keyword.trim()}
          onClose={() => setCreatingRule(false)}
        />
      )}
    </Stack>
  );
}
