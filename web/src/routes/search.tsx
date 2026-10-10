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
import { Suspense, useState, type FormEvent } from 'react';
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
  const [results, setResults] = useState<Result[] | null>(null);
  const [next, setNext] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [selected, setSelected] = useState<string | undefined>(undefined);
  const [creatingRule, setCreatingRule] = useState(false);
  // searched は、結果を出している検索の条件。入力を変えても、続きは同じ条件で読む。
  const [searched, setSearched] = useState<searchProgramsQuery['variables'] | null>(null);
  const conditions = toConditions(form);

  const load = async (variables: searchProgramsQuery['variables'], append: boolean) => {
    setLoading(true);
    setError(false);
    try {
      const data = await fetchQuery<searchProgramsQuery>(environment, query, variables, {
        fetchPolicy: 'network-only',
      }).toPromise();
      const page = data?.searchPrograms;
      if (page === undefined) {
        throw new Error('no data');
      }
      const nodes = page.edges.map((edge) => edge.node);
      setResults((prev) => (append && prev !== null ? [...prev, ...nodes] : nodes));
      // 照合する番組の数の上限に達すると、first 件に満たなくても続きがある。
      setNext(page.pageInfo.hasNextPage ? (page.pageInfo.endCursor ?? null) : null);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
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
    setSearched(variables);
    void load(variables, false);
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
            <Button type="submit" loading={loading && searched?.after == null}>
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
      {results !== null && results.length === 0 && next === null && (
        <Alert color="gray">{t('search.empty')}</Alert>
      )}
      {results !== null && results.length > 0 && (
        <Stack gap={4}>
          {results.map((program) => (
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
      {next !== null && searched !== null && (
        <Button
          variant="default"
          loading={loading}
          onClick={() => void load({ ...searched, after: next }, true)}
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
