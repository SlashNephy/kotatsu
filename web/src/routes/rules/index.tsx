import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Checkbox,
  Group,
  Modal,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { ListChecks, Plus, Trash2 } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { useDataChanges } from '../../changes/DataChanges';
import type { DataChannel } from '../../changes/dataChanges';
import { EmptyState, ErrorBoundary, PageLoader } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import { outOfValidity } from '../../rules/validity';
import type { rulesDeleteManyMutation } from './__generated__/rulesDeleteManyMutation.graphql';
import type { rulesEnableMutation } from './__generated__/rulesEnableMutation.graphql';
import type { rulesListQuery } from './__generated__/rulesListQuery.graphql';
import type { rulesSetManyEnabledMutation } from './__generated__/rulesSetManyEnabledMutation.graphql';

export const Route = createFileRoute('/rules/')({
  component: RulesPage,
});

// RulesPage は、ルールの一覧の画面 (設計書 10.3)。ルールは家族で共有する。
function RulesPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('rules.title')}</Title>
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
          <Rules />
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
          updatedAt
          upcomingReservationCount
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
  const [toggleError, setToggleError] = useState(false);
  // selecting は、まとめて操作するルールを選んでいるかどうか。selected は、選んだルールの ID (#471)。
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  // bulkResult は、まとめて行った操作の結果。
  const [bulkResult, setBulkResult] = useState<{
    action: 'enabled' | 'disabled' | 'deleted';
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
    action: 'enabled' | 'disabled' | 'deleted',
    result: { processed: number; skipped: number },
  ) => {
    setBulkResult({ action, ...result });
    setSelecting(false);
    setSelected(new Set());
    setConfirmingDelete(false);
  };
  const failed = () => {
    setConfirmingDelete(false);
    setToggleError(true);
  };
  const setEnabledMany = (enabled: boolean) =>
    setManyEnabled({
      variables: { ids: [...selected], enabled },
      // 一覧の有効と無効の表示は、ルールごとに読み直さずに、その場で変える。
      updater: (store) => {
        for (const id of selected) {
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
      variables: { ids: [...selected] },
      // 消したルールを、一覧から外す。
      updater: (store) => {
        const root = store.getRoot();
        const rules = root.getLinkedRecords('rules') ?? [];
        root.setLinkedRecords(
          rules.filter((rule) => !selected.has(rule.getDataID())),
          'rules',
        );
      },
      onCompleted: (response, errors) =>
        errors != null && errors.length > 0 ? failed() : finish('deleted', response.deleteRules),
      onError: failed,
    });

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
        </Group>
      )}
      {selecting && (
        <Group gap="xs">
          <Button
            size="xs"
            variant="default"
            disabled={selected.size === 0}
            loading={settingMany}
            onClick={() => setEnabledMany(true)}
          >
            {t('rules.enableSelected', { count: selected.size })}
          </Button>
          <Button
            size="xs"
            variant="default"
            disabled={selected.size === 0}
            loading={settingMany}
            onClick={() => setEnabledMany(false)}
          >
            {t('rules.disableSelected', { count: selected.size })}
          </Button>
          <Button
            size="xs"
            color="red"
            variant="light"
            leftSection={<Trash2 size={14} aria-hidden />}
            disabled={selected.size === 0}
            onClick={() => setConfirmingDelete(true)}
          >
            {t('rules.deleteSelected', { count: selected.size })}
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
          <Text size="sm">{t('rules.deleteSelectedConfirm', { count: selected.size })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirmingDelete(false)}>
              {t('rules.cancel')}
            </Button>
            <Button color="red" loading={deletingMany} onClick={removeMany}>
              {t('rules.deleteSelected', { count: selected.size })}
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
      {data.rules.map((rule) => (
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
                    {data.channelTypes.find((channelType) => channelType.type === type)
                      ?.displayName ?? type}
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
              {/* このルールの予約へ移れるようにする (#468)。 */}
              <Anchor
                size="xs"
                c="dimmed"
                renderRoot={(props) => (
                  <Link {...props} to="/reservations" search={{ rule: rule.id }} />
                )}
              >
                {t('rules.upcomingReservations', { count: rule.upcomingReservationCount })}
              </Anchor>
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
      ))}
    </Stack>
  );
}
