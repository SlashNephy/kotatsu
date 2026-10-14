import {
  Alert,
  Button,
  Card,
  Checkbox,
  Code,
  Group,
  Modal,
  Progress,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useMutation, useRelayEnvironment } from 'react-relay';

import { ProgramTitle } from '../futon';
import type {
  RecordingRenamesQuery,
  RecordingRenamesQuery$data,
} from './__generated__/RecordingRenamesQuery.graphql';
import type { RecordingRenamesMutation } from './__generated__/RecordingRenamesMutation.graphql';

const query = graphql`
  query RecordingRenamesQuery {
    recordingRenames {
      recording {
        id
        name
      }
      files {
        from
        to
      }
    }
  }
`;

type Rename = RecordingRenamesQuery$data['recordingRenames'][number];

// renameBatch は、1 回の renameRecordingFiles で送る録画の数 (サーバーの上限)。
const renameBatch = 100;

// RecordingRenames は、ファイル名のテンプレートを変えた後に、既存の録画のファイルを今のテンプレートの名前に付け直す (設計書 4.7)。
// 元の名前と新しい名前の一覧を見せ、確かめてから付け直す。
export function RecordingRenames() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [found, setFound] = useState<readonly Rename[] | null>(null);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [searching, setSearching] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [rename] = useMutation<RecordingRenamesMutation>(graphql`
    mutation RecordingRenamesMutation($ids: [ID!]!) {
      renameRecordingFiles(ids: $ids) {
        renamed
        skipped
        failed
      }
    }
  `);

  // search は、付け直すと名前が変わる録画を探す。付け直した後に探し直すときは、結果の表示を残す (keepMessage)。
  const search = async (keepMessage = false) => {
    setSearching(true);
    if (!keepMessage) {
      setMessage(null);
    }
    try {
      const data = await fetchQuery<RecordingRenamesQuery>(
        environment,
        query,
        {},
        { fetchPolicy: 'network-only' },
      ).toPromise();
      const list = data?.recordingRenames ?? [];
      setFound(list);
      setSelected(new Set(list.map((r) => r.recording.id)));
    } catch {
      setMessage({ ok: false, text: t('admin.rename.searchError') });
    } finally {
      setSearching(false);
    }
  };

  const toggle = (id: string, checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(id);
      } else {
        next.delete(id);
      }

      return next;
    });

  // run は、選んだ録画を、サーバーの上限ごとに分けて付け直し、進み具合を示す。
  const run = async () => {
    const ids = [...selected];
    const total = { renamed: 0, skipped: 0, failed: 0 };
    setConfirming(false);
    setProgress({ done: 0, total: ids.length });
    try {
      for (let start = 0; start < ids.length; start += renameBatch) {
        const batch = ids.slice(start, start + renameBatch);
        const r = await new Promise<{ renamed: number; skipped: number; failed: number }>(
          (resolve, reject) => {
            rename({
              variables: { ids: batch },
              onCompleted: (response, errors) =>
                errors != null && errors.length > 0
                  ? reject(new Error(errors[0]?.message ?? 'GraphQL error'))
                  : resolve(response.renameRecordingFiles),
              onError: reject,
            });
          },
        );
        total.renamed += r.renamed;
        total.skipped += r.skipped;
        total.failed += r.failed;
        setProgress({ done: start + batch.length, total: ids.length });
      }
      setMessage({ ok: total.failed === 0, text: t('admin.rename.done', total) });
    } catch {
      setMessage({ ok: false, text: t('admin.rename.error') });
    } finally {
      setProgress(null);
      void search(true);
    }
  };

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('admin.rename.title')}
          </Title>
          <Button
            size="xs"
            variant="default"
            loading={searching}
            disabled={progress !== null}
            onClick={() => void search()}
          >
            {t('admin.rename.search')}
          </Button>
        </Group>
        <Text size="xs" c="dimmed">
          {t('admin.rename.description')}
        </Text>
        {message !== null && (
          <Alert color={message.ok ? 'teal' : 'red'} role={message.ok ? 'status' : 'alert'}>
            {message.text}
          </Alert>
        )}
        {progress !== null && (
          <Stack gap={4}>
            <Progress value={(progress.done / Math.max(progress.total, 1)) * 100} animated />
            <Text size="xs" c="dimmed">
              {t('admin.rename.progress', progress)}
            </Text>
          </Stack>
        )}
        {found !== null && found.length === 0 && (
          <Text size="sm" c="dimmed">
            {t('admin.rename.none')}
          </Text>
        )}
        {found !== null && found.length > 0 && (
          <>
            <Table fz="xs" verticalSpacing={4}>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th w={40} />
                  <Table.Th>{t('admin.rename.name')}</Table.Th>
                  <Table.Th>{t('admin.rename.files')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {found.map((r) => (
                  <Table.Tr key={r.recording.id}>
                    <Table.Td>
                      <Checkbox
                        aria-label={t('admin.rename.select', { name: r.recording.name })}
                        checked={selected.has(r.recording.id)}
                        disabled={progress !== null}
                        onChange={(event) => toggle(r.recording.id, event.currentTarget.checked)}
                      />
                    </Table.Td>
                    <Table.Td>
                      <ProgramTitle name={r.recording.name} />
                    </Table.Td>
                    <Table.Td>
                      <Stack gap={2}>
                        {r.files.map((file) => (
                          <Text key={file.from} size="xs" style={{ wordBreak: 'break-all' }}>
                            <Code>{file.from}</Code> → <Code>{file.to}</Code>
                          </Text>
                        ))}
                      </Stack>
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
            <Group justify="flex-end">
              <Button
                disabled={selected.size === 0 || progress !== null}
                onClick={() => setConfirming(true)}
              >
                {t('admin.rename.run', { count: selected.size })}
              </Button>
            </Group>
          </>
        )}
      </Stack>
      <Modal
        opened={confirming}
        onClose={() => setConfirming(false)}
        title={t('admin.rename.confirmTitle')}
      >
        <Stack>
          <Text size="sm">{t('admin.rename.confirm', { count: selected.size })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirming(false)}>
              {t('admin.rename.cancel')}
            </Button>
            <Button onClick={() => void run()}>{t('admin.rename.confirmButton')}</Button>
          </Group>
        </Stack>
      </Modal>
    </Card>
  );
}
