import { Button, Card, Group, Modal, Stack, Text, TextInput, Title } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { authApi } from '../auth/api';
import { unsubscribeDevicePush } from '../auth/logout';
import {
  refreshOfflineRecordings,
  removeOfflineRecording,
  savedOfflineRecordings,
} from '../offline/store';
import { mutate } from './mutate';
import type { useReauthGate } from './useReauthGate';
import type { DeleteAccountMutation } from './__generated__/DeleteAccountMutation.graphql';

// finishDeletion は、アカウントを消した後に、この端末に残るものを片付けて、ログインの画面へ移す。
// サーバーの購読はアカウントと一緒に消えているので、ブラウザの購読だけを解除する。
async function finishDeletion(): Promise<void> {
  await unsubscribeDevicePush(() => Promise.resolve());
  await refreshOfflineRecordings().catch(() => {});
  await Promise.allSettled(savedOfflineRecordings().map((meta) => removeOfflineRecording(meta.id)));
  // セッションはもう消えているので、ログアウトに失敗しても、ログインの画面へ移す。
  void authApi.logout().finally(() => window.location.assign(`${import.meta.env.BASE_URL}login`));
}

// DeleteAccount は、自分のアカウントと個人のデータを消すカード (設計書 9.7、#1382)。
// 取り返しがつかないので、ユーザー名を入れてもらってから消す。
export function DeleteAccount({
  username,
  run,
  fail,
}: {
  username: string;
  run: ReturnType<typeof useReauthGate>['run'];
  fail: (error: unknown) => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [confirming, setConfirming] = useState(false);
  const [typed, setTyped] = useState('');
  const close = () => {
    setConfirming(false);
    setTyped('');
  };
  const confirm = () => {
    close();
    run(async () => {
      await mutate<DeleteAccountMutation>(
        environment,
        graphql`
          mutation DeleteAccountMutation {
            deleteAccount
          }
        `,
        {},
      );
      await finishDeletion();
    }, fail);
  };

  return (
    <Card withBorder>
      <Stack gap="xs" align="flex-start">
        <Title order={3} size="h5">
          {t('settings.account.deleteAccount')}
        </Title>
        <Text size="sm" c="dimmed">
          {t('settings.account.deleteAccountHelp')}
        </Text>
        <Button variant="default" color="red" size="xs" onClick={() => setConfirming(true)}>
          {t('settings.account.deleteAccountAction')}
        </Button>
      </Stack>
      {confirming && (
        <Modal opened onClose={close} title={t('settings.account.deleteAccountTitle')}>
          <Stack>
            <Text size="sm">{t('settings.account.deleteAccountHelp')}</Text>
            <TextInput
              label={t('settings.account.deleteAccountConfirm', { username })}
              autoComplete="off"
              value={typed}
              onChange={(event) => setTyped(event.currentTarget.value)}
            />
            <Group justify="flex-end">
              <Button variant="default" onClick={close}>
                {t('reservations.cancel')}
              </Button>
              <Button color="red" disabled={typed !== username} onClick={confirm}>
                {t('settings.account.deleteAccountAction')}
              </Button>
            </Group>
          </Stack>
        </Modal>
      )}
    </Card>
  );
}
