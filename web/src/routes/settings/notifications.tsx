import { Alert, Anchor, Card, Group, Stack, Switch, Table, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { currentSubscription, pushSupported, subscribe } from '../../notifications/push';
import { isUnauthenticated } from '../../relay/environment';
import type { notificationsPreferenceMutation } from './__generated__/notificationsPreferenceMutation.graphql';
import type { notificationsSettingsQuery } from './__generated__/notificationsSettingsQuery.graphql';
import type { notificationsSubscribeMutation } from './__generated__/notificationsSubscribeMutation.graphql';
import type { notificationsUnsubscribeMutation } from './__generated__/notificationsUnsubscribeMutation.graphql';

export const Route = createFileRoute('/settings/notifications')({
  component: NotificationSettingsPage,
});

// NotificationSettingsPage は、通知の受け取り方の設定 (設計書 6.2)。
function NotificationSettingsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/notifications" size="sm">
        {t('notifications.back')}
      </Anchor>
      <Title order={2} size="h4">
        {t('notifications.settingsTitle')}
      </Title>
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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Settings />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

const kindKeys = {
  RECORDING_STARTED: 'notifications.kind.RECORDING_STARTED',
  RECORDING_COMPLETED: 'notifications.kind.RECORDING_COMPLETED',
  RECORDING_FAILED: 'notifications.kind.RECORDING_FAILED',
  RESERVATION_CONFLICT: 'notifications.kind.RESERVATION_CONFLICT',
  STORAGE_LOW: 'notifications.kind.STORAGE_LOW',
  TUNER_SERVER_UNREACHABLE: 'notifications.kind.TUNER_SERVER_UNREACHABLE',
} as const;

function Settings() {
  const { t } = useTranslation();
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<notificationsSettingsQuery>(
    graphql`
      query notificationsSettingsQuery {
        webPushPublicKey
        webPushSubscriptions {
          endpoint
        }
        notificationPreferences {
          kind
          inApp
          webPush
        }
      }
    `,
    {},
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const [updatePreference] = useMutation<notificationsPreferenceMutation>(graphql`
    mutation notificationsPreferenceMutation($input: NotificationPreferenceInput!) {
      updateNotificationPreference(input: $input) {
        kind
        inApp
        webPush
      }
    }
  `);
  const [subscribePush] = useMutation<notificationsSubscribeMutation>(graphql`
    mutation notificationsSubscribeMutation($input: WebPushSubscriptionInput!) {
      subscribeWebPush(input: $input) {
        endpoint
      }
    }
  `);
  const [unsubscribePush] = useMutation<notificationsUnsubscribeMutation>(graphql`
    mutation notificationsUnsubscribeMutation($endpoint: String!) {
      unsubscribeWebPush(endpoint: $endpoint)
    }
  `);
  // endpoint は、この端末の購読の送り先。購読していなければ null、まだ調べていなければ undefined。
  const [endpoint, setEndpoint] = useState<string | null | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const supported = pushSupported();
  const refresh = () => setFetchKey((key) => key + 1);

  useEffect(() => {
    if (!supported) {
      return;
    }
    void currentSubscription().then(
      (subscription) => setEndpoint(subscription?.endpoint ?? null),
      () => setEndpoint(null),
    );
  }, [supported]);
  // この端末で購読していて、サーバーにも登録してあれば、受け取れる。
  const subscribed =
    endpoint != null &&
    data.webPushSubscriptions.some((subscription) => subscription.endpoint === endpoint);

  const toggle = async (next: boolean) => {
    setBusy(true);
    setError(null);
    try {
      if (next) {
        const keys = await subscribe(data.webPushPublicKey);
        await new Promise<void>((resolve, reject) =>
          subscribePush({
            variables: { input: keys },
            onCompleted: () => resolve(),
            onError: reject,
          }),
        );
        setEndpoint(keys.endpoint);
      } else {
        const subscription = await currentSubscription();
        if (subscription !== null) {
          await new Promise<void>((resolve, reject) =>
            unsubscribePush({
              variables: { endpoint: subscription.endpoint },
              onCompleted: () => resolve(),
              onError: reject,
            }),
          );
          await subscription.unsubscribe();
        }
        setEndpoint(null);
      }
      refresh();
    } catch {
      setError(next ? t('notifications.subscribeError') : t('notifications.unsubscribeError'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Stack gap="md">
      <Card withBorder>
        <Stack gap="xs">
          <Text fw={600}>{t('notifications.webPush')}</Text>
          {supported ? (
            <Switch
              label={t('notifications.thisDevice')}
              checked={subscribed}
              disabled={busy || endpoint === undefined}
              onChange={(event) => void toggle(event.currentTarget.checked)}
            />
          ) : (
            <Text size="sm" c="dimmed">
              {t('notifications.pushUnsupported')}
            </Text>
          )}
          {error !== null && (
            <Alert color="red" role="alert">
              {error}
            </Alert>
          )}
        </Stack>
      </Card>
      <Table>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('notifications.kindColumn')}</Table.Th>
            <Table.Th>{t('notifications.inApp')}</Table.Th>
            <Table.Th>{t('notifications.webPush')}</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {data.notificationPreferences.map((preference) => {
            const label = kindKeys[preference.kind as keyof typeof kindKeys];
            if (label === undefined) {
              return null;
            }
            const set = (input: { inApp: boolean; webPush: boolean }) =>
              updatePreference({
                variables: { input: { kind: preference.kind, ...input } },
                optimisticResponse: {
                  updateNotificationPreference: { kind: preference.kind, ...input },
                },
                onError: () => setError(t('notifications.preferenceError')),
              });

            return (
              <Table.Tr key={preference.kind}>
                <Table.Td>
                  <Text size="sm">{t(label)}</Text>
                </Table.Td>
                <Table.Td>
                  <Switch
                    aria-label={`${t(label)} ${t('notifications.inApp')}`}
                    checked={preference.inApp}
                    onChange={(event) =>
                      set({ inApp: event.currentTarget.checked, webPush: preference.webPush })
                    }
                  />
                </Table.Td>
                <Table.Td>
                  <Switch
                    aria-label={`${t(label)} ${t('notifications.webPush')}`}
                    checked={preference.webPush}
                    onChange={(event) =>
                      set({ inApp: preference.inApp, webPush: event.currentTarget.checked })
                    }
                  />
                </Table.Td>
              </Table.Tr>
            );
          })}
        </Table.Tbody>
      </Table>
      <Group>
        <Text size="xs" c="dimmed">
          {t('notifications.preferenceNote')}
        </Text>
      </Group>
    </Stack>
  );
}
