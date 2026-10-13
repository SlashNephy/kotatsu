import {
  Alert,
  Anchor,
  Button,
  Group,
  SegmentedControl,
  Select,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Suspense, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { GuideGrid, type GuideColumn } from '../../guide/GuideGrid';
import { GuideViewEditor, type EditableGuideView } from '../../guide/GuideViewEditor';
import { broadcastDay } from '../../guide/layout';
import { ProgramDetail } from '../../guide/ProgramDetail';
import { collapseSharedColumns } from '../../guide/sharedEvent';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { guideQuery } from './__generated__/guideQuery.graphql';

type GuideSearch = {
  // date は、表示する放送日 (YYYY-MM-DD)。省くと、いまの放送日。
  date?: string;
  // type は、表示するチャンネル種別 (GR など)。省くと、最初の種別。
  type?: string;
  // program は、詳細を開いている番組の ID。
  program?: string;
  // view は、適用する番組表ビューの ID。省くと、絞り込まない。
  view?: string;
};

const datePattern = /^\d{4}-\d{2}-\d{2}$/;
const day = 24 * 60 * 60_000;
// guideDays は、番組表で移れる、いまの放送日からの日数。tenban は 7 日分までの時間帯を受け付ける。
const guideDays = 7;

export const Route = createFileRoute('/guide/')({
  validateSearch: (search: Record<string, unknown>): GuideSearch => {
    const text = (key: string) =>
      typeof search[key] === 'string' && search[key] !== '' ? search[key] : undefined;
    const date = text('date');
    const type = text('type');
    const program = text('program');
    const view = text('view');

    return {
      ...(date !== undefined && datePattern.test(date) ? { date } : {}),
      ...(type !== undefined ? { type } : {}),
      ...(program !== undefined ? { program } : {}),
      ...(view !== undefined ? { view } : {}),
    };
  },
  component: GuidePage,
});

// dateOf は、放送日の始まりの時刻を YYYY-MM-DD (日本時間) にする。
function dateOf(time: number): string {
  return new Date(time + 9 * 3_600_000).toISOString().slice(0, 10);
}

function GuidePage() {
  const { t } = useTranslation();
  const navigate = useNavigate({ from: '/guide/' });
  const { view } = Route.useSearch();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('guide.title')}</Title>
      {/* ビューを外したら、エラーの表示を消して読み直す。 */}
      <ErrorBoundary
        key={view ?? ''}
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : view !== undefined && graphQLErrorDetails(error).code === 'NOT_FOUND' ? (
            // 消したビューのリンクを開いたときは、ビューを外して開き直せるようにする。
            <Alert color="amber">
              {t('guide.view.notFound')}{' '}
              <Anchor
                component="button"
                type="button"
                onClick={() => void navigate({ search: (prev) => ({ ...prev, view: undefined }) })}
              >
                {t('guide.view.showAll')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Guide />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Guide() {
  const { t } = useTranslation();
  const navigate = useNavigate({ from: '/guide/' });
  const search = Route.useSearch();
  // now は、画面を開いた時刻。日付の移動の基準と、いまの時刻の線に使う。
  const [now] = useState(() => Date.now());
  const today = broadcastDay(now);
  const window = useMemo(() => {
    const from =
      search.date === undefined
        ? today.from
        : broadcastDay(Date.parse(`${search.date}T12:00:00+09:00`)).from;

    return { from, to: from + day };
  }, [search.date, today.from]);
  // fetchKey は、番組表ビューを保存したときに、番組表とビューの一覧を取り直すための合図。
  const [fetchKey, setFetchKey] = useState(0);
  // editing は、編集中の番組表ビュー。
  const [editing, setEditing] = useState<EditableGuideView | null>(null);
  const data = useLazyLoadQuery<guideQuery>(
    graphql`
      query guideQuery($from: Time!, $to: Time!, $view: ID) {
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
        guideViews {
          id
          name
          genres {
            lv1
            lv2
          }
          excludeGenres {
            lv1
            lv2
          }
          minDurationSeconds
          services {
            id
          }
          hideEmptyServices
        }
        guide(from: $from, to: $to, view: $view) {
          service {
            id
            name
            logoUrl
            channelType {
              type
            }
          }
          programs {
            id
            startAt
            endAt
            endUndecided
            name
            genres {
              lv1
            }
            sharedFrom {
              id
            }
            reservation {
              id
              enabled
            }
          }
        }
      }
    `,
    {
      from: new Date(window.from).toISOString(),
      to: new Date(window.to).toISOString(),
      view: search.view ?? null,
    },
    // fetchKey が変わると、Relay は fetchPolicy によらず取り直す。
    { fetchKey, fetchPolicy: 'store-or-network' },
  );
  const currentView = data.guideViews.find((view) => view.id === search.view);

  const types = data.channelTypes.filter((channelType) =>
    data.guide.some((row) => row.service.channelType.type === channelType.type),
  );
  // URL の種別が、この日の番組表にないときは、最初の種別にする。
  const type = types.some((channelType) => channelType.type === search.type)
    ? search.type
    : types[0]?.type;
  // サブチャンネルは、元のサービスと違う番組を流す時間がある日だけ列を出し、同じ番組の枠は薄くする。
  const columns = useMemo<GuideColumn[]>(
    () =>
      collapseSharedColumns(data.guide.map((row) => ({ ...row, id: row.service.id })))
        .filter((row) => row.service.channelType.type === type)
        .map((row) => ({
          id: row.service.id,
          name: row.service.name,
          logoUrl: row.service.logoUrl ?? null,
          programs: row.programs.map((program) => ({
            id: program.id,
            startAt: Date.parse(program.startAt),
            endAt: Date.parse(program.endAt),
            endUndecided: program.endUndecided,
            name: program.name,
            genre: program.genres[0]?.lv1,
            shared: program.shared,
            reserved: program.reservation?.enabled === true,
          })),
        })),
    [data.guide, type],
  );

  const offset = Math.round((window.from - today.from) / day);
  const moveTo = (days: number) =>
    void navigate({
      search: (prev) => ({ ...prev, date: dateOf(today.from + days * day), program: undefined }),
    });
  const dayFormat = new Intl.DateTimeFormat('ja-JP', {
    month: 'numeric',
    day: 'numeric',
    weekday: 'short',
    timeZone: 'Asia/Tokyo',
  });

  return (
    <Stack gap="sm">
      <Group justify="space-between" wrap="wrap" gap="xs">
        <Group gap="xs">
          <Button
            variant="default"
            size="xs"
            disabled={offset <= 0}
            onClick={() => moveTo(offset - 1)}
            leftSection={<ChevronLeft size={14} aria-hidden />}
          >
            {t('guide.previousDay')}
          </Button>
          <Text fw={700} miw={96} ta="center">
            {dayFormat.format(window.from)}
          </Text>
          <Button
            variant="default"
            size="xs"
            disabled={offset >= guideDays - 1}
            onClick={() => moveTo(offset + 1)}
            rightSection={<ChevronRight size={14} aria-hidden />}
          >
            {t('guide.nextDay')}
          </Button>
          <Button
            variant="default"
            size="xs"
            disabled={offset === 0}
            onClick={() => moveTo(0)}
            leftSection={<CalendarDays size={14} aria-hidden />}
          >
            {t('guide.today')}
          </Button>
        </Group>
        <Group gap="xs">
          <Select
            size="xs"
            aria-label={t('guide.view.label')}
            // URL のビューが消えていても、「すべての番組」を選び直して外せるよう、URL の値をそのまま使う。
            value={search.view ?? ''}
            onChange={(value) =>
              void navigate({
                search: (prev) => ({
                  ...prev,
                  view: value === '' ? undefined : (value ?? undefined),
                }),
              })
            }
            data={[
              { value: '', label: t('guide.view.all') },
              ...data.guideViews.map((view) => ({ value: view.id, label: view.name })),
            ]}
            allowDeselect={false}
          />
          <Button
            variant="default"
            size="xs"
            onClick={() =>
              setEditing(
                currentView === undefined
                  ? {
                      id: undefined,
                      name: '',
                      genres: [],
                      excludeGenres: [],
                      minDurationSeconds: null,
                      services: [],
                      hideEmptyServices: false,
                    }
                  : {
                      id: currentView.id,
                      name: currentView.name,
                      genres: currentView.genres,
                      excludeGenres: currentView.excludeGenres,
                      minDurationSeconds: currentView.minDurationSeconds,
                      services: currentView.services.map((service) => service.id),
                      hideEmptyServices: currentView.hideEmptyServices,
                    },
              )
            }
          >
            {currentView === undefined ? t('guide.view.create') : t('guide.view.edit')}
          </Button>
        </Group>
        {types.length > 0 && (
          <SegmentedControl
            size="xs"
            value={type ?? ''}
            onChange={(value) =>
              void navigate({ search: (prev) => ({ ...prev, type: value, program: undefined }) })
            }
            data={types.map((channelType) => ({
              value: channelType.type,
              label: channelType.displayName,
            }))}
          />
        )}
      </Group>
      {columns.length === 0 ? (
        <Alert color="gray">{t('guide.empty')}</Alert>
      ) : (
        <GuideGrid
          window={window}
          columns={columns}
          now={now}
          initialTime={offset === 0 ? now : window.from + 15 * 3_600_000}
          onSelect={(program) => void navigate({ search: (prev) => ({ ...prev, program }) })}
        />
      )}
      {editing !== null && (
        <GuideViewEditor
          view={editing}
          services={data.services}
          onClose={() => setEditing(null)}
          onSaved={(id) => {
            setEditing(null);
            setFetchKey((key) => key + 1);
            void navigate({ search: (prev) => ({ ...prev, view: id }) });
          }}
        />
      )}
      <ProgramDetail
        programId={search.program}
        onClose={() => void navigate({ search: (prev) => ({ ...prev, program: undefined }) })}
      />
    </Stack>
  );
}
