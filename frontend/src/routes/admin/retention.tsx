import { Alert, Card, NumberInput, Stack, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import { useSyncedForm } from '../../rules/useSyncedForm';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { useAction } from '../../admin/useAction';
import { SaveRow } from '../../admin/SaveRow';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import type { retentionMutation } from './__generated__/retentionMutation.graphql';
import type { retentionQuery } from './__generated__/retentionQuery.graphql';

export const Route = createFileRoute('/admin/retention')({
  component: RetentionPage,
});

// RetentionPage は、過去の番組と画面内の通知と録画の再生位置を残す日数を変える、管理者向けの画面 (設計書 10.3、11.2)。
function RetentionPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.retention.title')}>
      <RetentionSettings />
    </AdminPage>
  );
}

const query = graphql`
  query retentionQuery {
    systemSettings {
      archiveRetentionDays
      notificationRetentionDays
      watchProgressRetentionDays
      networkActivityRetentionDays
      updatedAt
    }
  }
`;

// Form は、保持期間のフォームの値。空の入力は '' になる。
type Form = {
  archiveDays: number | string;
  notificationDays: number | string;
  watchProgressDays: number | string;
  networkActivityDays: number | string;
};

// sameForm は、2 つのフォームの値が同じかどうかを返す。
function sameForm(a: Form, b: Form): boolean {
  return (Object.keys(a) as (keyof Form)[]).every((key) => a[key] === b[key]);
}

function RetentionSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<retentionQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const action = useAction();
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const settings = data.systemSettings;
  // 触っていないフォームは、読み直した値 (ほかの管理者が保存したものなど) に合わせる。触ったフォームの入力は捨てない (#1452)。
  const initial: Form = {
    archiveDays: settings.archiveRetentionDays,
    notificationDays: settings.notificationRetentionDays,
    watchProgressDays: settings.watchProgressRetentionDays,
    networkActivityDays: settings.networkActivityRetentionDays,
  };
  const { form, set } = useSyncedForm(initial, sameForm);
  const { archiveDays, notificationDays, watchProgressDays, networkActivityDays } = form;
  const valid =
    typeof networkActivityDays === 'number' &&
    typeof archiveDays === 'number' &&
    typeof notificationDays === 'number' &&
    typeof watchProgressDays === 'number';

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setMessage(null);
    action.run(
      async () => {
        // この画面の項目だけを送る。省いた項目は、サーバーが変えない。
        await mutate<retentionMutation>(
          environment,
          graphql`
            mutation retentionMutation($input: SystemSettingsInput!) {
              updateSystemSettings(input: $input) {
                updatedAt
              }
            }
          `,
          {
            input: {
              archiveRetentionDays: Number(archiveDays),
              notificationRetentionDays: Number(notificationDays),
              watchProgressRetentionDays: Number(watchProgressDays),
              networkActivityRetentionDays: Number(networkActivityDays),
            },
          },
        );
        setMessage({ ok: true, text: t('admin.saved') });
        startTransition(() => setFetchKey((key) => key + 1));
      },
      (caught) => setMessage({ ok: false, text: settingsErrorMessage(t, caught) }),
    );
  };

  return (
    <form onSubmit={submit}>
      <Stack gap="md">
        {message !== null && (
          <Alert
            color={message.ok ? 'teal' : 'red'}
            role={message.ok ? 'status' : 'alert'}
            withCloseButton
            onClose={() => setMessage(null)}
          >
            {message.text}
          </Alert>
        )}

        <Card withBorder>
          <Stack gap="xs">
            <Title order={3} size="h5">
              {t('admin.settings.archive')}
            </Title>
            <NumberInput
              size="xs"
              maw={480}
              label={t('admin.settings.archiveRetentionDays')}
              description={t('admin.settings.archiveRetentionHelp')}
              min={0}
              allowDecimal={false}
              value={archiveDays}
              onChange={(value) => set({ archiveDays: value })}
            />
          </Stack>
        </Card>

        <Card withBorder>
          <Stack gap="xs">
            <Title order={3} size="h5">
              {t('admin.settings.notifications')}
            </Title>
            <NumberInput
              size="xs"
              maw={480}
              label={t('admin.settings.notificationRetentionDays')}
              description={t('admin.settings.notificationRetentionHelp')}
              min={0}
              allowDecimal={false}
              value={notificationDays}
              onChange={(value) => set({ notificationDays: value })}
            />
          </Stack>
        </Card>

        <Card withBorder>
          <Stack gap="xs">
            <Title order={3} size="h5">
              {t('admin.settings.watchProgress')}
            </Title>
            <NumberInput
              size="xs"
              maw={480}
              label={t('admin.settings.watchProgressRetentionDays')}
              description={t('admin.settings.watchProgressRetentionHelp')}
              min={0}
              allowDecimal={false}
              value={watchProgressDays}
              onChange={(value) => set({ watchProgressDays: value })}
            />
          </Stack>
        </Card>

        <Card withBorder>
          <Stack gap="xs">
            <Title order={3} size="h5">
              {t('admin.settings.networkActivity')}
            </Title>
            <NumberInput
              size="xs"
              maw={480}
              label={t('admin.settings.networkActivityRetentionDays')}
              description={t('admin.settings.networkActivityRetentionHelp')}
              min={1}
              allowDecimal={false}
              value={networkActivityDays}
              onChange={(value) => set({ networkActivityDays: value })}
            />
          </Stack>
        </Card>

        <SaveRow updatedAt={settings.updatedAt} disabled={!valid || action.busy} />
      </Stack>
    </form>
  );
}
