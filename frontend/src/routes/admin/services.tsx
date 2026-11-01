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
import type {
  AnimeChannelInput,
  servicesAnimeChannelMutation,
} from './__generated__/servicesAnimeChannelMutation.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';

export const Route = createFileRoute('/admin/services')({
  component: ServicesPage,
});

// ServicesPage は、放送波 (チャンネル種別) の表示名と並び順、番組表とライブに出すサービスと並び順を変える、
// 管理者向けの画面 (設計書 10.3)。タイムシフトで録るサービスは、タイムシフトの画面 (/admin/timeshift) で決める。
// しょぼいカレンダーと Annict のチャンネルの対応の上書き (設計書 3.8) も、ここで変える。
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
    systemSettings {
      animeLinkingEnabled
    }
    animeChannels {
      serviceId
      definitionName
      importedSyobocalChannelId
      importedAnnictChannelId
      syobocalChannelIdOverride
      annictChannelIdOverride
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

      <Section title={t('admin.system.animeChannels')}>
        <Text size="sm" c="dimmed">
          {t('admin.system.animeChannelsDescription')}
        </Text>
        {!data.systemSettings.animeLinkingEnabled && (
          <Text size="sm" c="dimmed">
            {t('admin.system.animeChannelsDisabled')}
          </Text>
        )}
        <ScrollArea.Autosize mah={480}>
          <Stack gap="xs" data-testid="anime-channels">
            {/* サービスの表示と同じ順に並べる。 */}
            {data.services.flatMap((service) => {
              const channel = data.animeChannels.find((item) => item.serviceId === service.id);
              if (channel === undefined) {
                return [];
              }

              return [
                <AnimeChannelEditor
                  key={`${channel.serviceId}/${channel.syobocalChannelIdOverride}/${channel.annictChannelIdOverride}`}
                  service={service}
                  channel={channel}
                  onSave={(input) =>
                    action.run(async () => {
                      await mutate<servicesAnimeChannelMutation>(
                        environment,
                        graphql`
                          mutation servicesAnimeChannelMutation(
                            $serviceId: ID!
                            $input: AnimeChannelInput!
                          ) {
                            updateAnimeChannel(serviceId: $serviceId, input: $input) {
                              serviceId
                              syobocalChannelIdOverride
                              annictChannelIdOverride
                            }
                          }
                        `,
                        { serviceId: channel.serviceId, input },
                      );
                      done();
                    }, fail)
                  }
                />,
              ];
            })}
          </Stack>
        </ScrollArea.Autosize>
      </Section>
    </Stack>
  );
}

type AnimeChannelItem = {
  definitionName: string | null | undefined;
  importedSyobocalChannelId: number | null | undefined;
  importedAnnictChannelId: number | null | undefined;
  syobocalChannelIdOverride: number | null | undefined;
  annictChannelIdOverride: number | null | undefined;
};

// overrideChange は、上書きの入力 value を、今の上書き current と比べ、変える項目にする。
// 空にすると上書きを消し、取り込んだ対応に戻す。変えていなければ、set も reset もない値を返す。
function overrideChange(
  value: number | string,
  current: number | null | undefined,
): { set?: number; reset: boolean } {
  if (typeof value !== 'number') {
    return { reset: current != null };
  }
  if (value === current) {
    return { reset: false };
  }

  return { set: value, reset: false };
}

// AnimeChannelEditor は、サービスとしょぼいカレンダーの ChID、Annict のチャンネルの対応の上書きを変える行 (設計書 3.8)。
// 入力を空にすると、上書きを消して、saya-definitions から取り込んだ対応 (入力の薄い文字) に戻す。
function AnimeChannelEditor({
  service,
  channel,
  onSave,
}: {
  service: {
    name: string;
    logoUrl: string | null | undefined;
    channelType: { displayName: string };
  };
  channel: AnimeChannelItem;
  onSave: (input: AnimeChannelInput) => void;
}) {
  const { t } = useTranslation();
  const [syobocal, setSyobocal] = useState<number | string>(
    channel.syobocalChannelIdOverride ?? '',
  );
  const [annict, setAnnict] = useState<number | string>(channel.annictChannelIdOverride ?? '');
  const imported = (id: number | null | undefined) =>
    id == null ? t('admin.system.animeChannelNone') : String(id);
  const syobocalChange = overrideChange(syobocal, channel.syobocalChannelIdOverride);
  const annictChange = overrideChange(annict, channel.annictChannelIdOverride);
  const changed =
    syobocalChange.set !== undefined ||
    syobocalChange.reset ||
    annictChange.set !== undefined ||
    annictChange.reset;

  return (
    <Group gap="xs" align="flex-end">
      <Stack gap={0} w={200} pb={4}>
        <Text size="sm" fw={600} truncate>
          <ServiceLabel name={service.name} logoUrl={service.logoUrl} />
        </Text>
        <Text size="xs" c="dimmed" truncate>
          {channel.definitionName ?? service.channelType.displayName}
        </Text>
      </Stack>
      <NumberInput
        size="xs"
        w={180}
        label={t('admin.system.syobocalChannel')}
        placeholder={imported(channel.importedSyobocalChannelId)}
        min={1}
        allowDecimal={false}
        value={syobocal}
        onChange={setSyobocal}
      />
      <NumberInput
        size="xs"
        w={180}
        label={t('admin.system.annictChannel')}
        placeholder={imported(channel.importedAnnictChannelId)}
        min={1}
        allowDecimal={false}
        value={annict}
        onChange={setAnnict}
      />
      <Button
        size="compact-sm"
        mb={2}
        disabled={!changed}
        onClick={() =>
          onSave({
            syobocalChannelId: syobocalChange.set,
            resetSyobocalChannelId: syobocalChange.reset,
            annictChannelId: annictChange.set,
            resetAnnictChannelId: annictChange.reset,
          })
        }
      >
        {t('admin.save')}
      </Button>
    </Group>
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
