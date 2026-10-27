import { Alert, Card, Group, NumberInput, Select, Stack, Switch, Text, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { SaveRow } from '../../admin/SaveRow';
import { authApi, type OIDCStatus } from '../../auth/api';
import { settingsErrorMessage } from '../../settings/errors';
import { FileNameTemplateField } from '../../reservations/FileNameTemplateField';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { SudoGate } from '../../settings/SudoGate';
import type { recordingMutation } from './__generated__/recordingMutation.graphql';
import type { recordingQuery } from './__generated__/recordingQuery.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/recording';

export const Route = createFileRoute('/admin/recording')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: RecordingPage,
});

// autoPool は、保存先プールのメニューで「自動で選ぶ」を表す値。保存するときも空文字のまま送る。
// API では、null は「変えない」で、空文字が「自動で選ぶのに戻す」なので。
const autoPool = '';

// RecordingPage は、録画設定の既定値と、ルールを作るときの既定値を変える、管理者向けの画面 (設計書 4.7、10.3、11.2)。
function RecordingPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.recording.title')}>
      <RecordingSettings />
    </AdminPage>
  );
}

const query = graphql`
  query recordingQuery {
    systemSettings {
      recordingDefaults {
        poolId
        fileNameTemplate
        startMarginSeconds
        endMarginSeconds
        priority
        joinSegments
      }
      ruleDefaults {
        avoidDuplicates
        duplicateWindowDays
        followEventRelay
      }
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
  // ルールを作るときの既定値 (#470)。重複を避ける日数は、空なら期間を区切らない。
  ruleAvoidDuplicates: boolean;
  ruleDuplicateWindowDays: number | string;
  ruleFollowEventRelay: boolean;
};

function RecordingSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<recordingQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const methods = reauthMethods(data.secondFactors, data.passkeys, oidc);
  const gate = useReauthGate(methods, path);
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
    ruleAvoidDuplicates: settings.ruleDefaults.avoidDuplicates,
    ruleDuplicateWindowDays: settings.ruleDefaults.duplicateWindowDays ?? '',
    ruleFollowEventRelay: settings.ruleDefaults.followEventRelay,
  });
  const set = (patch: Partial<Form>) => setForm((current) => ({ ...current, ...patch }));
  const numbers = [form.startMarginSeconds, form.endMarginSeconds, form.priority];
  const valid = form.fileNameTemplate.trim() !== '' && numbers.every((n) => typeof n === 'number');

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setMessage(null);
    gate.run(
      async () => {
        // この画面の項目だけを送る。省いた項目 (保持期間や MCP) は、サーバーが変えない。
        await mutate<recordingMutation>(
          environment,
          graphql`
            mutation recordingMutation($input: SystemSettingsInput!) {
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
              ruleDefaults: {
                avoidDuplicates: form.ruleAvoidDuplicates,
                ...(typeof form.ruleDuplicateWindowDays === 'number'
                  ? { duplicateWindowDays: form.ruleDuplicateWindowDays }
                  : { clearDuplicateWindowDays: true }),
                followEventRelay: form.ruleFollowEventRelay,
              },
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
      <SudoGate methods={methods} returnTo={path}>
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
                  {t('admin.settings.ruleDefaults')}
                </Title>
                <Text size="xs" c="dimmed">
                  {t('admin.settings.ruleDefaultsNote')}
                </Text>
                <Group gap="xs" align="flex-end">
                  <Switch
                    size="sm"
                    label={t('rules.avoidDuplicates')}
                    checked={form.ruleAvoidDuplicates}
                    onChange={(event) => set({ ruleAvoidDuplicates: event.currentTarget.checked })}
                  />
                  {form.ruleAvoidDuplicates && (
                    <NumberInput
                      size="xs"
                      w={160}
                      aria-label={t('rules.duplicateWindowDays')}
                      placeholder={t('rules.duplicateWindowDaysPlaceholder')}
                      min={1}
                      max={3650}
                      allowDecimal={false}
                      suffix={t('rules.days')}
                      value={form.ruleDuplicateWindowDays}
                      onChange={(value) => set({ ruleDuplicateWindowDays: value })}
                    />
                  )}
                </Group>
                <Switch
                  size="sm"
                  label={t('rules.followEventRelay')}
                  checked={form.ruleFollowEventRelay}
                  onChange={(event) => set({ ruleFollowEventRelay: event.currentTarget.checked })}
                />
              </Stack>
            </Card>

            <SaveRow updatedAt={settings.updatedAt} disabled={!valid || gate.busy} />
          </Stack>
        </form>
      </SudoGate>
    </>
  );
}
