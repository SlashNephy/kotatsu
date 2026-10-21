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
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';

import { formatBytes, zonedFormat } from '../futon';
import { mutate } from '../settings/mutate';
import type { ReauthRun } from '../settings/useReauthGate';
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
      segmentDir
      size
      modifiedAt
    }
  }
`;

const deleteMutation = graphql`
  mutation OrphanFilesDeleteMutation($files: [OrphanFileInput!]!) {
    deleteOrphanFiles(files: $files) {
      deleted
      skipped
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

// keyOf は、選んだファイルを覚えておくためのキー。プール (データディレクトリなら null) とパスと、生成物のディレクトリかどうかで決まる。
const keyOf = (orphan: Orphan) => JSON.stringify([orphan.poolId, orphan.path, orphan.segmentDir]);

// OrphanFiles は、プールとデータディレクトリにあるのに DB に記録のないファイルを探し、確かめてから消す (設計書 5.6)。
// 録画を消すのに失敗したときや、取り込みをやり直したときに残るので、管理者が片付ける。
// ファイルを消すのは重要な操作なので、ページの再認証 (run) を通して送る (設計書 9.5)。
export function OrphanFiles({ run: reauthRun }: { run: ReauthRun }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [found, setFound] = useState<readonly Orphan[] | null>(null);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [searching, setSearching] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [deleting, setDeleting] = useState(false);

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
  // 途中で再認証を求められたら、再認証した後、まだ送っていない分から続ける。
  const run = () => {
    const files = all
      .filter((orphan) => selected.has(keyOf(orphan)))
      .map(({ poolId, path, segmentDir }) => ({ poolId, path, segmentDir }));
    const total = { deleted: 0, skipped: 0 };
    let next = 0;
    // 再認証の画面と重ならないよう、確かめる画面は先に閉じる。
    setConfirming(false);
    const finish = (text: { ok: boolean; text: string } | null) => {
      setDeleting(false);
      if (text !== null) {
        setMessage(text);
        void search(true);
      }
    };
    reauthRun(
      async () => {
        setDeleting(true);
        for (; next < files.length; next += deleteBatch) {
          const { deleteOrphanFiles: r } = await mutate<OrphanFilesDeleteMutation>(
            environment,
            deleteMutation,
            { files: files.slice(next, next + deleteBatch) },
          );
          total.deleted += r.deleted;
          total.skipped += r.skipped;
        }
        finish({ ok: true, text: t('admin.orphan.deleted', total) });
      },
      () => finish({ ok: false, text: t('admin.orphan.deleteError') }),
      () => finish(null),
    );
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
                  // データディレクトリはプールではないので、プールの ID の代わりに名前を出す。
                  const place = orphan.poolId ?? t('admin.orphan.dataDir');
                  const label = `${place}:${orphan.path}`;

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
                          {place}:
                        </Text>
                        {orphan.path}
                        {orphan.segmentDir && (
                          <Text span size="xs" c="dimmed">
                            {' '}
                            {t('admin.orphan.segmentDir')}
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
                loading={deleting}
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
            <Button color="red" onClick={run}>
              {t('admin.orphan.confirmButton')}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Card>
  );
}
