import { Alert, Badge, Button, Group, NumberInput, Select, Stack, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { useAction } from '../../admin/useAction';
import { Section } from '../../admin/Section';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import type { timeshiftQuery } from './__generated__/timeshiftQuery.graphql';
import type { timeshiftRemoveMutation } from './__generated__/timeshiftRemoveMutation.graphql';
import type { timeshiftSetMutation } from './__generated__/timeshiftSetMutation.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';
import { renderServiceOption, serviceGroups } from '../../guide/serviceOption';
import { zonedFormat } from '../../futon';

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

export const Route = createFileRoute('/admin/timeshift')({
  component: TimeshiftPage,
});

// TimeshiftPage は、タイムシフトで録るサービスと保持時間を決める、管理者向けの画面 (設計書 5.7、10.3)。
function TimeshiftPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.timeshift.title')}>
      <TimeshiftSettings />
    </AdminPage>
  );
}

const query = graphql`
  query timeshiftQuery {
    # タイムシフトに足せるサービス。ほかの画面でサービスを選ぶときと同じく、映像か音声を流す、非表示でないサービスにする。
    services {
      id
      name
      logoUrl
      serviceId
      channelType {
        displayName
      }
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

function TimeshiftSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<timeshiftQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const action = useAction();
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const done = () => {
    setMessage({ ok: true, text: t('admin.saved') });
    refresh();
  };
  const setTimeshift = (serviceId: string, retentionSeconds: number) =>
    mutate<timeshiftSetMutation>(
      environment,
      graphql`
        mutation timeshiftSetMutation($serviceId: ID!, $retentionSeconds: Int!) {
          setTimeshiftChannel(serviceId: $serviceId, retentionSeconds: $retentionSeconds) {
            retentionSeconds
          }
        }
      `,
      { serviceId, retentionSeconds },
    );

  if (!data.timeshiftAvailable) {
    return (
      <Alert color="amber" role="status">
        {t('admin.timeshift.unavailable')}
      </Alert>
    );
  }

  return (
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

      <Section title={t('admin.timeshift.services')}>
        <Text size="sm" c="dimmed">
          {t('admin.timeshift.description')}
        </Text>
        {data.timeshiftChannels.length === 0 && <Text size="sm">{t('admin.timeshift.empty')}</Text>}
        {data.timeshiftChannels.map((channel) => (
          <TimeshiftServiceEditor
            key={`${channel.service.id}/${channel.retentionSeconds}`}
            channel={channel}
            onSave={(retentionSeconds) =>
              action.run(async () => {
                await setTimeshift(channel.service.id, retentionSeconds);
                done();
              }, fail)
            }
            onRemove={() =>
              action.run(async () => {
                await mutate<timeshiftRemoveMutation>(
                  environment,
                  graphql`
                    mutation timeshiftRemoveMutation($serviceId: ID!) {
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
        <TimeshiftServiceAdder
          services={data.services.filter(
            (service) =>
              !data.timeshiftChannels.some((channel) => channel.service.id === service.id),
          )}
          onAdd={(serviceId, retentionSeconds, onAdded) =>
            action.run(async () => {
              await setTimeshift(serviceId, retentionSeconds);
              onAdded();
              done();
            }, fail)
          }
        />
      </Section>
    </Stack>
  );
}

// retentionHours は、保持時間 (秒) を、画面で選ぶ時間にする。時間単位でない値は、近い時間に丸める。
function retentionHours(seconds: number): number {
  return Math.min(maxRetentionHours, Math.max(1, Math.round(seconds / secondsPerHour)));
}

// TimeshiftServiceEditor は、タイムシフトで録るサービスの、録っているかとバッファの範囲を出し、保持時間を変えるか、外す行。
function TimeshiftServiceEditor({
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
            {channel.recording ? t('admin.timeshift.recording') : t('admin.timeshift.stopped')}
          </Badge>
          <Text size="xs" c="dimmed">
            {channel.bufferedFrom && channel.bufferedUntil
              ? t('admin.timeshift.buffer', {
                  from: formatBufferTime(channel.bufferedFrom),
                  until: formatBufferTime(channel.bufferedUntil),
                })
              : t('admin.timeshift.noBuffer')}
          </Text>
        </Stack>
        <NumberInput
          size="xs"
          w={140}
          label={t('admin.timeshift.retention')}
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
          {t('admin.timeshift.remove')}
        </Button>
      </Group>
      {confirming && (
        <Alert color="red" role="alert">
          <Stack gap="xs">
            <Text size="sm">{t('admin.timeshift.confirmRemove', { name: service.name })}</Text>
            <Group gap="xs">
              <Button
                size="compact-sm"
                color="red"
                onClick={() => {
                  setConfirming(false);
                  onRemove();
                }}
              >
                {t('admin.timeshift.remove')}
              </Button>
              <Button size="compact-sm" variant="subtle" onClick={() => setConfirming(false)}>
                {t('admin.timeshift.cancel')}
              </Button>
            </Group>
          </Stack>
        </Alert>
      )}
    </Stack>
  );
}

// TimeshiftServiceAdder は、まだ録っていないサービスを選び、保持時間を決めて、タイムシフトで録るサービスに足す行。
// サービスを選ぶ欄は、ほかの画面と同じく、放送波ごとにまとめ、ロゴを添える。
function TimeshiftServiceAdder({
  services,
  onAdd,
}: {
  services: readonly {
    id: string;
    name: string;
    logoUrl: string | null | undefined;
    serviceId: number;
    channelType: { displayName: string };
  }[];
  onAdd: (serviceId: string, retentionSeconds: number, onAdded: () => void) => void;
}) {
  const { t } = useTranslation();
  const [serviceId, setServiceId] = useState<string | null>(null);
  const [hours, setHours] = useState<number | string>(defaultRetentionHours);

  return (
    <Group gap="xs" align="flex-end">
      <Select
        size="xs"
        w={320}
        label={t('admin.timeshift.service')}
        placeholder={t('admin.timeshift.servicePlaceholder')}
        data={serviceGroups(
          services,
          // 同じ名前のサービスが、別のチャンネル種別や、同じ放送局の別のサービスにもあるので、種別とサービス ID を添える。
          (s) => `${s.name}（${s.channelType.displayName}・${s.serviceId}）`,
        )}
        renderOption={renderServiceOption(services)}
        value={serviceId}
        onChange={setServiceId}
        searchable
      />
      <NumberInput
        size="xs"
        w={140}
        label={t('admin.timeshift.retention')}
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
            // 選んだサービスは、足し終えてから消す。失敗したときは、選び直さずにやり直せる。
            onAdd(serviceId, Number(hours) * secondsPerHour, () => setServiceId(null));
          }
        }}
      >
        {t('admin.timeshift.add')}
      </Button>
    </Group>
  );
}
