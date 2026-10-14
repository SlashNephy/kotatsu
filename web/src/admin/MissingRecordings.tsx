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

import { ProgramTitle } from '../futon';
import type {
  MissingRecordingsQuery,
  MissingRecordingsQuery$data,
} from './__generated__/MissingRecordingsQuery.graphql';
import type { MissingRecordingsCleanUpMutation } from './__generated__/MissingRecordingsCleanUpMutation.graphql';

const query = graphql`
  query MissingRecordingsQuery {
    missingRecordings {
      recording {
        id
        name
        recordedFrom
        protected
      }
      segments
      missingSegments
    }
  }
`;

type Missing = MissingRecordingsQuery$data['missingRecordings'][number];

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// MissingRecordings は、セグメントのファイルが見つからない録画を探し、確かめてから DB から消す (設計書 5.6)。
// ファイルを Kotatsu を通さずに消すと、録画の一覧に残り続けるので、管理者が片付ける。
export function MissingRecordings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [found, setFound] = useState<readonly Missing[] | null>(null);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [searching, setSearching] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [cleanUp, cleaning] = useMutation<MissingRecordingsCleanUpMutation>(graphql`
    mutation MissingRecordingsCleanUpMutation($ids: [ID!]!) {
      cleanUpMissingRecordings(ids: $ids) {
        deletedRecordings
        prunedSegments
        skipped
      }
    }
  `);

  // search は、ファイルの見つからない録画を探す。消した後に探し直すときは、消した結果の表示を残す (keepMessage)。
  const search = async (keepMessage = false) => {
    setSearching(true);
    if (!keepMessage) {
      setMessage(null);
    }
    try {
      const data = await fetchQuery<MissingRecordingsQuery>(
        environment,
        query,
        {},
        { fetchPolicy: 'network-only' },
      ).toPromise();
      const list = data?.missingRecordings ?? [];
      setFound(list);
      // 保護した録画は消さないので、はじめから選ばない。
      setSelected(new Set(list.filter((m) => !m.recording.protected).map((m) => m.recording.id)));
    } catch {
      setMessage({ ok: false, text: t('admin.missing.searchError') });
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

  const run = () => {
    cleanUp({
      variables: { ids: [...selected] },
      onCompleted: (response, errors) => {
        setConfirming(false);
        if (errors != null && errors.length > 0) {
          setMessage({ ok: false, text: t('admin.missing.cleanUpError') });

          return;
        }
        const r = response.cleanUpMissingRecordings;
        setMessage({
          ok: true,
          text: t('admin.missing.cleaned', {
            deleted: r.deletedRecordings,
            pruned: r.prunedSegments,
            skipped: r.skipped,
          }),
        });
        void search(true);
      },
      onError: () => {
        setConfirming(false);
        setMessage({ ok: false, text: t('admin.missing.cleanUpError') });
      },
    });
  };

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('admin.missing.title')}
          </Title>
          <Button size="xs" variant="default" loading={searching} onClick={() => void search()}>
            {t('admin.missing.search')}
          </Button>
        </Group>
        <Text size="xs" c="dimmed">
          {t('admin.missing.description')}
        </Text>
        {message !== null && (
          <Alert color={message.ok ? 'teal' : 'red'} role={message.ok ? 'status' : 'alert'}>
            {message.text}
          </Alert>
        )}
        {found !== null && found.length === 0 && (
          <Text size="sm" c="dimmed">
            {t('admin.missing.none')}
          </Text>
        )}
        {found !== null && found.length > 0 && (
          <>
            <Table fz="sm" verticalSpacing={4}>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th w={40} />
                  <Table.Th>{t('admin.missing.name')}</Table.Th>
                  <Table.Th>{t('admin.missing.recordedFrom')}</Table.Th>
                  <Table.Th>{t('admin.missing.segments')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {found.map((m) => (
                  <Table.Tr key={m.recording.id}>
                    <Table.Td>
                      <Checkbox
                        aria-label={t('admin.missing.select', { name: m.recording.name })}
                        checked={selected.has(m.recording.id)}
                        disabled={m.recording.protected}
                        onChange={(event) => toggle(m.recording.id, event.currentTarget.checked)}
                      />
                    </Table.Td>
                    <Table.Td>
                      <ProgramTitle name={m.recording.name} />
                      {m.recording.protected && (
                        <Text span size="xs" c="dimmed">
                          {' '}
                          {t('admin.missing.protected')}
                        </Text>
                      )}
                    </Table.Td>
                    <Table.Td>
                      {dateTimeFormat.format(Date.parse(m.recording.recordedFrom))}
                    </Table.Td>
                    <Table.Td>
                      {m.missingSegments === m.segments
                        ? t('admin.missing.allMissing')
                        : t('admin.missing.someMissing', {
                            missing: m.missingSegments,
                            segments: m.segments,
                          })}
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
            <Group justify="flex-end">
              <Button
                color="red"
                disabled={selected.size === 0}
                onClick={() => setConfirming(true)}
              >
                {t('admin.missing.cleanUp', { count: selected.size })}
              </Button>
            </Group>
          </>
        )}
      </Stack>
      <Modal
        opened={confirming}
        onClose={() => setConfirming(false)}
        title={t('admin.missing.confirmTitle')}
      >
        <Stack>
          <Text size="sm">{t('admin.missing.confirm', { count: selected.size })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirming(false)}>
              {t('admin.missing.cancel')}
            </Button>
            <Button color="red" loading={cleaning} onClick={run}>
              {t('admin.missing.confirmButton')}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Card>
  );
}
