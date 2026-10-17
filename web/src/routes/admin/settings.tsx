import {
  Alert,
  Button,
  Card,
  Group,
  NumberInput,
  Select,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { AdminPage } from '../../admin/AdminPage';
import { authApi, type OIDCStatus } from '../../auth/api';
import { settingsErrorMessage } from '../../settings/errors';
import { FileNameTemplateField } from '../../reservations/FileNameTemplateField';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import type { settingsSystemMutation } from './__generated__/settingsSystemMutation.graphql';
import type { settingsSystemQuery } from './__generated__/settingsSystemQuery.graphql';
import { zonedFormat } from '../../futon';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/settings';

export const Route = createFileRoute('/admin/settings')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: SettingsPage,
});

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// autoPool は、保存先プールのメニューで「自動で選ぶ」を表す値。保存するときも空文字のまま送る。
// API では、null は「変えない」で、空文字が「自動で選ぶのに戻す」なので。
const autoPool = '';

// SettingsPage は、録画設定の既定値と、過去の番組の保持期間を変える、管理者向けの画面 (設計書 4.7、11.2)。
function SettingsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.settings.title')}>
      <SystemSettings />
    </AdminPage>
  );
}

const query = graphql`
  query settingsSystemQuery {
    systemSettings {
      recordingDefaults {
        poolId
        fileNameTemplate
        startMarginSeconds
        endMarginSeconds
        priority
        joinSegments
      }
      archiveRetentionDays
      notificationRetentionDays
      updatedAt
    }
    systemStatus {
      pools {
        id
      }
    }
    secondFactors {
      totp
      remainingRecoveryCodes
    }
    passkeys {
      id
    }
  }
`;

type Form = {
  poolId: string;
  fileNameTemplate: string;
  startMarginSeconds: number | string;
  endMarginSeconds: number | string;
  priority: number | string;
  joinSegments: boolean;
  archiveRetentionDays: number | string;
  notificationRetentionDays: number | string;
};

function SystemSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<settingsSystemQuery>(
    query,
    {},
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const gate = useReauthGate(reauthMethods(data.secondFactors, data.passkeys, oidc), path);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const settings = data.systemSettings;
  const defaults = settings.recordingDefaults;
  const [form, setForm] = useState<Form>({
    poolId: defaults.poolId ?? autoPool,
    fileNameTemplate: defaults.fileNameTemplate,
    startMarginSeconds: defaults.startMarginSeconds,
    endMarginSeconds: defaults.endMarginSeconds,
    priority: defaults.priority,
    joinSegments: defaults.joinSegments,
    archiveRetentionDays: settings.archiveRetentionDays,
    notificationRetentionDays: settings.notificationRetentionDays,
  });
  const set = (patch: Partial<Form>) => setForm((current) => ({ ...current, ...patch }));
  const numbers = [
    form.startMarginSeconds,
    form.endMarginSeconds,
    form.priority,
    form.archiveRetentionDays,
    form.notificationRetentionDays,
  ];
  const valid = form.fileNameTemplate.trim() !== '' && numbers.every((n) => typeof n === 'number');

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setMessage(null);
    gate.run(
      async () => {
        await mutate<settingsSystemMutation>(
          environment,
          graphql`
            mutation settingsSystemMutation($input: SystemSettingsInput!) {
              updateSystemSettings(input: $input) {
                updatedAt
              }
            }
          `,
          {
            input: {
              recordingDefaults: {
                poolId: form.poolId,
                fileNameTemplate: form.fileNameTemplate.trim(),
                startMarginSeconds: Number(form.startMarginSeconds),
                endMarginSeconds: Number(form.endMarginSeconds),
                priority: Number(form.priority),
                joinSegments: form.joinSegments,
              },
              archiveRetentionDays: Number(form.archiveRetentionDays),
              notificationRetentionDays: Number(form.notificationRetentionDays),
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
    // 再認証のモーダルは、form の外に置く。ポータルの中の送信も、React では外の form に伝わるので。
    <>
      {gate.modal}
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
                {t('admin.settings.recordingDefaults')}
              </Title>
              <Text size="xs" c="dimmed">
                {t('admin.settings.recordingDefaultsNote')}
              </Text>
              <Select
                size="xs"
                maw={480}
                label={t('admin.settings.pool')}
                description={t('admin.settings.poolHelp')}
                allowDeselect={false}
                data={[
                  { value: autoPool, label: t('admin.settings.poolAuto') },
                  ...data.systemStatus.pools.map((pool) => ({ value: pool.id, label: pool.id })),
                ]}
                value={form.poolId}
                onChange={(value) => set({ poolId: value ?? autoPool })}
              />
              <FileNameTemplateField
                label={t('admin.settings.fileNameTemplate')}
                description={t('admin.settings.fileNameTemplateHelp')}
                value={form.fileNameTemplate}
                onChange={(value) => set({ fileNameTemplate: value })}
              />
              <Group gap="xs" align="flex-start">
                <NumberInput
                  size="xs"
                  w={180}
                  label={t('admin.settings.startMargin')}
                  min={0}
                  max={3600}
                  allowDecimal={false}
                  value={form.startMarginSeconds}
                  onChange={(value) => set({ startMarginSeconds: value })}
                />
                <NumberInput
                  size="xs"
                  w={180}
                  label={t('admin.settings.endMargin')}
                  min={0}
                  max={3600}
                  allowDecimal={false}
                  value={form.endMarginSeconds}
                  onChange={(value) => set({ endMarginSeconds: value })}
                />
              </Group>
              <NumberInput
                size="xs"
                maw={480}
                label={t('admin.settings.priority')}
                description={t('admin.settings.priorityHelp')}
                min={-10}
                max={10}
                allowDecimal={false}
                value={form.priority}
                onChange={(value) => set({ priority: value })}
              />
              <Switch
                size="sm"
                label={t('admin.settings.joinSegments')}
                description={t('admin.settings.joinSegmentsHelp')}
                checked={form.joinSegments}
                onChange={(event) => set({ joinSegments: event.currentTarget.checked })}
              />
            </Stack>
          </Card>

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
                value={form.archiveRetentionDays}
                onChange={(value) => set({ archiveRetentionDays: value })}
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
                value={form.notificationRetentionDays}
                onChange={(value) => set({ notificationRetentionDays: value })}
              />
            </Stack>
          </Card>

          <Group justify="space-between">
            <Text size="xs" c="dimmed">
              {settings.updatedAt == null
                ? t('admin.settings.neverUpdated')
                : t('admin.settings.updatedAt', {
                    at: dateTimeFormat.format(Date.parse(settings.updatedAt)),
                  })}
            </Text>
            <Button type="submit" size="compact-sm" disabled={!valid || gate.busy}>
              {t('admin.save')}
            </Button>
          </Group>
        </Stack>
      </form>
    </>
  );
}
