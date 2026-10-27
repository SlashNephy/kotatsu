import {
  Alert,
  Button,
  Group,
  NumberInput,
  ScrollArea,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core';
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
import type { servicesChannelTypeMutation } from './__generated__/servicesChannelTypeMutation.graphql';
import type { servicesQuery } from './__generated__/servicesQuery.graphql';
import type { servicesServiceMutation } from './__generated__/servicesServiceMutation.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';

export const Route = createFileRoute('/admin/services')({
  component: ServicesPage,
});

// ServicesPage は、放送波 (チャンネル種別) の表示名と並び順、番組表とライブに出すサービスと並び順を変える、
// 管理者向けの画面 (設計書 10.3)。タイムシフトで録るサービスは、タイムシフトの画面 (/admin/timeshift) で決める。
function ServicesPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.services.title')}>
      <ServiceSettings />
    </AdminPage>
  );
}

const query = graphql`
  query servicesQuery {
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
  }
`;

function ServiceSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<servicesQuery>(
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

      <Section title={t('admin.system.channelTypes')}>
        {data.channelTypes.map((channelType) => (
          <ChannelTypeEditor
            key={`${channelType.type}/${channelType.displayName}/${channelType.sortOrder}`}
            channelType={channelType}
            onSave={(input) =>
              action.run(async () => {
                await mutate<servicesChannelTypeMutation>(
                  environment,
                  graphql`
                    mutation servicesChannelTypeMutation(
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
                  action.run(async () => {
                    await mutate<servicesServiceMutation>(
                      environment,
                      graphql`
                        mutation servicesServiceMutation($id: ID!, $input: ServiceInput!) {
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
