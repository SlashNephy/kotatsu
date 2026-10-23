import {
  Alert,
  Card,
  Group,
  NumberInput,
  ScrollArea,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
  Button,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { authApi, type OIDCStatus } from '../../auth/api';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import {
  reauthMethods,
  useReauthBeforeInput,
  useReauthGate,
  type ReauthPrepare,
} from '../../settings/useReauthGate';
import type { channelsChannelTypeMutation } from './__generated__/channelsChannelTypeMutation.graphql';
import type { channelsQuery } from './__generated__/channelsQuery.graphql';
import type { channelsServiceMutation } from './__generated__/channelsServiceMutation.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/channels';

export const Route = createFileRoute('/admin/channels')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: ChannelsPage,
});

// ChannelsPage は、チャンネル種別の表示名と並び順、番組表とライブに出すチャンネルと並び順を変える、管理者向けの画面 (設計書 10.3)。
function ChannelsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.channels.title')}>
      <ChannelSettings />
    </AdminPage>
  );
}

const query = graphql`
  query channelsQuery {
    channelTypes {
      type
      displayName
      sortOrder
    }
    services(includeHidden: true) {
      id
      name
      logoUrl
      hidden
      sortOrder
      channelType {
        displayName
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

function ChannelSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<channelsQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const gate = useReauthGate(reauthMethods(data.secondFactors, data.passkeys, oidc), path);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const done = () => {
    setMessage({ ok: true, text: t('admin.saved') });
    refresh();
  };

  return (
    <Stack gap="md">
      {gate.modal}
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

      <Section title={t('admin.system.channelTypes')}>
        {data.channelTypes.map((channelType) => (
          <ChannelTypeEditor
            key={`${channelType.type}/${channelType.displayName}/${channelType.sortOrder}`}
            channelType={channelType}
            prepare={gate.prepare}
            onSave={(input) =>
              gate.run(async () => {
                await mutate<channelsChannelTypeMutation>(
                  environment,
                  graphql`
                    mutation channelsChannelTypeMutation(
                      $type: String!
                      $input: ChannelTypeInput!
                    ) {
                      updateChannelType(type: $type, input: $input) {
                        type
                        displayName
                        sortOrder
                      }
                    }
                  `,
                  { type: channelType.type, input },
                );
                done();
              }, fail)
            }
          />
        ))}
      </Section>

      <Section title={t('admin.system.services')}>
        <Text size="sm" c="dimmed">
          {t('admin.system.servicesDescription')}
        </Text>
        <ScrollArea.Autosize mah={480}>
          <Stack gap="xs">
            {data.services.map((service) => (
              <ServiceEditor
                key={`${service.id}/${service.hidden}/${service.sortOrder}`}
                service={service}
                prepare={gate.prepare}
                onSave={(input) =>
                  gate.run(async () => {
                    await mutate<channelsServiceMutation>(
                      environment,
                      graphql`
                        mutation channelsServiceMutation($id: ID!, $input: ServiceInput!) {
                          updateService(id: $id, input: $input) {
                            id
                            hidden
                            sortOrder
                          }
                        }
                      `,
                      { id: service.id, input },
                    );
                    done();
                  }, fail)
                }
              />
            ))}
          </Stack>
        </ScrollArea.Autosize>
      </Section>
    </Stack>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {title}
        </Title>
        {children}
      </Stack>
    </Card>
  );
}

// ServiceEditor は、サービスの、番組表に出さない印と並び順の上書きを変える行。
// 並び順を空にして保存すると、上書きを消して、リモコンのキーの順に戻す。
function ServiceEditor({
  service,
  prepare,
  onSave,
}: {
  service: {
    id: string;
    name: string;
    logoUrl: string | null | undefined;
    hidden: boolean;
    sortOrder: number | null | undefined;
    channelType: { displayName: string };
  };
  prepare: ReauthPrepare;
  onSave: (input: { hidden: boolean; sortOrder?: number; resetSortOrder: boolean }) => void;
}) {
  const { t } = useTranslation();
  const [hidden, setHidden] = useState(service.hidden);
  const [sortOrder, setSortOrder] = useState<number | string>(service.sortOrder ?? '');
  const order = typeof sortOrder === 'number' ? sortOrder : null;
  // 変え始める前に再認証させる。IdP で再認証すると、ページを離れ、変えた値が消えるため (#916)。
  const input = useReauthBeforeInput(prepare, 'service.edit', service.id);

  return (
    <Group gap="xs" align="flex-end" onFocus={input.onFocus}>
      <Stack gap={0} w={200} pb={4}>
        <Text size="sm" fw={600} truncate>
          <ServiceLabel name={service.name} logoUrl={service.logoUrl} />
        </Text>
        <Text size="xs" c="dimmed">
          {service.channelType.displayName}
        </Text>
      </Stack>
      <Switch
        ref={input.ref}
        size="sm"
        pb={6}
        label={t('admin.system.serviceHidden')}
        checked={hidden}
        onChange={(event) => setHidden(event.currentTarget.checked)}
      />
      <NumberInput
        size="xs"
        w={120}
        label={t('admin.system.serviceOrder')}
        placeholder={t('admin.system.serviceOrderDefault')}
        min={0}
        max={10000}
        allowDecimal={false}
        value={sortOrder}
        onChange={setSortOrder}
      />
      <Button
        size="compact-sm"
        mb={2}
        onClick={() =>
          onSave(
            order === null
              ? { hidden, resetSortOrder: true }
              : { hidden, sortOrder: order, resetSortOrder: false },
          )
        }
      >
        {t('admin.save')}
      </Button>
    </Group>
  );
}

function ChannelTypeEditor({
  channelType,
  prepare,
  onSave,
}: {
  channelType: { type: string; displayName: string; sortOrder: number };
  prepare: ReauthPrepare;
  onSave: (input: { displayName: string; sortOrder: number }) => void;
}) {
  const { t } = useTranslation();
  const [displayName, setDisplayName] = useState(channelType.displayName);
  const [sortOrder, setSortOrder] = useState<number | string>(channelType.sortOrder);
  const name = displayName.trim();
  // 変え始める前に再認証させる。IdP で再認証すると、ページを離れ、変えた値が消えるため (#916)。
  const input = useReauthBeforeInput(prepare, 'channelType.edit', channelType.type);

  return (
    <Group gap="xs" align="flex-end" data-channel-type={channelType.type} onFocus={input.onFocus}>
      <Text fw={600} w={48} pb={4}>
        {channelType.type}
      </Text>
      <TextInput
        ref={input.ref}
        size="xs"
        w={160}
        label={t('admin.system.channelTypeName')}
        maxLength={32}
        value={displayName}
        onChange={(event) => setDisplayName(event.currentTarget.value)}
      />
      <NumberInput
        size="xs"
        w={120}
        label={t('admin.system.channelTypeOrder')}
        min={0}
        max={10000}
        allowDecimal={false}
        value={sortOrder}
        onChange={setSortOrder}
      />
      <Button
        size="compact-sm"
        disabled={name === '' || typeof sortOrder !== 'number'}
        onClick={() => onSave({ displayName: name, sortOrder: Number(sortOrder) })}
      >
        {t('admin.save')}
      </Button>
    </Group>
  );
}
