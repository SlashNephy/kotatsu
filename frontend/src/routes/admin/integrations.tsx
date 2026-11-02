import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Code,
  CopyButton,
  Group,
  PasswordInput,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { Webhook } from 'lucide-react';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type FormEvent, type ReactNode } from 'react';
import { useDataChanges } from '../../changes/DataChanges';
import { EmptyState, zonedFormat } from '../../futon';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { AdminPage } from '../../admin/AdminPage';
import { Section } from '../../admin/Section';
import { authApi, type OIDCStatus } from '../../auth/api';
import {
  notificationKinds,
  personalKinds,
  type NotificationKindName,
} from '../../notifications/kinds';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { SudoGate } from '../../settings/SudoGate';
import type {
  WebhookFormat,
  integrationsWebhookCreateMutation,
} from './__generated__/integrationsWebhookCreateMutation.graphql';
import type { integrationsWebhookDeleteMutation } from './__generated__/integrationsWebhookDeleteMutation.graphql';
import type {
  SystemSettingsInput,
  integrationsSystemSettingsMutation,
} from './__generated__/integrationsSystemSettingsMutation.graphql';
import type {
  AnimeRecordAppInput,
  AnimeRecordDestination,
  integrationsAnimeRecordAppMutation,
} from './__generated__/integrationsAnimeRecordAppMutation.graphql';
import type { integrationsAnimeImportTokenMutation } from './__generated__/integrationsAnimeImportTokenMutation.graphql';
import type { AnimeImportSource } from './__generated__/integrationsQuery.graphql';
import type { integrationsQuery } from './__generated__/integrationsQuery.graphql';
import type { integrationsWebhookUpdateMutation } from './__generated__/integrationsWebhookUpdateMutation.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/integrations';

export const Route = createFileRoute('/admin/integrations')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: IntegrationsPage,
});

const formats = ['JSON', 'DISCORD', 'SLACK'] as const satisfies readonly WebhookFormat[];
// 視聴予約と見たアニメの記録の通知は個人に届けるものなので、全体向けの Webhook では送らない (設計書 4.8、6.2)。
const kinds = (Object.keys(notificationKinds) as NotificationKindName[]).filter(
  (kind) => !personalKinds.includes(kind),
);

// IntegrationsPage は、外部のサービスとの連携 (MCP のクライアント、データ放送の通信の中継、しょぼいカレンダー、Annict、AniList、
// 通知を送る Webhook) を変える、管理者向けの画面 (設計書 3.8、6.2、7.4、10.3)。
function IntegrationsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.integrations.title')}>
      <Integrations />
    </AdminPage>
  );
}

