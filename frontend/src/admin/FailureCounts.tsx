import { Alert, Button, Group, Modal, Stack, Table, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { failureKey } from '../recordings/failureReason';
import type {
  FailureCountsResetMutation,
  RecordingFailureReason,
} from './__generated__/FailureCountsResetMutation.graphql';

export type FailureCount = {
  reason: RecordingFailureReason;
  // Int64 は Relay の型では any になるので、受け取ってから数に直す。
  count: unknown;
  lastFailedAt?: string | null;
  resetAt?: string | null;
};

type Props = {
  counts: readonly FailureCount[];
  formatTime: (time: string) => string;
  // onReset は、累計を 0 に戻した後に呼ぶ。画面を読み直すのに使う。
  onReset: () => void;
};

// FailureCounts は、理由ごとの、録画の失敗の累計と、0 に戻すボタン (設計書 5.5、#1035)。
// 検証で失敗させた分や、直した障害の分を消すのに使う。戻す前に確かめ、戻したことは監査ログに残る。
export function FailureCounts({ counts, formatTime, onReset }: Props) {
  const { t } = useTranslation();
  const [confirming, setConfirming] = useState<RecordingFailureReason | null>(null);
  const [error, setError] = useState(false);
  const [reset, resetting] = useMutation<FailureCountsResetMutation>(graphql`
    mutation FailureCountsResetMutation($reasons: [RecordingFailureReason!]!) {
      resetRecordingFailureCounts(reasons: $reasons) {
        reason
        count
        lastFailedAt
        resetAt
      }
    }
  `);
  const confirm = () => {
    if (confirming === null) {
      return;
    }
    setError(false);
    reset({
      variables: { reasons: [confirming] },
      onCompleted: () => {
        setConfirming(null);
        onReset();
      },
      onError: () => {
        setConfirming(null);
        setError(true);
      },
    });
  };

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t('admin.system.failures.description')}
      </Text>
      {error && (
        <Alert color="red" role="alert">
          {t('admin.system.failures.resetError')}
        </Alert>
      )}
      <Table.ScrollContainer minWidth={560}>
        <Table
          fz="sm"
          verticalSpacing={4}
          styles={{ td: { whiteSpace: 'nowrap' }, th: { whiteSpace: 'nowrap' } }}
        >
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t('admin.system.failures.reason')}</Table.Th>
              <Table.Th>{t('admin.system.failures.count')}</Table.Th>
              <Table.Th>{t('admin.system.failures.lastFailedAt')}</Table.Th>
              <Table.Th>{t('admin.system.failures.resetAt')}</Table.Th>
              <Table.Th />
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {counts.map((row) => (
              <Table.Tr key={row.reason}>
                <Table.Td>{t(failureKey(row.reason))}</Table.Td>
                <Table.Td>{Number(row.count)}</Table.Td>
                <Table.Td>{row.lastFailedAt == null ? '-' : formatTime(row.lastFailedAt)}</Table.Td>
                <Table.Td>{row.resetAt == null ? '-' : formatTime(row.resetAt)}</Table.Td>
                <Table.Td>
                  <Button
                    size="compact-xs"
                    variant="default"
                    // 戻すものがなければ、押せないようにする。
                    disabled={Number(row.count) === 0 && row.lastFailedAt == null}
                    onClick={() => setConfirming(row.reason)}
                  >
                    {t('admin.system.failures.reset')}
                  </Button>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      <Modal
        opened={confirming !== null}
        onClose={() => setConfirming(null)}
        title={t('admin.system.failures.confirmTitle')}
      >
        <Stack>
          <Text size="sm">
            {t('admin.system.failures.confirm', {
              reason: confirming === null ? '' : t(failureKey(confirming)),
            })}
          </Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirming(null)}>
              {t('admin.system.failures.cancel')}
            </Button>
            <Button color="red" loading={resetting} onClick={confirm}>
              {t('admin.system.failures.reset')}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  );
}
