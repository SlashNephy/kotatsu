import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Code,
  Group,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { Webhook } from 'lucide-react';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type FormEvent } from 'react';
import { useDataChanges } from '../../changes/DataChanges';
import { EmptyState } from '../../futon';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { AdminPage } from '../../admin/AdminPage';
import { authApi, type OIDCStatus } from '../../auth/api';
import { notificationKinds, type NotificationKindName } from '../../notifications/kinds';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import {
  reauthMethods,
  useReauthGate,
  useReauthResume,
  type ReauthResume,
} from '../../settings/useReauthGate';
import { SudoGate } from '../../settings/SudoGate';
import type {
  WebhookFormat,
  webhooksCreateMutation,
} from './__generated__/webhooksCreateMutation.graphql';
import type { webhooksDeleteMutation } from './__generated__/webhooksDeleteMutation.graphql';
import type { webhooksQuery } from './__generated__/webhooksQuery.graphql';
import type { webhooksUpdateMutation } from './__generated__/webhooksUpdateMutation.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/webhooks';

export const Route = createFileRoute('/admin/webhooks')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: WebhooksPage,
});

const formats = ['JSON', 'DISCORD', 'SLACK'] as const satisfies readonly WebhookFormat[];
// 視聴予約の通知は個人に届けるものなので、全体向けの Webhook では送らない (設計書 4.8)。
const kinds = (Object.keys(notificationKinds) as NotificationKindName[]).filter(
  (kind) => kind !== 'WATCH_REMINDER',
);

// WebhooksPage は、通知を外部のサービスに送る Webhook を作り、変え、消す、管理者向けの画面 (設計書 6.2、10.3)。
function WebhooksPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.webhooks.title')}>
      <Webhooks />
    </AdminPage>
  );
}

const query = graphql`
  query webhooksQuery {
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

function Webhooks() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<webhooksQuery>(
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

        {/* 空のときは、空の表示だけを出す。カードで囲むと、枠が二重になる。 */}
        {data.webhooks.length === 0 ? (
          <EmptyState
            icon={Webhook}
            title={t('admin.webhooks.empty')}
            hint={t('admin.webhooks.emptyHint')}
          />
        ) : (
          <Card withBorder>
            <Stack gap="sm">
              {data.webhooks.map((webhook) => (
                <WebhookRow
                  key={`${webhook.id}/${webhook.name}/${webhook.url}/${webhook.format}/${webhook.kinds.join()}/${String(webhook.enabled)}`}
                  webhook={webhook}
                  onUpdate={(input, onDone) =>
                    gate.run(async () => {
                      await mutate<webhooksUpdateMutation>(
                        environment,
                        graphql`
                          mutation webhooksUpdateMutation($id: ID!, $input: UpdateWebhookInput!) {
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
                  onDelete={(resume) =>
                    gate.run(
                      async () => {
                        await mutate<webhooksDeleteMutation>(
                          environment,
                          graphql`
                            mutation webhooksDeleteMutation($id: ID!) {
                              deleteWebhook(id: $id)
                            }
                          `,
                          { id: webhook.id },
                        );
                        succeed(t('admin.webhooks.deleted', { name: webhook.name }));
                      },
                      fail,
                      undefined,
                      undefined,
                      resume,
                    )
                  }
                />
              ))}
            </Stack>
          </Card>
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
                  const response = await mutate<webhooksCreateMutation>(
                    environment,
                    graphql`
                      mutation webhooksCreateMutation($input: CreateWebhookInput!) {
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
  onDelete: (resume: ReauthResume) => void;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const knownFormat = formats.find((format) => format === webhook.format);
  // 削除は、確かめてから押すので、IdP で再認証して戻ってきたら、そのまま続ける (#916)。
  const remove = () => onDelete({ key: 'webhook.delete', params: webhook.id });
  useReauthResume('webhook.delete', remove, webhook.id);

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
                  remove();
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
