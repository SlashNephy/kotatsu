import {
  Alert,
  Button,
  Card,
  Checkbox,
  Group,
  Modal,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useMutation, useRelayEnvironment } from 'react-relay';

import { formatBytes, zonedFormat } from '../futon';
import type {
  OrphanFilesQuery,
  OrphanFilesQuery$data,
} from './__generated__/OrphanFilesQuery.graphql';
import type { OrphanFilesDeleteMutation } from './__generated__/OrphanFilesDeleteMutation.graphql';

const query = graphql`
  query OrphanFilesQuery {
    orphanFiles {
      poolId
      path
      sidecar
      size
      modifiedAt
    }
  }
`;

type Orphan = OrphanFilesQuery$data['orphanFiles'][number];

// deleteBatch は、1 回の deleteOrphanFiles で送るファイルの数 (サーバーの上限)。
const deleteBatch = 500;

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// keyOf は、選んだファイルを覚えておくためのキー。プールとパスと、付属ファイルのディレクトリかどうかで決まる。
const keyOf = (orphan: Orphan) => JSON.stringify([orphan.poolId, orphan.path, orphan.sidecar]);

// OrphanFiles は、プールにあるのに DB に記録のないファイルを探し、確かめてから消す (設計書 5.6)。
// 録画を消すのに失敗したときや、取り込みをやり直したときに残るので、管理者が片付ける。
export function OrphanFiles() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [found, setFound] = useState<readonly Orphan[] | null>(null);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [searching, setSearching] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [remove] = useMutation<OrphanFilesDeleteMutation>(graphql`
    mutation OrphanFilesDeleteMutation($files: [OrphanFileInput!]!) {
      deleteOrphanFiles(files: $files) {
        deleted
        skipped
      }
    }
  `);

  // search は、DB にないファイルを探す。消した後に探し直すときは、消した結果の表示を残す (keepMessage)。
  // 消すかどうかは一つずつ確かめてほしいので、はじめは何も選ばない。
  const search = async (keepMessage = false) => {
    setSearching(true);
    if (!keepMessage) {
      setMessage(null);
    }
    try {
      const data = await fetchQuery<OrphanFilesQuery>(
        environment,
        query,
        {},
        { fetchPolicy: 'network-only' },
      ).toPromise();
      setFound(data?.orphanFiles ?? []);
      setSelected(new Set());
    } catch {
      setMessage({ ok: false, text: t('admin.orphan.searchError') });
    } finally {
      setSearching(false);
    }
  };

  const toggle = (key: string, checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(key);
      } else {
        next.delete(key);
      }

      return next;
    });

  const all = found ?? [];
  const allSelected = all.length > 0 && selected.size === all.length;
  const toggleAll = (checked: boolean) => setSelected(new Set(checked ? all.map(keyOf) : []));

  // run は、選んだファイルを、サーバーの上限 (1 回に 500 件) ごとに分けて消す。
  const run = async () => {
    const files = all
      .filter((orphan) => selected.has(keyOf(orphan)))
      .map(({ poolId, path, sidecar }) => ({ poolId, path, sidecar }));
    const total = { deleted: 0, skipped: 0 };
    setDeleting(true);
    try {
      for (let start = 0; start < files.length; start += deleteBatch) {
        const r = await new Promise<{ deleted: number; skipped: number }>((resolve, reject) => {
          remove({
            variables: { files: files.slice(start, start + deleteBatch) },
            onCompleted: (response, errors) =>
              errors != null && errors.length > 0
                ? reject(new Error(errors[0]?.message ?? 'GraphQL error'))
                : resolve(response.deleteOrphanFiles),
            onError: reject,
          });
        });
        total.deleted += r.deleted;
        total.skipped += r.skipped;
      }
      setMessage({ ok: true, text: t('admin.orphan.deleted', total) });
    } catch {
      setMessage({ ok: false, text: t('admin.orphan.deleteError') });
    } finally {
      setDeleting(false);
      setConfirming(false);
      void search(true);
    }
  };

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('admin.orphan.title')}
          </Title>
          <Button size="xs" variant="default" loading={searching} onClick={() => void search()}>
            {t('admin.orphan.search')}
          </Button>
        </Group>
        <Text size="xs" c="dimmed">
          {t('admin.orphan.description')}
        </Text>
        {message !== null && (
          <Alert color={message.ok ? 'teal' : 'red'} role={message.ok ? 'status' : 'alert'}>
            {message.text}
          </Alert>
        )}
        {found !== null && found.length === 0 && (
          <Text size="sm" c="dimmed">
            {t('admin.orphan.none')}
          </Text>
        )}
        {found !== null && found.length > 0 && (
          <>
            <Table fz="sm" verticalSpacing={4}>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th w={40}>
                    <Checkbox
                      aria-label={t('admin.orphan.selectAll')}
                      checked={allSelected}
                      indeterminate={selected.size > 0 && !allSelected}
                      onChange={(event) => toggleAll(event.currentTarget.checked)}
                    />
                  </Table.Th>
                  <Table.Th>{t('admin.orphan.path')}</Table.Th>
                  <Table.Th>{t('admin.orphan.size')}</Table.Th>
                  <Table.Th>{t('admin.orphan.modifiedAt')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {found.map((orphan) => {
                  const key = keyOf(orphan);
                  const label = `${orphan.poolId}:${orphan.path}`;

                  return (
                    <Table.Tr key={key}>
                      <Table.Td>
                        <Checkbox
                          aria-label={t('admin.orphan.select', { name: label })}
                          checked={selected.has(key)}
                          onChange={(event) => toggle(key, event.currentTarget.checked)}
                        />
                      </Table.Td>
                      <Table.Td style={{ wordBreak: 'break-all' }}>
                        <Text span size="xs" c="dimmed">
                          {orphan.poolId}:
                        </Text>
                        {orphan.path}
                        {orphan.sidecar && (
                          <Text span size="xs" c="dimmed">
                            {' '}
                            {t('admin.orphan.sidecar')}
                          </Text>
                        )}
                      </Table.Td>
                      <Table.Td style={{ whiteSpace: 'nowrap' }}>
                        {formatBytes(orphan.size)}
                      </Table.Td>
                      <Table.Td style={{ whiteSpace: 'nowrap' }}>
                        {dateTimeFormat.format(Date.parse(orphan.modifiedAt))}
                      </Table.Td>
                    </Table.Tr>
                  );
                })}
              </Table.Tbody>
            </Table>
            <Group justify="flex-end">
              <Button
                color="red"
                disabled={selected.size === 0}
                onClick={() => setConfirming(true)}
              >
                {t('admin.orphan.delete', { count: selected.size })}
              </Button>
            </Group>
          </>
        )}
      </Stack>
      <Modal
        opened={confirming}
        onClose={() => setConfirming(false)}
        title={t('admin.orphan.confirmTitle')}
      >
        <Stack>
          <Text size="sm">{t('admin.orphan.confirm', { count: selected.size })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirming(false)}>
              {t('admin.orphan.cancel')}
            </Button>
            <Button color="red" loading={deleting} onClick={() => void run()}>
              {t('admin.orphan.confirmButton')}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Card>
  );
}
