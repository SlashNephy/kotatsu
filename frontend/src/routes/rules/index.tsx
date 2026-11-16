import {
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Checkbox,
  Group,
  Modal,
  Select,
  Stack,
  Switch,
  Tabs,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import {
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  FolderInput,
  ListChecks,
  Plus,
  Search,
  Trash2,
} from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import type { DataChannel } from '../../changes/dataChanges';
import { EmptyState, ErrorBoundary, PageLoader, useDocumentTitle } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import { loadCollapsedGroups, saveCollapsedGroups } from '../../rules/collapsedGroups';
import {
  isFiltered,
  parseRuleListSearch,
  ruleReservationFilters,
  ruleSections,
  ruleSortKeys,
  ruleStateFilters,
  sectionKey,
  visibleRules,
  type RuleListSearch,
  type RuleReservationFilter,
  type RuleSortKey,
  type RuleStateFilter,
} from '../../rules/listFilter';
import { CreateRuleGroupButton, RuleGroupHeader } from '../../rules/RuleGroupHeader';
import { RuleGroupSelect } from '../../rules/RuleGroupSelect';
import { outOfValidity } from '../../rules/validity';
import { WatchRuleList } from '../../watch/WatchRuleList';
import type { rulesDeleteManyMutation } from './__generated__/rulesDeleteManyMutation.graphql';
import type { rulesEnableMutation } from './__generated__/rulesEnableMutation.graphql';
import type { rulesListQuery } from './__generated__/rulesListQuery.graphql';
import type { rulesSetManyEnabledMutation } from './__generated__/rulesSetManyEnabledMutation.graphql';
import type { rulesSetManyGroupMutation } from './__generated__/rulesSetManyGroupMutation.graphql';

type RulesSearch = RuleListSearch & {
  // watch は、録画のルールの代わりに、自分の視聴ルールの一覧を出すかどうか (設計書 4.8)。
  watch?: boolean;
};

export const Route = createFileRoute('/rules/')({
  // 探す語、絞り込み、並べ替えは、戻ったときに保つよう、URL に持つ (#1493)。既定の値は載せない。
  validateSearch: (search: Record<string, unknown>): RulesSearch => ({
    ...(search['watch'] === true ? { watch: true } : {}),
    ...parseRuleListSearch(search),
  }),
  component: RulesPage,
});

// RulesPage は、ルールの一覧の画面 (設計書 10.3)。ルールは家族で共有する。
function RulesPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('rules.title')]);
  const { watch } = Route.useSearch();
  const navigate = Route.useNavigate();
  const tab = watch === true ? 'watch' : 'recording';

  return (
    <Stack gap="sm">
      <Title order={2}>{t('rules.title')}</Title>
      {/* 録画のルールは家族で共有し、視聴ルールはユーザーごとのものなので、タブで分ける (設計書 4.8)。 */}
      <Tabs
        value={tab}
        onChange={(value) => void navigate({ search: value === 'watch' ? { watch: true } : {} })}
      >
        <Tabs.List>
          <Tabs.Tab value="recording">{t('watchRules.recordingTab')}</Tabs.Tab>
          <Tabs.Tab value="watch">{t('watchRules.tab')}</Tabs.Tab>
        </Tabs.List>
      </Tabs>
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
          {tab === 'watch' ? <WatchRuleList /> : <Rules />}
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// ruleChannels は、ルールの一覧に関わるデータの種類。
const ruleChannels: readonly DataChannel[] = ['RULES', 'RESERVATIONS'];

function Rules() {
  const { t } = useTranslation();
  // ルールと予約の件数は、ほかの端末や neko で変わるので、変わったら読み直す (#503)。
  const changes = useDataChanges(ruleChannels);
  const [now] = useState(Date.now);
  const data = useLazyLoadQuery<rulesListQuery>(
    graphql`
      query rulesListQuery {
        viewer {
          role
        }
        channelTypes {
          type
          displayName
        }
        ruleGroups {
          id
          name
        }
        rules {
          id
          name
          enabled
          memo
          conditions {
            keywords {
              text
            }
            channelTypes
          }
          validFrom
          validUntil
          createdAt
          updatedAt
          upcomingReservationCount
          group {
            id
          }
        }
      }
    `,
    {},
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const [commit] = useMutation<rulesEnableMutation>(graphql`
    mutation rulesEnableMutation($id: ID!, $enabled: Boolean!) {
      setRuleEnabled(id: $id, enabled: $enabled) {
        id
        enabled
      }
    }
  `);
  const canEdit = data.viewer.role !== 'VIEWER';
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  // query は、検索の入力。打つたびに URL を置き換えるが、空白を打った途中の値も残すよう、入力は手元に持つ。
  const [query, setQuery] = useState(search.q ?? '');
  // 上のメニューから開き直すなど、入力の外で URL の語が変わったら、入力をそれに合わせる。
  const [syncedQuery, setSyncedQuery] = useState(search.q);
  if (search.q !== syncedQuery) {
    setSyncedQuery(search.q);
    if ((search.q ?? '') !== query && !(search.q === undefined && query.trim() === '')) {
      setQuery(search.q ?? '');
    }
  }
  // update は、探す語、絞り込み、並べ替えを変える。履歴を積まず、URL を置き換える。既定の値は、URL から外す。
  const update = (changes: Partial<RuleListSearch>) =>
    void navigate({
      replace: true,
      search: (prev) => ({
        ...(prev.watch === true ? { watch: true } : {}),
        ...parseRuleListSearch({ ...prev, ...changes }),
      }),
    });
  const clearFilters = () => {
    setQuery('');
    update({ q: undefined, state: undefined, reservations: undefined });
  };
  const visible = visibleRules(data.rules, search, now);
  // グループがあれば、グループごとの見出しに分ける (#1493)。並べ替えは、見出しの中で効く。
  // 絞り込んでいないときは、ルールのないグループの見出しも出す。
  const grouped = data.ruleGroups.length > 0;
  const sections = ruleSections(visible, data.ruleGroups, !isFiltered(search));
  // collapsed は、折りたたんだ見出しの鍵。端末に覚え、ルールを開いてから戻っても保つ。
  const [collapsed, setCollapsed] = useState(loadCollapsedGroups);
  // 探す語や絞り込みがあるときは、当たったルールを隠さないよう、すべての見出しを開く。
  const collapsible = grouped && !isFiltered(search);
  const isCollapsed = (key: string) => collapsible && collapsed.has(key);
  const toggleSection = (key: string) => {
    const next = new Set(collapsed);
    if (!next.delete(key)) {
      next.add(key);
    }
    setCollapsed(next);
    saveCollapsedGroups(next);
  };
  // shown は、いま一覧に出しているルール。折りたたんだ見出しの下のルールは含めない。
  const shown = sections
    .filter((section) => !isCollapsed(sectionKey(section.group)))
    .flatMap((section) => section.rules);
  const [toggleError, setToggleError] = useState(false);
  // selecting は、まとめて操作するルールを選んでいるかどうか。selected は、選んだルールの ID (#471)。
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  // targets は、まとめて操作するルールの ID。選んだルールのうち、いま一覧に出しているものだけにする。
  // 選んだ後に絞り込みを変えたり、見出しを折りたたんだりしても、見えていないルールを操作しないためである。
  const targets = shown.filter((rule) => selected.has(rule.id)).map((rule) => rule.id);
  const allShownSelected = shown.length > 0 && targets.length === shown.length;
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  // moving は、選んだルールをグループへ移すダイアログを開いているかどうか。moveTo は、移す先のグループ (null なら「その他」)。
  const [moving, setMoving] = useState(false);
  const [moveTo, setMoveTo] = useState<string | null>(null);
  // bulkResult は、まとめて行った操作の結果。
  const [bulkResult, setBulkResult] = useState<{
    action: 'enabled' | 'disabled' | 'deleted' | 'moved';
    processed: number;
    skipped: number;
  } | null>(null);
  const [setManyEnabled, settingMany] = useMutation<rulesSetManyEnabledMutation>(graphql`
    mutation rulesSetManyEnabledMutation($ids: [ID!]!, $enabled: Boolean!) {
      setRulesEnabled(ids: $ids, enabled: $enabled) {
        processed
        skipped
      }
    }
  `);
  const [setManyGroup, movingMany] = useMutation<rulesSetManyGroupMutation>(graphql`
    mutation rulesSetManyGroupMutation($ids: [ID!]!, $groupId: ID) {
      setRulesGroup(ids: $ids, groupId: $groupId) {
        processed
        skipped
      }
    }
  `);
  const [deleteMany, deletingMany] = useMutation<rulesDeleteManyMutation>(graphql`
    mutation rulesDeleteManyMutation($ids: [ID!]!) {
      deleteRules(ids: $ids) {
        processed
        skipped
      }
    }
  `);
  const toggle = (id: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (!next.delete(id)) {
        next.add(id);
      }

      return next;
    });
  // finish は、まとめて行った操作を終えて、選ぶのをやめ、結果を出す。一覧は、データの変更の通知で読み直す。
  const finish = (
    action: 'enabled' | 'disabled' | 'deleted' | 'moved',
    result: { processed: number; skipped: number },
  ) => {
    setBulkResult({ action, ...result });
    setSelecting(false);
    setSelected(new Set());
    setConfirmingDelete(false);
    setMoving(false);
  };
  const failed = () => {
    setConfirmingDelete(false);
    setMoving(false);
    setToggleError(true);
  };
  const moveMany = () =>
    setManyGroup({
      variables: { ids: targets, groupId: moveTo },
      // 一覧の見出しは、ルールごとに読み直さずに、その場で移す。
      updater: (store) => {
        const group = moveTo === null ? null : store.get(moveTo);
        for (const id of targets) {
          const rule = store.get(id);
          if (group == null) {
            rule?.setValue(null, 'group');
          } else {
            rule?.setLinkedRecord(group, 'group');
          }
        }
      },
      onCompleted: (response, errors) =>
        errors != null && errors.length > 0 ? failed() : finish('moved', response.setRulesGroup),
      onError: failed,
    });
  const setEnabledMany = (enabled: boolean) =>
    setManyEnabled({
      variables: { ids: targets, enabled },
      // 一覧の有効と無効の表示は、ルールごとに読み直さずに、その場で変える。
      updater: (store) => {
        for (const id of targets) {
          store.get(id)?.setValue(enabled, 'enabled');
        }
      },
      onCompleted: (response, errors) =>
        errors != null && errors.length > 0
          ? failed()
          : finish(enabled ? 'enabled' : 'disabled', response.setRulesEnabled),
      onError: failed,
    });
  const removeMany = () =>
    deleteMany({
      variables: { ids: targets },
      // 消したルールを、一覧から外す。
      updater: (store) => {
        const root = store.getRoot();
        const rules = root.getLinkedRecords('rules') ?? [];
        root.setLinkedRecords(
          rules.filter((rule) => !targets.includes(rule.getDataID())),
          'rules',
        );
      },
      onCompleted: (response, errors) =>
        errors != null && errors.length > 0 ? failed() : finish('deleted', response.deleteRules),
      onError: failed,
    });

  // renderRule は、一覧のルール 1 件を描く。
  const renderRule = (rule: (typeof visible)[number]) => (
    <Card key={rule.id} withBorder padding="sm" style={{ opacity: rule.enabled ? 1 : 0.6 }}>
      <Group justify="space-between" wrap="nowrap">
        {selecting && (
          <Checkbox
            aria-label={rule.name}
            checked={selected.has(rule.id)}
            onChange={() => toggle(rule.id)}
          />
        )}
        <Stack gap={2} style={{ minWidth: 0, flex: 1 }}>
          <Anchor
            fw={700}
            renderRoot={(props) => (
              <Link {...props} to="/rules/$ruleId" params={{ ruleId: rule.id }} />
            )}
          >
            {rule.name}
          </Anchor>
          <Group gap={4}>
            {rule.conditions.keywords.map((keyword, index) => (
              <Badge key={index} variant="light" color="gray" size="sm">
                {keyword.text}
              </Badge>
            ))}
            {rule.conditions.channelTypes.map((type) => (
              <Badge key={type} variant="outline" color="gray" size="sm">
                {/* GR などの内部の種別ではなく、番組表と同じ表示名で示す。 */}
                {data.channelTypes.find((channelType) => channelType.type === type)?.displayName ??
                  type}
              </Badge>
            ))}
            {!rule.enabled && (
              <Badge color="gray" size="sm">
                {t('rules.disabled')}
              </Badge>
            )}
            {/* 有効でも、有効期間の外なら予約しないので、そのことを示す (#464)。 */}
            {rule.enabled && outOfValidity(rule.validFrom, rule.validUntil, now) && (
              <Badge color="amber" variant="light" size="sm">
                {t('rules.outOfValidity')}
              </Badge>
            )}
          </Group>
          {rule.memo !== '' && (
            <Text size="xs" c="dimmed" truncate>
              {rule.memo}
            </Text>
          )}
          {/* このルールの予約と、このルールで録った録画へ移れるようにする (#468)。 */}
          <Group gap="sm">
            <Anchor
              size="xs"
              c="dimmed"
              renderRoot={(props) => (
                <Link {...props} to="/reservations" search={{ rule: rule.id }} />
              )}
            >
              {t('rules.upcomingReservations', { count: rule.upcomingReservationCount })}
            </Anchor>
            <Anchor
              size="xs"
              c="dimmed"
              renderRoot={(props) => (
                <Link {...props} to="/recordings" search={{ rule: rule.id }} />
              )}
            >
              {t('rules.recordings')}
            </Anchor>
          </Group>
        </Stack>
        {canEdit && (
          <Switch
            aria-label={t('rules.enabled')}
            checked={rule.enabled}
            onChange={(e) =>
              commit({
                variables: { id: rule.id, enabled: e.currentTarget.checked },
                onCompleted: (_response, errors) =>
                  setToggleError(errors != null && errors.length > 0),
                onError: () => setToggleError(true),
              })
            }
          />
        )}
      </Group>
    </Card>
  );

  return (
    <Stack>
      {canEdit && (
        <Group>
          <Button
            leftSection={<Plus size={16} aria-hidden />}
            renderRoot={(props) => (
              <Link {...props} to="/rules/$ruleId" params={{ ruleId: 'new' }} />
            )}
          >
            {t('rules.create')}
          </Button>
          {/* ルールを選んで、まとめて有効、無効にしたり、消したりする (#471)。 */}
          {data.rules.length > 0 && (
            <Button
              variant={selecting ? 'light' : 'default'}
              leftSection={<ListChecks size={16} aria-hidden />}
              onClick={() => {
                setSelecting(!selecting);
                setSelected(new Set());
              }}
            >
              {t(selecting ? 'rules.selectDone' : 'rules.select')}
            </Button>
          )}
          {/* グループは、ルールの一覧を分けて並べるためのもの (#1493)。ルールの編集の画面でも作れる。 */}
          <CreateRuleGroupButton />
        </Group>
      )}
      {data.rules.length > 0 && (
        <Group gap="xs" align="flex-end">
          <TextInput
            leftSection={<Search size={16} aria-hidden />}
            aria-label={t('rules.filter.search')}
            placeholder={t('rules.filter.search')}
            value={query}
            onChange={(event) => {
              const value = event.currentTarget.value;
              setQuery(value);
              update({ q: value });
            }}
            style={{ flex: '1 1 16rem' }}
          />
          <Select
            aria-label={t('rules.filter.state')}
            data={[
              { value: 'all', label: t('rules.filter.stateAll') },
              ...ruleStateFilters.map((value) => ({
                value,
                label: t(`rules.filter.states.${value}`),
              })),
            ]}
            value={search.state ?? 'all'}
            allowDeselect={false}
            onChange={(value) =>
              update({
                state: value === null || value === 'all' ? undefined : (value as RuleStateFilter),
              })
            }
            w={140}
          />
          <Select
            aria-label={t('rules.filter.reservations')}
            data={[
              { value: 'all', label: t('rules.filter.reservationsAll') },
              ...ruleReservationFilters.map((value) => ({
                value,
                label: t(`rules.filter.reservationStates.${value}`),
              })),
            ]}
            value={search.reservations ?? 'all'}
            allowDeselect={false}
            onChange={(value) =>
              update({
                reservations:
                  value === null || value === 'all' ? undefined : (value as RuleReservationFilter),
              })
            }
            w={200}
          />
          <Group gap={4} wrap="nowrap">
            <Select
              aria-label={t('rules.sort.label')}
              data={ruleSortKeys.map((value) => ({ value, label: t(`rules.sort.keys.${value}`) }))}
              value={search.sort ?? 'created'}
              allowDeselect={false}
              onChange={(value) =>
                update({
                  sort:
                    value === null || value === 'created'
                      ? undefined
                      : (value as Exclude<RuleSortKey, 'created'>),
                })
              }
              w={200}
            />
            <ActionIcon
              variant="default"
              size="input-sm"
              aria-label={t(
                search.order === 'desc' ? 'rules.sort.descending' : 'rules.sort.ascending',
              )}
              title={t(search.order === 'desc' ? 'rules.sort.descending' : 'rules.sort.ascending')}
              onClick={() => update({ order: search.order === 'desc' ? undefined : 'desc' })}
            >
              {search.order === 'desc' ? (
                <ArrowDownWideNarrow size={16} aria-hidden />
              ) : (
                <ArrowUpNarrowWide size={16} aria-hidden />
              )}
            </ActionIcon>
          </Group>
        </Group>
      )}
      {isFiltered(search) && data.rules.length > 0 && (
        <Text size="sm" c="dimmed" role="status">
          {t('rules.filter.count', { shown: visible.length, total: data.rules.length })}
        </Text>
      )}
      {selecting && (
        <Group gap="xs">
          {/* 絞り込んだルールを、まとめて選ぶ。終わったルールを探して、まとめて無効にするのに使う (#1493)。 */}
          <Button
            size="xs"
            variant="default"
            disabled={shown.length === 0}
            onClick={() =>
              setSelected((current) => {
                // 一覧に出しているルールだけを、選ぶか外す。隠れているルールの選び方は変えない。
                const next = new Set(current);
                for (const rule of shown) {
                  if (allShownSelected) {
                    next.delete(rule.id);
                  } else {
                    next.add(rule.id);
                  }
                }

                return next;
              })
            }
          >
            {t(allShownSelected ? 'rules.deselectShown' : 'rules.selectShown', {
              count: shown.length,
            })}
          </Button>
          <Button
            size="xs"
            variant="default"
            disabled={targets.length === 0}
            loading={settingMany}
            onClick={() => setEnabledMany(true)}
          >
            {t('rules.enableSelected', { count: targets.length })}
          </Button>
          <Button
            size="xs"
            variant="default"
            disabled={targets.length === 0}
            loading={settingMany}
            onClick={() => setEnabledMany(false)}
          >
            {t('rules.disableSelected', { count: targets.length })}
          </Button>
          <Button
            size="xs"
            variant="default"
            leftSection={<FolderInput size={14} aria-hidden />}
            disabled={targets.length === 0}
            onClick={() => {
              setMoveTo(null);
              setMoving(true);
            }}
          >
            {t('rules.groups.moveSelected', { count: targets.length })}
          </Button>
          <Button
            size="xs"
            color="red"
            variant="light"
            leftSection={<Trash2 size={14} aria-hidden />}
            disabled={targets.length === 0}
            onClick={() => setConfirmingDelete(true)}
          >
            {t('rules.deleteSelected', { count: targets.length })}
          </Button>
        </Group>
      )}
      {bulkResult !== null && (
        <Alert color="teal" role="status" withCloseButton onClose={() => setBulkResult(null)}>
          {t(`rules.bulk.${bulkResult.action}`, { count: bulkResult.processed })}
          {bulkResult.skipped > 0 && t('rules.bulk.skipped', { count: bulkResult.skipped })}
        </Alert>
      )}
      <Modal
        opened={confirmingDelete}
        onClose={() => setConfirmingDelete(false)}
        title={t('rules.deleteSelectedTitle')}
      >
        <Stack>
          <Text size="sm">{t('rules.deleteSelectedConfirm', { count: targets.length })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirmingDelete(false)}>
              {t('rules.cancel')}
            </Button>
            <Button color="red" loading={deletingMany} onClick={removeMany}>
              {t('rules.deleteSelected', { count: targets.length })}
            </Button>
          </Group>
        </Stack>
      </Modal>
      <Modal opened={moving} onClose={() => setMoving(false)} title={t('rules.groups.moveTitle')}>
        <Stack>
          <RuleGroupSelect
            label={t('rules.groups.moveTo')}
            groups={data.ruleGroups}
            value={moveTo}
            onChange={setMoveTo}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setMoving(false)}>
              {t('rules.cancel')}
            </Button>
            <Button loading={movingMany} onClick={moveMany}>
              {t('rules.groups.moveSelected', { count: targets.length })}
            </Button>
          </Group>
        </Stack>
      </Modal>
      {toggleError && (
        <Alert color="red" role="alert">
          {t('reservations.toggleError')}
        </Alert>
      )}
      {data.rules.length === 0 && (
        <EmptyState icon={ListChecks} title={t('rules.empty')} hint={t('rules.emptyHint')} />
      )}
      {data.rules.length > 0 && visible.length === 0 && (
        <Stack gap="xs">
          <EmptyState icon={Search} title={t('rules.filter.empty')} />
          <Button
            size="xs"
            variant="default"
            onClick={clearFilters}
            style={{ alignSelf: 'flex-start' }}
          >
            {t('rules.filter.clear')}
          </Button>
        </Stack>
      )}
      {sections.map((section) => {
        const key = sectionKey(section.group);

        return (
          <Stack key={key} gap="sm">
            {grouped && (
              <RuleGroupHeader
                group={section.group}
                count={section.rules.length}
                collapsible={collapsible}
                collapsed={isCollapsed(key)}
                onToggle={() => toggleSection(key)}
                groups={data.ruleGroups}
                canEdit={canEdit}
                onError={() => setToggleError(true)}
              />
            )}
            {!isCollapsed(key) && section.rules.map(renderRule)}
          </Stack>
        );
      })}
    </Stack>
  );
}
