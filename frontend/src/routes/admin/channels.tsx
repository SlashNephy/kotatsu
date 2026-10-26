import {
  Alert,
  Badge,
  Card,
  Group,
  NumberInput,
  ScrollArea,
  Select,
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
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { SudoGate } from '../../settings/SudoGate';
import type { channelsChannelTypeMutation } from './__generated__/channelsChannelTypeMutation.graphql';
import type { channelsQuery } from './__generated__/channelsQuery.graphql';
import type { channelsServiceMutation } from './__generated__/channelsServiceMutation.graphql';
import type { channelsSetTimeshiftMutation } from './__generated__/channelsSetTimeshiftMutation.graphql';
import type { channelsRemoveTimeshiftMutation } from './__generated__/channelsRemoveTimeshiftMutation.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';
import { zonedFormat } from '../../futon';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/channels';
// タイムシフトの保持時間は、時間単位で 1〜24 時間から選ぶ。既定は 2 時間。
const defaultRetentionHours = 2;
const maxRetentionHours = 24;
const secondsPerHour = 3600;

const bufferFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

const formatBufferTime = (time: string) => bufferFormat.format(Date.parse(time));

export const Route = createFileRoute('/admin/channels')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: ChannelsPage,
});

// ChannelsPage は、チャンネル種別の表示名と並び順、番組表とライブに出すチャンネルと並び順、タイムシフトで録るチャンネルを変える、
// 管理者向けの画面 (設計書 10.3)。
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
    timeshiftAvailable
    timeshiftChannels {
      service {
        id
        name
        logoUrl
        channelType {
          displayName
        }
      }
      retentionSeconds
      bufferedFrom
      bufferedUntil
      recording
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
  const methods = reauthMethods(data.secondFactors, data.passkeys, oidc);
  const gate = useReauthGate(methods, path);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const done = () => {
    setMessage({ ok: true, text: t('admin.saved') });
    refresh();
  };
  const setTimeshift = (serviceId: string, retentionSeconds: number) =>
    mutate<channelsSetTimeshiftMutation>(
      environment,
      graphql`
        mutation channelsSetTimeshiftMutation($serviceId: ID!, $retentionSeconds: Int!) {
          setTimeshiftChannel(serviceId: $serviceId, retentionSeconds: $retentionSeconds) {
            retentionSeconds
          }
        }
      `,
      { serviceId, retentionSeconds },
    );

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

        <Section title={t('admin.system.channelTypes')}>
          {data.channelTypes.map((channelType) => (
            <ChannelTypeEditor
              key={`${channelType.type}/${channelType.displayName}/${channelType.sortOrder}`}
              channelType={channelType}
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

        <Section title={t('admin.system.timeshift')}>
          {data.timeshiftAvailable ? (
            <>
              <Text size="sm" c="dimmed">
                {t('admin.system.timeshiftDescription')}
              </Text>
              {data.timeshiftChannels.length === 0 && (
                <Text size="sm">{t('admin.system.timeshiftEmpty')}</Text>
              )}
              {data.timeshiftChannels.map((channel) => (
                <TimeshiftChannelEditor
                  key={`${channel.service.id}/${channel.retentionSeconds}`}
                  channel={channel}
                  onSave={(retentionSeconds) =>
                    gate.run(async () => {
                      await setTimeshift(channel.service.id, retentionSeconds);
                      done();
                    }, fail)
                  }
                  onRemove={() =>
                    gate.run(async () => {
                      await mutate<channelsRemoveTimeshiftMutation>(
                        environment,
                        graphql`
                          mutation channelsRemoveTimeshiftMutation($serviceId: ID!) {
                            removeTimeshiftChannel(serviceId: $serviceId)
                          }
                        `,
                        { serviceId: channel.service.id },
                      );
                      done();
                    }, fail)
                  }
                />
              ))}
              <TimeshiftChannelAdder
                services={data.services.filter(
                  (service) =>
                    !data.timeshiftChannels.some((channel) => channel.service.id === service.id),
                )}
                onAdd={(serviceId, retentionSeconds) =>
                  gate.run(async () => {
                    await setTimeshift(serviceId, retentionSeconds);
                    done();
                  }, fail)
                }
              />
            </>
          ) : (
            <Text size="sm" c="dimmed">
              {t('admin.system.timeshiftUnavailable')}
            </Text>
          )}
        </Section>
      </SudoGate>
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
  onSave: (input: { hidden: boolean; sortOrder?: number; resetSortOrder: boolean }) => void;
}) {
  const { t } = useTranslation();
  const [hidden, setHidden] = useState(service.hidden);
  const [sortOrder, setSortOrder] = useState<number | string>(service.sortOrder ?? '');
  const order = typeof sortOrder === 'number' ? sortOrder : null;

  return (
    <Group gap="xs" align="flex-end">
      <Stack gap={0} w={200} pb={4}>
        <Text size="sm" fw={600} truncate>
          <ServiceLabel name={service.name} logoUrl={service.logoUrl} />
        </Text>
        <Text size="xs" c="dimmed">
          {service.channelType.displayName}
        </Text>
      </Stack>
      <Switch
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
  onSave,
}: {
  channelType: { type: string; displayName: string; sortOrder: number };
  onSave: (input: { displayName: string; sortOrder: number }) => void;
}) {
  const { t } = useTranslation();
  const [displayName, setDisplayName] = useState(channelType.displayName);
  const [sortOrder, setSortOrder] = useState<number | string>(channelType.sortOrder);
  const name = displayName.trim();

  return (
    <Group gap="xs" align="flex-end" data-channel-type={channelType.type}>
      <Text fw={600} w={48} pb={4}>
        {channelType.type}
      </Text>
      <TextInput
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

// retentionHours は、保持時間 (秒) を、画面で選ぶ時間にする。時間単位でない値は、近い時間に丸める。
function retentionHours(seconds: number): number {
  return Math.min(maxRetentionHours, Math.max(1, Math.round(seconds / secondsPerHour)));
}

// TimeshiftChannelEditor は、タイムシフトで録るチャンネルの、録っているかとバッファの範囲を出し、保持時間を変えるか、外す行。
function TimeshiftChannelEditor({
  channel,
  onSave,
  onRemove,
}: {
  channel: {
    service: {
      id: string;
      name: string;
      logoUrl: string | null | undefined;
      channelType: { displayName: string };
    };
    retentionSeconds: number;
    bufferedFrom: string | null | undefined;
    bufferedUntil: string | null | undefined;
    recording: boolean;
  };
  onSave: (retentionSeconds: number) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const [hours, setHours] = useState<number | string>(retentionHours(channel.retentionSeconds));
  const [confirming, setConfirming] = useState(false);
  const { service } = channel;

  return (
    <Stack gap="xs" data-timeshift-service={service.id}>
      <Group gap="xs" align="flex-end">
        <Stack gap={0} w={200} pb={4}>
          <Text size="sm" fw={600} truncate>
            <ServiceLabel name={service.name} logoUrl={service.logoUrl} />
          </Text>
          <Text size="xs" c="dimmed">
            {service.channelType.displayName}
          </Text>
        </Stack>
        <Stack gap={2} w={220} pb={4}>
          <Badge size="sm" variant="light" color={channel.recording ? 'teal' : 'gray'}>
            {channel.recording
              ? t('admin.system.timeshiftRecording')
              : t('admin.system.timeshiftStopped')}
          </Badge>
          <Text size="xs" c="dimmed">
            {channel.bufferedFrom && channel.bufferedUntil
              ? t('admin.system.timeshiftBuffer', {
                  from: formatBufferTime(channel.bufferedFrom),
                  until: formatBufferTime(channel.bufferedUntil),
                })
              : t('admin.system.timeshiftNoBuffer')}
          </Text>
        </Stack>
        <NumberInput
          size="xs"
          w={140}
          label={t('admin.system.timeshiftRetention')}
          min={1}
          max={maxRetentionHours}
          allowDecimal={false}
          value={hours}
          onChange={setHours}
        />
        <Button
          size="compact-sm"
          mb={2}
          disabled={typeof hours !== 'number'}
          onClick={() => onSave(Number(hours) * secondsPerHour)}
        >
          {t('admin.save')}
        </Button>
        <Button
          size="compact-sm"
          mb={2}
          variant="default"
          color="red"
          onClick={() => setConfirming(true)}
        >
          {t('admin.system.timeshiftRemove')}
        </Button>
      </Group>
      {confirming && (
        <Alert color="red" role="alert">
          <Stack gap="xs">
            <Text size="sm">
              {t('admin.system.timeshiftConfirmRemove', { name: service.name })}
            </Text>
            <Group gap="xs">
              <Button
                size="compact-sm"
                color="red"
                onClick={() => {
                  setConfirming(false);
                  onRemove();
                }}
              >
                {t('admin.system.timeshiftRemove')}
              </Button>
              <Button size="compact-sm" variant="subtle" onClick={() => setConfirming(false)}>
                {t('admin.system.timeshiftCancel')}
              </Button>
            </Group>
          </Stack>
        </Alert>
      )}
    </Stack>
  );
}

// TimeshiftChannelAdder は、まだ録っていないサービスを選び、保持時間を決めて、タイムシフトで録るチャンネルに足す行。
function TimeshiftChannelAdder({
  services,
  onAdd,
}: {
  services: readonly { id: string; name: string; channelType: { displayName: string } }[];
  onAdd: (serviceId: string, retentionSeconds: number) => void;
}) {
  const { t } = useTranslation();
  const [serviceId, setServiceId] = useState<string | null>(null);
  const [hours, setHours] = useState<number | string>(defaultRetentionHours);

  return (
    <Group gap="xs" align="flex-end">
      <Select
        size="xs"
        w={260}
        label={t('admin.system.timeshiftService')}
        placeholder={t('admin.system.timeshiftServicePlaceholder')}
        searchable
        data={services.map((service) => ({
          value: service.id,
          label: `${service.name} (${service.channelType.displayName})`,
        }))}
        value={serviceId}
        onChange={setServiceId}
      />
      <NumberInput
        size="xs"
        w={140}
        label={t('admin.system.timeshiftRetention')}
        min={1}
        max={maxRetentionHours}
        allowDecimal={false}
        value={hours}
        onChange={setHours}
      />
      <Button
        size="compact-sm"
        mb={2}
        disabled={serviceId === null || typeof hours !== 'number'}
        onClick={() => {
          if (serviceId !== null) {
            onAdd(serviceId, Number(hours) * secondsPerHour);
            setServiceId(null);
          }
        }}
      >
        {t('admin.system.timeshiftAdd')}
      </Button>
    </Group>
  );
}