const query = graphql`
  query integrationsQuery {
    systemSettings {
      mcpEnabled
      dataBroadcastIpEnabled
      animeLinkingEnabled
    }
    animeRecordApps {
      destination
      clientId
      configured
      redirectUri
    }
    animeImportToken {
      configured
    }
    animeImports {
      source
      checkedAt
      importedAt
      itemCount
      error
    }
    webhooks {
      id
      name
      url
      format
      kinds
      enabled
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

type WebhookInput = {
  name: string;
  url: string;
  format: WebhookFormat;
  kinds: NotificationKindName[];
  enabled: boolean;
};

type Message = { ok: boolean; text: string };

function Integrations() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<integrationsQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const methods = reauthMethods(data.secondFactors, data.passkeys, oidc);
  const gate = useReauthGate(methods, path);
  const [message, setMessage] = useState<Message | null>(null);
  // secret は、作った Webhook の署名用の秘密キー。作ったときにしか返らないので、ここで出す。
  const [secret, setSecret] = useState<{ name: string; secret: string } | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const succeed = (text: string) => {
    setMessage({ ok: true, text });
    refresh();
  };
  // saveSettings は、全体の設定のうち、この画面で変える項目を保存する。どれも再認証を求める (設計書 9.5)。
  const saveSettings = (input: SystemSettingsInput) =>
    gate.run(async () => {
      await mutate<integrationsSystemSettingsMutation>(
        environment,
        graphql`
          mutation integrationsSystemSettingsMutation($input: SystemSettingsInput!) {
            updateSystemSettings(input: $input) {
              updatedAt
            }
          }
        `,
        { input },
      );
      succeed(t('admin.saved'));
    }, fail);

  return (
    <Stack gap="md">
      {gate.modal}
      <SudoGate methods={methods} returnTo={path}>
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
        {secret !== null && (
          <Alert
            color="amber"
            title={`${t('admin.webhooks.secret')} (${secret.name})`}
            withCloseButton
            onClose={() => setSecret(null)}
          >
            <Text size="sm">{t('admin.webhooks.secretNote')}</Text>
            <Code block mt="xs">
              {secret.secret}
            </Code>
          </Alert>
        )}

        <SwitchSetting
          key={`mcp/${String(data.systemSettings.mcpEnabled)}`}
          title={t('admin.settings.mcp')}
          label={t('admin.settings.mcpEnabled')}
          help={t('admin.settings.mcpHelp')}
          enabled={data.systemSettings.mcpEnabled}
          busy={gate.busy}
          onSave={(mcpEnabled) => saveSettings({ mcpEnabled })}
        />

        {/* データ放送の文書の双方向の通信を、tenban に中継させるか (#501)。受信機の情報が外部へ届くので、ここで再認証を求める。 */}
        <SwitchSetting
          key={`data-broadcast/${String(data.systemSettings.dataBroadcastIpEnabled)}`}
          title={t('admin.settings.dataBroadcastIp')}
          label={t('admin.settings.dataBroadcastIpEnabled')}
          help={t('admin.settings.dataBroadcastIpHelp')}
          enabled={data.systemSettings.dataBroadcastIpEnabled}
          busy={gate.busy}
          onSave={(dataBroadcastIpEnabled) => saveSettings({ dataBroadcastIpEnabled })}
        />

        <AnimeLinkingSettings
          key={`anime/${String(data.systemSettings.animeLinkingEnabled)}`}
          enabled={data.systemSettings.animeLinkingEnabled}
          imports={data.animeImports}
          busy={gate.busy}
          onSave={(animeLinkingEnabled) => saveSettings({ animeLinkingEnabled })}
        >
          {/* 作品 ID の対応を作るための Annict のトークン (#1223)。サーバーが外部へ問い合わせるときの秘密情報なので、ここで再認証を求める。 */}
          <AnimeImportTokenForm
            key={String(data.animeImportToken.configured)}
            configured={data.animeImportToken.configured}
            busy={gate.busy}
            onSave={(annictToken, onDone) =>
              gate.run(async () => {
                await mutate<integrationsAnimeImportTokenMutation>(
                  environment,
                  graphql`
                    mutation integrationsAnimeImportTokenMutation($annictToken: String!) {
                      updateAnimeImportToken(annictToken: $annictToken) {
                        configured
                      }
                    }
                  `,
                  { annictToken },
                );
                onDone();
                succeed(t('admin.saved'));
              }, fail)
            }
          />
        </AnimeLinkingSettings>

        {/* 見たアニメを記録させるアプリ (#927)。利用者を記録先へ送り出すアプリを決めるので、ここで再認証を求める。 */}
        <Section title={t('admin.animeRecords.title')}>
          <Stack gap="md">
            <Text size="sm" c="dimmed">
              {t('admin.animeRecords.help')}
            </Text>
            {data.animeRecordApps.map((app) => (
              <AnimeRecordAppForm
                key={`${app.destination}/${app.clientId ?? ''}/${String(app.configured)}`}
                app={app}
                busy={gate.busy}
                onSave={(input, onDone) =>
                  gate.run(async () => {
                    await mutate<integrationsAnimeRecordAppMutation>(
                      environment,
                      graphql`
                        mutation integrationsAnimeRecordAppMutation(
                          $destination: AnimeRecordDestination!
                          $input: AnimeRecordAppInput!
                        ) {
                          updateAnimeRecordApp(destination: $destination, input: $input) {
                            destination
                            clientId
                            configured
                          }
                        }
                      `,
                      { destination: app.destination, input },
                    );
                    onDone();
                    succeed(t('admin.saved'));
                  }, fail)
                }
              />
            ))}
          </Stack>
        </Section>

        {/* 空のときは、空の表示だけを出す。カードで囲むと、枠が二重になる。 */}
        {data.webhooks.length === 0 ? (
          <EmptyState
            icon={Webhook}
            title={t('admin.webhooks.empty')}
            hint={t('admin.webhooks.emptyHint')}
          />
        ) : (
          <Section title={t('admin.webhooks.title')}>
            <Stack gap="sm">
              {data.webhooks.map((webhook) => (
                <WebhookRow
                  key={`${webhook.id}/${webhook.name}/${webhook.url}/${webhook.format}/${webhook.kinds.join()}/${String(webhook.enabled)}`}
                  webhook={webhook}
                  onUpdate={(input, onDone) =>
                    gate.run(async () => {
                      await mutate<integrationsWebhookUpdateMutation>(
                        environment,
                        graphql`
                          mutation integrationsWebhookUpdateMutation(
                            $id: ID!
                            $input: UpdateWebhookInput!
                          ) {
                            updateWebhook(id: $id, input: $input) {
                              id
                              name
                              url
                              format
                              kinds
                              enabled
                            }
                          }
                        `,
                        { id: webhook.id, input },
                      );
                      onDone();
                      succeed(t('admin.saved'));
                    }, fail)
                  }
                  onDelete={() =>
                    gate.run(async () => {
                      await mutate<integrationsWebhookDeleteMutation>(
                        environment,
                        graphql`
                          mutation integrationsWebhookDeleteMutation($id: ID!) {
                            deleteWebhook(id: $id)
                          }
                        `,
                        { id: webhook.id },
                      );
                      succeed(t('admin.webhooks.deleted', { name: webhook.name }));
                    }, fail)
                  }
                />
              ))}
            </Stack>
          </Section>
        )}

        <Card withBorder>
          <Stack gap="xs">
            <Title order={3} size="h5">
              {t('admin.webhooks.create')}
            </Title>
            <WebhookForm
              submitLabel={t('admin.webhooks.create')}
              onSubmit={(input, onDone) =>
                gate.run(async () => {
                  const response = await mutate<integrationsWebhookCreateMutation>(
                    environment,
                    graphql`
                      mutation integrationsWebhookCreateMutation($input: CreateWebhookInput!) {
                        createWebhook(input: $input) {
                          webhook {
                            id
                            name
                          }
                          secret
                        }
                      }
                    `,
                    { input },
                  );
                  onDone();
                  setSecret({
                    name: response.createWebhook.webhook.name,
                    secret: response.createWebhook.secret,
                  });
                  succeed(
                    t('admin.webhooks.created', { name: response.createWebhook.webhook.name }),
                  );
                }, fail)
              }
            />
          </Stack>
        </Card>
      </SudoGate>
    </Stack>
  );
}

// SwitchSetting は、全体の設定のうち、入れるか切るかの項目を変える。MCP のクライアントに使わせるか (#965) と、
// データ放送の通信を中継するか (#501) に使う。
function SwitchSetting({
  title,
  label,
  help,
  enabled,
  busy,
  onSave,
}: {
  title: string;
  label: string;
  help: string;
  enabled: boolean;
  busy: boolean;
  onSave: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  const [checked, setChecked] = useState(enabled);

  return (
    <Section title={title}>
      <Group justify="space-between" align="flex-end">
        <Switch
          size="sm"
          label={label}
          description={help}
          checked={checked}
          onChange={(event) => setChecked(event.currentTarget.checked)}
          maw={560}
        />
        <Button
          size="compact-sm"
          disabled={checked === enabled || busy}
          onClick={() => onSave(checked)}
        >
          {t('admin.save')}
        </Button>
      </Group>
    </Section>
  );
}

const importedAtFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// importSourceKeys は、取り込み元の名前の翻訳のキー。
const importSourceKeys = {
  SAYA_DEFINITIONS: 'admin.settings.animeSourceSaya',
  ARM: 'admin.settings.animeSourceArm',
  SYOBOCAL_TITLES: 'admin.settings.animeSourceSyobocalTitles',
  ANNICT_WORKS: 'admin.settings.animeSourceAnnictWorks',
} as const satisfies Partial<Record<AnimeImportSource, string>>;

type AnimeImportItem = {
  source: AnimeImportSource;
  checkedAt: string;
  importedAt: string | null | undefined;
  itemCount: number;
  error: string | null | undefined;
};

// AnimeLinkingSettings は、しょぼいカレンダー、Annict、AniList との連携 (アニメの番組と作品の結びつき) を使うかどうかを変え (設計書 3.8)、
// 連携先の情報を取り込んだ様子を出す。
function AnimeLinkingSettings({
  enabled,
  imports,
  busy,
  onSave,
  children,
}: {
  enabled: boolean;
  imports: readonly AnimeImportItem[];
  busy: boolean;
  onSave: (enabled: boolean) => void;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const [checked, setChecked] = useState(enabled);

  return (
    <Section title={t('admin.settings.anime')}>
      <Stack gap="sm">
        <Group justify="space-between" align="flex-end">
          <Switch
            size="sm"
            label={t('admin.settings.animeEnabled')}
            description={t('admin.settings.animeHelp')}
            checked={checked}
            onChange={(event) => setChecked(event.currentTarget.checked)}
            maw={560}
          />
          <Button
            size="compact-sm"
            disabled={checked === enabled || busy}
            onClick={() => onSave(checked)}
          >
            {t('admin.save')}
          </Button>
        </Group>
        {imports.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.settings.animeNotImported')}
          </Text>
        ) : (
          <Stack gap={4} data-testid="anime-imports">
            {imports.map((item) => (
              <Group key={item.source} gap="xs" wrap="nowrap" align="flex-start">
                <Text size="sm" fw={600} w={200}>
                  {item.source in importSourceKeys
                    ? t(importSourceKeys[item.source as keyof typeof importSourceKeys])
                    : item.source}
                </Text>
                <Stack gap={0}>
                  <Text size="sm">
                    {item.importedAt == null
                      ? t('admin.settings.animeNeverImported')
                      : t('admin.settings.animeImported', {
                          at: importedAtFormat.format(Date.parse(item.importedAt)),
                          items: item.itemCount,
                        })}
                  </Text>
                  {item.error != null && (
                    <Text size="xs" c="red">
                      {t('admin.settings.animeImportFailed', {
                        at: importedAtFormat.format(Date.parse(item.checkedAt)),
                        error: item.error,
                      })}
                    </Text>
                  )}
                </Stack>
              </Group>
            ))}
          </Stack>
        )}
        {children}
      </Stack>
    </Section>
  );
}

// AnimeImportTokenForm は、作品 ID の対応を作るための Annict のトークンを変える (#1223)。
// トークンはサーバーから返らないので、変えるときだけ入れる。
function AnimeImportTokenForm({
  configured,
  busy,
  onSave,
}: {
  configured: boolean;
  busy: boolean;
  onSave: (annictToken: string, onDone: () => void) => void;
}) {
  const { t } = useTranslation();
  const [token, setToken] = useState('');
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSave(token.trim(), () => setToken(''));
  };

  return (
    <form onSubmit={submit}>
      <Stack gap="xs">
        <Group gap="xs">
          <Text size="sm" fw={600}>
            {t('admin.settings.animeToken')}
          </Text>
          <Badge size="sm" variant="light" color={configured ? 'teal' : 'gray'}>
            {configured
              ? t('admin.settings.animeTokenConfigured')
              : t('admin.settings.animeTokenNotConfigured')}
          </Badge>
        </Group>
        <Text size="xs" c="dimmed" maw={560}>
          {t('admin.settings.animeTokenHelp')}
        </Text>
        <Group align="flex-end" wrap="wrap">
          <PasswordInput
            size="sm"
            aria-label={t('admin.settings.animeToken')}
            placeholder={configured ? t('admin.settings.animeTokenKept') : undefined}
            value={token}
            onChange={(event) => setToken(event.currentTarget.value)}
            autoComplete="new-password"
            w={320}
          />
          <Button type="submit" size="compact-sm" disabled={token.trim() === '' || busy}>
            {t('admin.save')}
          </Button>
          {configured && (
            <Button
              type="button"
              size="compact-sm"
              variant="default"
              color="red"
              disabled={busy}
              onClick={() => onSave('', () => setToken(''))}
            >
              {t('admin.settings.animeTokenDelete')}
            </Button>
          )}
        </Group>
      </Stack>
    </form>
  );
}

type AnimeRecordAppItem = {
  destination: AnimeRecordDestination;
  clientId: string | null | undefined;
  configured: boolean;
  redirectUri: string;
};

// AnimeRecordAppForm は、見たアニメを記録させるアプリ (Annict か AniList) のクライアント ID とシークレットを変える (#927)。
// シークレットはサーバーから返らないので、変えるときだけ入れる。クライアント ID を空にして保存すると、登録を消す。
function AnimeRecordAppForm({
  app,
  busy,
  onSave,
}: {
  app: AnimeRecordAppItem;
  busy: boolean;
  onSave: (input: AnimeRecordAppInput, onDone: () => void) => void;
}) {
  const { t } = useTranslation();
  const [clientId, setClientId] = useState(app.clientId ?? '');
  const [clientSecret, setClientSecret] = useState('');
  const name = app.destination === 'ANNICT' ? 'Annict' : 'AniList';
  const changed = clientId.trim() !== (app.clientId ?? '') || clientSecret !== '';
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSave(
      clientSecret === ''
        ? { clientId: clientId.trim() }
        : { clientId: clientId.trim(), clientSecret },
      () => setClientSecret(''),
    );
  };

  return (
    <form onSubmit={submit}>
      <Stack gap="xs">
        <Group gap="xs">
          <Text size="sm" fw={600}>
            {name}
          </Text>
          <Badge size="sm" variant="light" color={app.configured ? 'teal' : 'gray'}>
            {app.configured
              ? t('admin.animeRecords.configured')
              : t('admin.animeRecords.notConfigured')}
          </Badge>
        </Group>
        <Stack gap={2}>
          <Text size="xs" c="dimmed">
            {t('admin.animeRecords.redirectUri')}
          </Text>
          <Group gap="xs" wrap="nowrap">
            <Code style={{ overflowWrap: 'anywhere' }}>{app.redirectUri}</Code>
            <CopyButton value={app.redirectUri}>
              {({ copied, copy }) => (
                <Button size="compact-xs" variant="default" onClick={copy}>
                  {copied ? t('settings.account.copied') : t('settings.account.copy')}
                </Button>
              )}
            </CopyButton>
          </Group>
        </Stack>
        <Group align="flex-end" wrap="wrap">
          <TextInput
            size="sm"
            label={t('admin.animeRecords.clientId')}
            value={clientId}
            onChange={(event) => setClientId(event.currentTarget.value)}
            autoComplete="off"
            w={260}
          />
          <PasswordInput
            size="sm"
            label={t('admin.animeRecords.clientSecret')}
            placeholder={app.configured ? t('admin.animeRecords.secretKept') : undefined}
            value={clientSecret}
            onChange={(event) => setClientSecret(event.currentTarget.value)}
            autoComplete="new-password"
            w={260}
          />
          <Button type="submit" size="compact-sm" disabled={!changed || busy}>
            {t('admin.save')}
          </Button>
        </Group>
      </Stack>
    </form>
  );
}

type WebhookItem = {
  id: string;
  name: string;
  url: string;
  format: WebhookFormat;
  kinds: readonly string[];
  enabled: boolean;
};

// knownKinds は、サーバーが返した種類のうち、画面が知っているものだけを返す。
function knownKinds(values: readonly string[]): NotificationKindName[] {
  return kinds.filter((kind) => values.includes(kind));
}

// WebhookRow は、Webhook 1 つの行。URL は秘密のトークンを含むので、送り先のホストだけを出す。
function WebhookRow({
  webhook,
  onUpdate,
  onDelete,
}: {
  webhook: WebhookItem;
  onUpdate: (input: WebhookInput, onDone: () => void) => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const knownFormat = formats.find((format) => format === webhook.format);

  if (editing) {
    return (
      <Stack gap={4} data-webhook={webhook.name}>
        <WebhookForm
          initial={{
            name: webhook.name,
            url: webhook.url,
            format: webhook.format,
            kinds: knownKinds(webhook.kinds),
            enabled: webhook.enabled,
          }}
          submitLabel={t('admin.save')}
          onSubmit={(input, onDone) =>
            onUpdate(input, () => {
              onDone();
              setEditing(false);
            })
          }
          onCancel={() => setEditing(false)}
        />
      </Stack>
    );
  }

  return (
    <Stack gap={4} data-webhook={webhook.name}>
      <Group justify="space-between" wrap="nowrap" align="flex-start">
        <Stack gap={2} miw={0} flex={1}>
          <Group gap="xs">
            <Text fw={600}>{webhook.name}</Text>
            <Badge size="sm" variant="light">
              {knownFormat === undefined
                ? webhook.format
                : t(`admin.webhooks.formats.${knownFormat}`)}
            </Badge>
            <Badge size="sm" variant="light" color={webhook.enabled ? 'teal' : 'gray'}>
              {webhook.enabled ? t('admin.webhooks.enabled') : t('admin.webhooks.paused')}
            </Badge>
          </Group>
          <Text size="xs" c="dimmed" truncate>
            {urlHost(webhook.url)}
          </Text>
          <Group gap={4}>
            {knownKinds(webhook.kinds).map((kind) => (
              <Badge key={kind} size="xs" variant="outline" color="gray">
                {t(notificationKinds[kind])}
              </Badge>
            ))}
          </Group>
        </Stack>
        <Group gap="xs" flex="none">
          <Button size="compact-sm" variant="default" onClick={() => setEditing(true)}>
            {t('admin.webhooks.edit')}
          </Button>
          <Button
            size="compact-sm"
            variant="default"
            color="red"
            onClick={() => setConfirming(true)}
          >
            {t('admin.webhooks.delete')}
          </Button>
        </Group>
      </Group>
      {confirming && (
        <Alert color="red" role="alert">
          <Stack gap="xs">
            <Text size="sm">{t('admin.webhooks.confirmDelete', { name: webhook.name })}</Text>
            <Group gap="xs">
              <Button
                size="compact-sm"
                color="red"
                onClick={() => {
                  setConfirming(false);
                  onDelete();
                }}
              >
                {t('admin.webhooks.delete')}
              </Button>
              <Button size="compact-sm" variant="subtle" onClick={() => setConfirming(false)}>
                {t('admin.webhooks.cancel')}
              </Button>
            </Group>
          </Stack>
        </Alert>
      )}
    </Stack>
  );
}

// urlHost は、URL のうち、秘密を含まないホストの部分を返す。URL として読めなければ、何も出さない。
function urlHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return '';
  }
}

const emptyWebhook: WebhookInput = {
  name: '',
  url: '',
  format: 'JSON',
  kinds: [],
  enabled: true,
};

// WebhookForm は、Webhook を作るか変えるフォーム。
function WebhookForm({
  initial = emptyWebhook,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial?: WebhookInput;
  submitLabel: string;
  onSubmit: (input: WebhookInput, onDone: () => void) => void;
  onCancel?: () => void;
}) {
  const { t } = useTranslation();
  const [input, setInput] = useState<WebhookInput>(initial);
  const set = (patch: Partial<WebhookInput>) => setInput((current) => ({ ...current, ...patch }));
  const name = input.name.trim();
  const url = input.url.trim();

  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit({ ...input, name, url }, () => setInput(initial));
  };

  return (
    <form onSubmit={submit}>
      <Stack gap="xs">
        <Group gap="xs" align="flex-start">
          <TextInput
            size="xs"
            label={t('admin.webhooks.name')}
            required
            maxLength={100}
            value={input.name}
            onChange={(event) => set({ name: event.currentTarget.value })}
          />
          <Select
            size="xs"
            w={160}
            label={t('admin.webhooks.format')}
            data={formats.map((format) => ({
              value: format,
              label: t(`admin.webhooks.formats.${format}`),
            }))}
            value={input.format}
            allowDeselect={false}
            onChange={(value) => {
              const format = formats.find((known) => known === value);
              if (format !== undefined) {
                set({ format });
              }
            }}
          />
        </Group>
        <TextInput
          size="xs"
          type="url"
          label={t('admin.webhooks.url')}
          description={t('admin.webhooks.urlHelp')}
          required
          autoComplete="off"
          value={input.url}
          onChange={(event) => set({ url: event.currentTarget.value })}
        />
        <Checkbox.Group
          size="xs"
          label={t('admin.webhooks.kinds')}
          value={input.kinds}
          onChange={(values) => set({ kinds: knownKinds(values) })}
        >
          <Group gap="sm" mt={4}>
            {kinds.map((kind) => (
              <Checkbox key={kind} value={kind} label={t(notificationKinds[kind])} />
            ))}
          </Group>
        </Checkbox.Group>
        <Switch
          size="xs"
          label={t('admin.webhooks.enabled')}
          checked={input.enabled}
          onChange={(event) => set({ enabled: event.currentTarget.checked })}
        />
        <Group gap="xs">
          <Button
            type="submit"
            size="compact-sm"
            disabled={name === '' || url === '' || input.kinds.length === 0}
          >
            {submitLabel}
          </Button>
          {onCancel !== undefined && (
            <Button size="compact-sm" variant="subtle" onClick={onCancel}>
              {t('admin.webhooks.cancel')}
            </Button>
          )}
        </Group>
      </Stack>
    </form>
  );
}
