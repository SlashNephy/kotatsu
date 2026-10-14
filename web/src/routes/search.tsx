import {
  Alert,
  Anchor,
  Button,
  Card,
  Group,
  Loader,
  NumberInput,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { History, ListPlus, SearchX } from 'lucide-react';
import { Suspense, useEffect, useEffectEvent, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { ConditionsFields } from '../conditions/ConditionsFields';
import {
  emptyConditions,
  emptyKeyword,
  hasConditions,
  toConditionsInput,
  type ConditionsForm,
} from '../conditions/form';
import { ProgramList } from '../conditions/ProgramList';
import { EmptyState, ErrorBoundary, PageLoader } from '../futon';
import { ProgramDetail } from '../guide/ProgramDetail';
import { isUnauthenticated } from '../relay/environment';
import type {
  searchProgramsQuery,
  searchProgramsQuery$data,
} from './__generated__/searchProgramsQuery.graphql';
import type { searchViewerQuery } from './__generated__/searchViewerQuery.graphql';
import classes from './search.module.css';

export const Route = createFileRoute('/search')({
  component: SearchPage,
});

const day = 24 * 60 * 60_000;
// pageSize は、1 回に取る件数。
const pageSize = 50;
// debounceMs は、入力が止まってから検索するまでの時間。打つたびに問い合わせないようにする。
const debounceMs = 400;

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
            logoUrl
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
// 条件の入力欄は、ルールの画面と同じものを使い、左に条件、右に結果を並べる。
function SearchPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm" className={classes.page}>
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
        <Suspense fallback={<PageLoader />}>
          <Search />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Search() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const navigate = useNavigate();
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
        services {
          id
          name
          logoUrl
          serviceId
          channelType {
            displayName
          }
        }
      }
    `,
    {},
  );
  // キーワードの行を 1 つ開けておき、すぐに打ち始められるようにする。
  const [conditions, setConditions] = useState<ConditionsForm>(() => ({
    ...emptyConditions(),
    keywords: [emptyKeyword()],
  }));
  const [includeArchive, setIncludeArchive] = useState(false);
  const [archiveDays, setArchiveDays] = useState(30);
  const [page, setPage] = useState<Page | null>(null);
  // loading は、読み込み中の操作。新しい検索と、続きの読み込みで、表示を分ける。
  const [loading, setLoading] = useState<'search' | 'more' | null>(null);
  const [error, setError] = useState(false);
  // request は、最後に始めた読み込みの番号。古い読み込みの応答は捨てる。
  const request = useRef(0);
  const [selected, setSelected] = useState<string | undefined>(undefined);
  const enabled = hasConditions(conditions);
  // 条件の中身が変わったときだけ検索し直す。
  const key = JSON.stringify({
    conditions: toConditionsInput(conditions),
    archiveDays: includeArchive ? archiveDays : null,
  });

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

  // search は、いまの条件で、新しく検索する。過去の放送の期間は、検索するときの時刻から決める。
  const search = useEffectEvent(() => {
    const now = Date.now();
    void load(
      {
        conditions: toConditionsInput(conditions),
        archive: includeArchive
          ? {
              from: new Date(now - archiveDays * day).toISOString(),
              to: new Date(now).toISOString(),
            }
          : null,
        first: pageSize,
        after: null,
      },
      [],
      'search',
    );
  });

  // 条件を変えたら、入力が止まるのを待ってから検索する。条件を消したら、結果と読み込み中の表示を消す。
  useEffect(() => {
    if (!enabled) {
      request.current++;
      setPage(null);
      setLoading(null);
      setError(false);

      return;
    }
    const timer = setTimeout(search, debounceMs);

    return () => clearTimeout(timer);
  }, [key, enabled]);

  // createRule は、いまの条件を引き継いで、ルールの作成画面を開く。ルールの名前は、最初のキーワードにしておく。
  const createRule = () =>
    void navigate({
      to: '/rules/$ruleId',
      params: { ruleId: 'new' },
      state: { ruleConditions: conditions },
    });
  const canCreateRule = viewer.viewer.role !== 'VIEWER';

  return (
    // 広い画面では、画面の高さに収め、条件と結果をそれぞれの列の中でスクロールする。
    <div className={classes.layout}>
      <div className={classes.pane}>
        <Stack>
          <ConditionsFields
            value={conditions}
            onChange={(patch) => setConditions((prev) => ({ ...prev, ...patch }))}
            channelTypes={viewer.channelTypes}
            services={viewer.services}
          />
          <Card withBorder>
            <Group gap="sm" wrap="wrap">
              <Switch
                label={t('search.includeArchive')}
                checked={includeArchive}
                onChange={(e) => setIncludeArchive(e.currentTarget.checked)}
              />
              {includeArchive && (
                <NumberInput
                  aria-label={t('search.archiveDays')}
                  size="xs"
                  w={150}
                  leftSection={<History size={14} aria-hidden />}
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
          </Card>
        </Stack>
      </div>
      {/* 結果は、見出しの行を留め、その下の一覧だけをスクロールする。 */}
      <div className={classes.resultsPane}>
        <Card withBorder className={classes.results}>
          <Group justify="space-between" wrap="nowrap" mb="xs">
            <Group gap="xs">
              <Title order={3} size="h5">
                {t('search.results')}
              </Title>
              {loading === 'search' && <Loader size="xs" />}
            </Group>
            {canCreateRule && (
              <Button
                size="xs"
                variant="default"
                leftSection={<ListPlus size={16} aria-hidden />}
                disabled={!enabled}
                onClick={createRule}
              >
                {t('search.createRule')}
              </Button>
            )}
          </Group>
          <Stack gap="xs" className={classes.resultsBody}>
            {!enabled && (
              <Text size="sm" c="dimmed">
                {t('search.noConditions')}
              </Text>
            )}
            {error && (
              <Alert color="red" role="alert">
                {t('search.error')}
              </Alert>
            )}
            {enabled && page !== null && page.results.length === 0 && page.next === null && (
              <EmptyState icon={SearchX} title={t('search.empty')} hint={t('search.emptyHint')} />
            )}
            {enabled && page !== null && page.results.length > 0 && (
              <ProgramList programs={page.results} onSelect={setSelected} />
            )}
            {enabled && page?.next != null && (
              <Button
                variant="default"
                loading={loading === 'more'}
                // 新しい検索の最中は、前の検索の続きを読まない。
                disabled={loading === 'search'}
                onClick={() =>
                  void load({ ...page.variables, after: page.next }, page.results, 'more')
                }
              >
                {t('search.more')}
              </Button>
            )}
          </Stack>
        </Card>
      </div>
      <ProgramDetail programId={selected} onClose={() => setSelected(undefined)} />
    </div>
  );
}
