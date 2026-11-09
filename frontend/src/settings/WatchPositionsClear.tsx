import { Button, Card, Group, Modal, Stack, Text, Title } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { WatchPositionsClearMutation } from './__generated__/WatchPositionsClearMutation.graphql';

// WatchPositionsClear は、自分の録画の再生位置をすべて消すカード (#1366)。
export function WatchPositionsClear() {
  const { t } = useTranslation();
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [commit, inFlight] = useMutation<WatchPositionsClearMutation>(graphql`
    mutation WatchPositionsClearMutation {
      clearAllWatchPositions
    }
  `);

  const clear = () => {
    commit({
      variables: {},
      onCompleted: (response, errors) => {
        setConfirming(false);
        setMessage(
          errors != null && errors.length > 0
            ? t('settings.account.watchPositionsClearFailed')
            : t('settings.account.watchPositionsCleared', {
                count: response.clearAllWatchPositions,
              }),
        );
      },
      onError: () => {
        setConfirming(false);
        setMessage(t('settings.account.watchPositionsClearFailed'));
      },
    });
  };

  return (
    <Card withBorder>
      <Stack gap="xs" align="flex-start">
        <Title order={3} size="h5">
          {t('settings.account.watchPositions')}
        </Title>
        <Text size="sm" c="dimmed">
          {t('settings.account.watchPositionsHelp')}
        </Text>
        <Group gap="sm">
          <Button variant="default" color="red" size="xs" onClick={() => setConfirming(true)}>
            {t('settings.account.watchPositionsClear')}
          </Button>
          {message !== null && (
            <Text size="sm" c="dimmed" role="status">
              {message}
            </Text>
          )}
        </Group>
      </Stack>
      {confirming && (
        <Modal
          opened
          onClose={() => setConfirming(false)}
          title={t('settings.account.watchPositionsClearTitle')}
        >
          <Stack>
            <Text size="sm">{t('settings.account.watchPositionsHelp')}</Text>
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setConfirming(false)}>
                {t('reservations.cancel')}
              </Button>
              <Button color="red" loading={inFlight} onClick={clear}>
                {t('settings.account.watchPositionsClear')}
              </Button>
            </Group>
          </Stack>
        </Modal>
      )}
    </Card>
  );
}
