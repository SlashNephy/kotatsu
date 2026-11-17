import { Group, ScrollArea, Stack, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { Section } from '../../admin/Section';
import {
  AutoSaveNumberInput,
  AutoSaveSwitch,
  AutoSaveTextInput,
} from '../../settings/AutoSaveFields';
import { mutate } from '../../settings/mutate';
import { useAutoSave } from '../../settings/useAutoSave';
import type {
  ChannelTypeInput,
  servicesChannelTypeMutation,
} from './__generated__/servicesChannelTypeMutation.graphql';
import type { servicesQuery } from './__generated__/servicesQuery.graphql';
import type {
  ServiceInput,
  servicesServiceMutation,
} from './__generated__/servicesServiceMutation.graphql';
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
// どの項目も、変えたらその項目だけを保存する (#1740)。
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
  const { save } = useAutoSave();

  const saveChannelType = (type: string, input: ChannelTypeInput) =>
    save(async () => {
      await mutate<servicesChannelTypeMutation>(
        environment,
        graphql`
          mutation servicesChannelTypeMutation($type: String!, $input: ChannelTypeInput!) {
            updateChannelType(type: $type, input: $input) {
              type
              displayName
              sortOrder
            }
          }
        `,
        { type, input },
      );
      refresh();
    });
  const saveService = (id: string, input: ServiceInput) =>
    save(async () => {
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
        { id, input },
      );
      refresh();
    });
  const saveAnimeChannel = (serviceId: string, input: AnimeChannelInput) =>
    save(async () => {
      await mutate<servicesAnimeChannelMutation>(
        environment,
        graphql`
          mutation servicesAnimeChannelMutation($serviceId: ID!, $input: AnimeChannelInput!) {
            updateAnimeChannel(serviceId: $serviceId, input: $input) {
              serviceId
              syobocalChannelIdOverride
              annictChannelIdOverride
            }
          }
        `,
        { serviceId, input },
      );
      refresh();
    });

  return (
    <Stack gap="md">
      <Section title={t('admin.system.channelTypes')}>
        {data.channelTypes.map((channelType) => (
          <Group
            key={channelType.type}
            gap="xs"
            align="flex-start"
            data-channel-type={channelType.type}
          >
            <Text fw={600} w={48} pt={28}>
              {channelType.type}
            </Text>
            <AutoSaveTextInput
              size="xs"
              w={160}
              label={t('admin.system.channelTypeName')}
              maxLength={32}
              required
              value={channelType.displayName}
              onSave={(displayName) => saveChannelType(channelType.type, { displayName })}
            />
            <AutoSaveNumberInput
              size="xs"
              w={120}
              label={t('admin.system.channelTypeOrder')}
              min={0}
              max={10000}
              allowDecimal={false}
              value={channelType.sortOrder}
              onSave={(sortOrder) => saveChannelType(channelType.type, { sortOrder })}
            />
          </Group>
        ))}
      </Section>

      <Section title={t('admin.system.services')}>
        <Text size="sm" c="dimmed">
          {t('admin.system.servicesDescription')}
        </Text>
        <ScrollArea.Autosize mah={480}>
          <Stack gap="xs">
            {data.services.map((service) => (
              // 番組表に出さない印と、並び順の上書き。並び順を空にすると、上書きを消して、リモコンのキーの順に戻す。
              <Group key={service.id} gap="xs" align="flex-end">
                <ServiceName
                  name={service.name}
                  logoUrl={service.logoUrl}
                  sub={service.channelType.displayName}
                />
                <AutoSaveSwitch
                  size="sm"
                  pb={6}
                  label={t('admin.system.serviceHidden')}
                  checked={service.hidden}
                  onSave={(hidden) => saveService(service.id, { hidden })}
                />
                <AutoSaveNumberInput
                  nullable
                  size="xs"
                  w={120}
                  label={t('admin.system.serviceOrder')}
                  placeholder={t('admin.system.serviceOrderDefault')}
                  min={0}
                  max={10000}
                  allowDecimal={false}
                  value={service.sortOrder ?? null}
                  onSave={(order) =>
                    saveService(
                      service.id,
                      order === null ? { resetSortOrder: true } : { sortOrder: order },
                    )
                  }
                />
              </Group>
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
              const imported = (id: number | null | undefined) =>
                id == null ? t('admin.system.animeChannelNone') : String(id);

              // しょぼいカレンダーの ChID と Annict のチャンネルの対応の上書き (設計書 3.8)。
              // 欄を空にすると、上書きを消して、saya-definitions から取り込んだ対応 (欄の薄い文字) に戻す。
              return [
                <Group key={channel.serviceId} gap="xs" align="flex-end">
                  <ServiceName
                    name={service.name}
                    logoUrl={service.logoUrl}
                    sub={channel.definitionName ?? service.channelType.displayName}
                  />
                  <AutoSaveNumberInput
                    nullable
                    size="xs"
                    w={180}
                    label={t('admin.system.syobocalChannel')}
                    placeholder={imported(channel.importedSyobocalChannelId)}
                    min={1}
                    allowDecimal={false}
                    value={channel.syobocalChannelIdOverride ?? null}
                    onSave={(id) =>
                      saveAnimeChannel(
                        channel.serviceId,
                        id === null ? { resetSyobocalChannelId: true } : { syobocalChannelId: id },
                      )
                    }
                  />
                  <AutoSaveNumberInput
                    nullable
                    size="xs"
                    w={180}
                    label={t('admin.system.annictChannel')}
                    placeholder={imported(channel.importedAnnictChannelId)}
                    min={1}
                    allowDecimal={false}
                    value={channel.annictChannelIdOverride ?? null}
                    onSave={(id) =>
                      saveAnimeChannel(
                        channel.serviceId,
                        id === null ? { resetAnnictChannelId: true } : { annictChannelId: id },
                      )
                    }
                  />
                </Group>,
              ];
            })}
          </Stack>
        </ScrollArea.Autosize>
      </Section>
    </Stack>
  );
}

// ServiceName は、行の左に出す、サービスの名前とロゴ、その下の小さな説明 sub。
function ServiceName({
  name,
  logoUrl,
  sub,
}: {
  name: string;
  logoUrl: string | null | undefined;
  sub: string;
}) {
  return (
    <Stack gap={0} w={200} pb={4}>
      <Text size="sm" fw={600} truncate>
        <ServiceLabel name={name} logoUrl={logoUrl} />
      </Text>
      <Text size="xs" c="dimmed" truncate>
        {sub}
      </Text>
    </Stack>
  );
}
