import {
  Alert,
  Anchor,
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
import { startTransition, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useDataChanges } from '../../changes/DataChanges';
import { EmptyState, zonedFormat } from '../../futon';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { AdminPage } from '../../admin/AdminPage';
import {
  GoogleDriveSettings,
  googleCallbackErrors,
  type GoogleCallbackError,
} from '../../admin/GoogleDriveSettings';
import { IssuedStreamUrls } from '../../admin/IssuedStreamUrls';
import { Section } from '../../admin/Section';
import { authApi, type OIDCStatus } from '../../auth/api';
import {
  notificationKinds,
  personalKinds,
  type NotificationKindName,
} from '../../notifications/kinds';
import {
  AutoSaveSelect,
  AutoSaveSwitch,
  AutoSaveTextInput,
  useAutoSavedValue,
} from '../../settings/AutoSaveFields';
import { notifySaved, useSaveNotice } from '../../settings/saveNotice';
import { useAutoSave } from '../../settings/useAutoSave';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { parseTMDbSeriesId, TMDbAttribution } from '../../drama/DramaEpisode';
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
import type { integrationsGoogleAppMutation } from './__generated__/integrationsGoogleAppMutation.graphql';
import type { integrationsGoogleDriveBeginMutation } from './__generated__/integrationsGoogleDriveBeginMutation.graphql';
import type { integrationsGoogleDriveAddFolderMutation } from './__generated__/integrationsGoogleDriveAddFolderMutation.graphql';
import type { integrationsGoogleDriveRemoveFolderMutation } from './__generated__/integrationsGoogleDriveRemoveFolderMutation.graphql';
import type { integrationsGoogleDriveDisconnectMutation } from './__generated__/integrationsGoogleDriveDisconnectMutation.graphql';
import type { integrationsAnimeImportTokenMutation } from './__generated__/integrationsAnimeImportTokenMutation.graphql';
import type { integrationsDramaTokenMutation } from './__generated__/integrationsDramaTokenMutation.graphql';
import type {
  DramaTitleOverrideInput,
  integrationsDramaOverrideMutation,
} from './__generated__/integrationsDramaOverrideMutation.graphql';
import type { AnimeImportSource, CommentSource } from './__generated__/integrationsQuery.graphql';
import type { integrationsQuery } from './__generated__/integrationsQuery.graphql';
import type { integrationsRevokeStreamUrlMutation } from './__generated__/integrationsRevokeStreamUrlMutation.graphql';
import type {
  UpdateWebhookInput,
  integrationsWebhookUpdateMutation,
} from './__generated__/integrationsWebhookUpdateMutation.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/integrations';

type IntegrationsSearch = {
  // connected は、Google Drive を接続して戻ってきたときに付く (#1902)。
  connected?: 'google-drive';
  // error は、Google Drive の接続に失敗して戻ってきたときの理由。
  error?: GoogleCallbackError;
};

export const Route = createFileRoute('/admin/integrations')({
  validateSearch: (search: Record<string, unknown>): IntegrationsSearch => {
    const result: IntegrationsSearch = {};
    if (search['connected'] === 'google-drive') {
      result.connected = 'google-drive';
    }
    const error = googleCallbackErrors.find((reason) => reason === search['error']);
    if (error !== undefined) {
      result.error = error;
    }

    return result;
  },
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: IntegrationsPage,
});

const formats = ['JSON', 'DISCORD', 'SLACK'] as const satisfies readonly WebhookFormat[];
// 視聴予約と見たアニメの記録の通知は個人に届けるものなので、全体向けの Webhook では送らない (設計書 4.8、6.2)。
const kinds = (Object.keys(notificationKinds) as NotificationKindName[]).filter(
  (kind) => !personalKinds.includes(kind),
);

// IntegrationsPage は、外部のサービスとの連携 (MCP のクライアント、データ放送の通信の中継、外部プレイヤーと IPTV のクライアントの URL、ニコニコ実況、しょぼいカレンダー、Annict、AniList、
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
      externalPlayerUrlsEnabled
      iptvFeedsEnabled
      recordingDownloadsEnabled
      epgstationApiEnabled
      networkActivityEnabled
      animeLinkingEnabled
      dramaLinkingEnabled
      commentSources
      hibpEnabled
      webPushSubject
    }
    # 発行された外部プレイヤー用の URL (#1757)。
    issuedStreamUrls {
      id
      userDisplayName
      targetName
      live
      createdAt
      expiresAt
      lastUsedAt
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
    # Google Drive をライブラリとして接続する (#1902)。
    googleIntegration {
      clientId
      configured
      redirectUri
      drive {
        accountName
        connectedAt
        revokedAt
      }
      folders {
        id
        name
      }
    }
    # TMDb との連携 (#1651、設計書 3.9)。
    dramaLinking {
      tokenConfigured
      tokenHint
    }
    dramaTitleOverrides {
      titleKey
      tmdbSeriesId
      updatedAt
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

function Integrations() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const clearSearch = () => void navigate({ search: {}, replace: true });
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
  const notice = useSaveNotice();
  const { save } = useAutoSave();
  // secret は、作った Webhook の署名用の秘密キー。作ったときにしか返らないので、ここで出す。
  const [secret, setSecret] = useState<{ name: string; secret: string } | null>(null);
  // revoking は、外部プレイヤーの URL を取り消している途中かどうか (#1757)。続けて押して、同じ URL を二度取り消さないようにする。
  const [revoking, setRevoking] = useState(false);
  // fail と succeed は、設定ではない操作 (トークンの登録、Webhook を作る、URL を取り消すなど) の結果を、
  // 設定の保存と同じ通知で知らせる (#1740)。
  const fail = (caught: unknown) => notice.failed(caught);
  const succeed = (text: string) => {
    notifySaved(text);
    refresh();
  };
  // saveSettings は、全体の設定のうち、変えた項目だけを、変えたらすぐ保存する (#1740)。どれも再認証を求める (設計書 9.5)。
  // 再認証をやめたら、保存できなかった知らせは出さず、切り替えを元に戻す。
  // input に関数を渡すと、送る直前に入力を作る。前の保存を待っている間に変えた分も含めて送るため。
  const saveSettings = (input: SystemSettingsInput | (() => SystemSettingsInput)) =>
    save(() =>
      gate.save(async () => {
        await mutate<integrationsSystemSettingsMutation>(
          environment,
          graphql`
            mutation integrationsSystemSettingsMutation($input: SystemSettingsInput!) {
              updateSystemSettings(input: $input) {
                updatedAt
              }
            }
          `,
          { input: typeof input === 'function' ? input() : input },
        );
        refresh();
      }),
    );
  // enabledSources は、実況のコメントのソースの、送っている途中の一覧。ソースの一覧はまるごと置き換えるので、
  // 続けて切り替えたときに、前の切り替えを読み直す前の一覧で上書きしないよう、送る一覧をここに積む。
  const enabledSources = useRef<{ sources: readonly CommentSource[]; pending: number } | null>(
    null,
  );
  const toggleSource = (source: CommentSource, enabled: boolean) => {
    const queued = enabledSources.current ?? {
      sources: data.systemSettings.commentSources,
      pending: 0,
    };
    const others = queued.sources.filter((value) => value !== source);
    const sources = enabled ? [...others, source] : others;
    enabledSources.current = { sources, pending: queued.pending + 1 };

    return saveSettings(() => ({
      commentSources: [...(enabledSources.current?.sources ?? sources)],
    })).then((ok) => {
      // すべて送り終えたら、読み直した一覧から数え直す。
      const current = enabledSources.current;
      enabledSources.current =
        current === null || current.pending <= 1
          ? null
          : { sources: current.sources, pending: current.pending - 1 };

      return ok;
    });
  };

  return (
    <Stack gap="md">
      {gate.modal}
      <SudoGate methods={methods} returnTo={path}>
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
          key="mcp"
          title={t('admin.settings.mcp')}
          label={t('admin.settings.mcpEnabled')}
          help={t('admin.settings.mcpHelp')}
          enabled={data.systemSettings.mcpEnabled}
          onSave={(mcpEnabled) => saveSettings({ mcpEnabled })}
        />

        {/* 外部との通信を記録するか (#1369、設計書 11.8)。 */}
        <SwitchSetting
          key="network-activity"
          title={t('networkActivity.title')}
          label={t('networkActivity.adminLabel')}
          help={t('networkActivity.adminHelp')}
          enabled={data.systemSettings.networkActivityEnabled}
          onSave={(networkActivityEnabled) => saveSettings({ networkActivityEnabled })}
        />

        {/* Have I Been Pwned への照会 (設計書 9.3)。外部へ問い合わせ始めるので、ここで再認証を求める。設定ファイルから移した (#1869)。 */}
        <SwitchSetting
          key="hibp"
          title={t('admin.settings.hibp')}
          label={t('admin.settings.hibpEnabled')}
          help={t('admin.settings.hibpHelp')}
          enabled={data.systemSettings.hibpEnabled}
          onSave={(hibpEnabled) => saveSettings({ hibpEnabled })}
        />

        {/* Web Push で、プッシュサービスに示す送り手の連絡先 (設計書 6.2)。設定ファイルから移した (#1869)。 */}
        <WebPushSubjectSetting
          subject={data.systemSettings.webPushSubject ?? ''}
          onSave={(webPushSubject) => saveSettings({ webPushSubject })}
        />

        {/* データ放送の文書の双方向の通信を、tenban に中継させるか (#501)。受信機の情報が外部へ届くので、ここで再認証を求める。 */}
        <SwitchSetting
          key="data-broadcast"
          title={t('admin.settings.dataBroadcastIp')}
          label={t('admin.settings.dataBroadcastIpEnabled')}
          help={t('admin.settings.dataBroadcastIpHelp')}
          enabled={data.systemSettings.dataBroadcastIpEnabled}
          onSave={(dataBroadcastIpEnabled) => saveSettings({ dataBroadcastIpEnabled })}
        />

        {/* 外の道具に渡す URL を発行させるか (#1755)。URL を持つ人が映像を見られるようになるので、ここで再認証を求める。 */}
        <SwitchSetting
          key="external-player-urls"
          title={t('admin.settings.externalPlayerUrls')}
          label={t('admin.settings.externalPlayerUrlsEnabled')}
          help={t('admin.settings.externalPlayerUrlsHelp')}
          enabled={data.systemSettings.externalPlayerUrlsEnabled}
          onSave={(externalPlayerUrlsEnabled) => saveSettings({ externalPlayerUrlsEnabled })}
        />
        {/* 発行された外部プレイヤー用の URL を、1 本ずつ取り消せる (#1757)。権限を広げない操作なので、再認証は求めない。
            無効にしている間も、期限の過ぎていない URL があれば出す。 */}
        {(data.systemSettings.externalPlayerUrlsEnabled || data.issuedStreamUrls.length > 0) && (
          <IssuedStreamUrls
            urls={data.issuedStreamUrls}
            busy={gate.busy || revoking}
            onRevoke={(id) => {
              setRevoking(true);
              mutate<integrationsRevokeStreamUrlMutation>(
                environment,
                graphql`
                  mutation integrationsRevokeStreamUrlMutation($id: ID!) {
                    revokeStreamUrl(id: $id)
                  }
                `,
                { id },
              )
                .then(() => succeed(t('admin.issuedStreamUrls.revoked')), fail)
                .finally(() => setRevoking(false));
            }}
          />
        )}
        {/* 録画を 1 本のファイルとして保存させるか (#1762)。録画を Kotatsu の外へ持ち出せるので、ここで再認証を求める。 */}
        <SwitchSetting
          key="recording-downloads"
          title={t('admin.settings.recordingDownloads')}
          label={t('admin.settings.recordingDownloadsEnabled')}
          help={t('admin.settings.recordingDownloadsHelp')}
          enabled={data.systemSettings.recordingDownloadsEnabled}
          onSave={(recordingDownloadsEnabled) => saveSettings({ recordingDownloadsEnabled })}
        />
        {/* EPGStation 互換の API を提供するか (#1769)。トークンを持つ人が映像を見られるので、ここで再認証を求める。 */}
        <SwitchSetting
          key="epgstation-api"
          title={t('admin.settings.epgstationApi')}
          label={t('admin.settings.epgstationApiEnabled')}
          help={t('admin.settings.epgstationApiHelp')}
          enabled={data.systemSettings.epgstationApiEnabled}
          onSave={(epgstationApiEnabled) => saveSettings({ epgstationApiEnabled })}
        />
        <SwitchSetting
          key="iptv-feeds"
          title={t('admin.settings.iptvFeeds')}
          label={t('admin.settings.iptvFeedsEnabled')}
          help={t('admin.settings.iptvFeedsHelp')}
          enabled={data.systemSettings.iptvFeedsEnabled}
          onSave={(iptvFeedsEnabled) => saveSettings({ iptvFeedsEnabled })}
        />

        {/* 実況のコメントのソースごとのオプトイン (#1405)。外部と通信し始めるので、ここで再認証を求める (設計書 0.3、7.4)。 */}
        {commentSources.map((source) => {
          const enabled = data.systemSettings.commentSources.includes(source);

          return (
            <SwitchSetting
              key={source}
              title={t(`admin.settings.commentSource.${source}.title`)}
              label={t(`admin.settings.commentSource.${source}.label`)}
              help={t(`admin.settings.commentSource.${source}.help`)}
              enabled={enabled}
              onSave={(next) => toggleSource(source, next)}
            />
          );
        })}

        <AnimeLinkingSettings
          key="anime"
          enabled={data.systemSettings.animeLinkingEnabled}
          imports={data.animeImports}
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

        {/* TMDb との連携 (#1651)。外部へ問い合わせ始めるかどうかを決めるので、ここで再認証を求める。 */}
        <DramaLinkingSettings
          key="drama"
          enabled={data.systemSettings.dramaLinkingEnabled}
          tokenConfigured={data.dramaLinking.tokenConfigured}
          tokenForm={
            // TMDb の API の読み取りアクセストークン。サーバーが外部へ問い合わせるときの秘密情報なので、ここで再認証を求める。
            <DramaTokenForm
              key={`${String(data.dramaLinking.tokenConfigured)}/${data.dramaLinking.tokenHint ?? ''}`}
              configured={data.dramaLinking.tokenConfigured}
              hint={data.dramaLinking.tokenHint ?? null}
              busy={gate.busy}
              onSave={(token, onDone) =>
                gate.run(async () => {
                  await mutate<integrationsDramaTokenMutation>(
                    environment,
                    graphql`
                      mutation integrationsDramaTokenMutation($token: String!) {
                        updateDramaLinkingToken(token: $token) {
                          tokenConfigured
                          tokenHint
                        }
                      }
                    `,
                    { token },
                  );
                  onDone();
                  succeed(t('admin.saved'));
                }, fail)
              }
            />
          }
          overrides={data.dramaTitleOverrides}
          onSave={(dramaLinkingEnabled) => saveSettings({ dramaLinkingEnabled })}
          onOverride={async (input) => {
            try {
              await mutate<integrationsDramaOverrideMutation>(
                environment,
                graphql`
                  mutation integrationsDramaOverrideMutation($input: DramaTitleOverrideInput!) {
                    updateDramaTitleOverride(input: $input) {
                      titleKey
                    }
                  }
                `,
                { input },
              );
              succeed(t('admin.saved'));

              return true;
            } catch (caught) {
              fail(caught);

              return false;
            }
          }}
        />

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

        {/* Google Drive をライブラリとして接続する (#1902)。サーバーが Google と通信し始めるかどうかと、Drive のファイルを家族に
            見せ始めるかどうかを決めるので、クライアントの登録と接続で再認証を求める。外すのは見せるものを減らすので、求めない。 */}
        <GoogleDriveSettings
          integration={data.googleIntegration}
          busy={gate.busy}
          connected={search.connected === 'google-drive'}
          callbackError={search.error ?? null}
          onDismiss={clearSearch}
          onSaveApp={(input, onDone) =>
            gate.run(async () => {
              await mutate<integrationsGoogleAppMutation>(
                environment,
                graphql`
                  mutation integrationsGoogleAppMutation($input: GoogleAppInput!) {
                    updateGoogleApp(input: $input) {
                      configured
                    }
                  }
                `,
                { input },
              );
              onDone();
              succeed(t('admin.saved'));
            }, fail)
          }
          onConnect={() =>
            gate.run(async () => {
              const response = await mutate<integrationsGoogleDriveBeginMutation>(
                environment,
                graphql`
                  mutation integrationsGoogleDriveBeginMutation {
                    beginGoogleDriveConnection
                  }
                `,
                {},
              );
              window.location.assign(response.beginGoogleDriveConnection);
            }, fail)
          }
          onAddFolder={(id, onDone) =>
            gate.run(async () => {
              await mutate<integrationsGoogleDriveAddFolderMutation>(
                environment,
                graphql`
                  mutation integrationsGoogleDriveAddFolderMutation($id: String!) {
                    addGoogleDriveFolder(id: $id) {
                      folders {
                        id
                      }
                    }
                  }
                `,
                { id },
              );
              onDone();
              succeed(t('admin.googleDrive.folders.added'));
            }, fail)
          }
          onRemoveFolder={(id) => {
            mutate<integrationsGoogleDriveRemoveFolderMutation>(
              environment,
              graphql`
                mutation integrationsGoogleDriveRemoveFolderMutation($id: String!) {
                  removeGoogleDriveFolder(id: $id) {
                    folders {
                      id
                    }
                  }
                }
              `,
              { id },
            ).then(() => succeed(t('admin.googleDrive.folders.removed')), fail);
          }}
          onDisconnect={() => {
            mutate<integrationsGoogleDriveDisconnectMutation>(
              environment,
              graphql`
                mutation integrationsGoogleDriveDisconnectMutation {
                  disconnectGoogleDrive {
                    configured
                  }
                }
              `,
              {},
            ).then(() => {
              clearSearch();
              succeed(t('admin.googleDrive.disconnected'));
            }, fail);
          }}
        />

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
                  key={webhook.id}
                  webhook={webhook}
                  onUpdate={(input) =>
                    save(() =>
                      gate.save(async () => {
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
                        refresh();
                      }),
                    )
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
// commentSources は、実況のコメントのソースの一覧 (#1405)。ソースを足したら、ここと翻訳に足す。
const commentSources = ['NX_JIKKYO', 'NICOLOG'] as const;

function SwitchSetting({
  title,
  label,
  help,
  enabled,
  onSave,
}: {
  title: string;
  label: string;
  help: string;
  enabled: boolean;
  onSave: (enabled: boolean) => Promise<boolean>;
}) {
  // 切り替えたらすぐ保存する (#1740)。
  return (
    <Section title={title}>
      <AutoSaveSwitch
        size="sm"
        label={label}
        description={help}
        checked={enabled}
        onSave={onSave}
        maw={560}
      />
    </Section>
  );
}

// webPushSubjectPattern は、Web Push の送り手の連絡先に書ける形。空白を含まない mailto: か https:// の URI (RFC 8292)。サーバーと同じ。
const webPushSubjectPattern = /^(mailto:|https:\/\/)\S+$/;

// WebPushSubjectSetting は、Web Push で、プッシュサービスに示す送り手の連絡先を変える (#1869)。
// 欄を離れたときか Enter で保存する (#1740)。空にすると、Kotatsu のリポジトリの URL に戻す。
function WebPushSubjectSetting({
  subject,
  onSave,
}: {
  subject: string;
  onSave: (subject: string) => Promise<boolean>;
}) {
  const { t } = useTranslation();

  return (
    <Section title={t('admin.settings.webPush')}>
      <AutoSaveTextInput
        size="xs"
        w="100%"
        maw={560}
        label={t('admin.settings.webPushSubject')}
        description={t('admin.settings.webPushSubjectHelp')}
        placeholder="mailto:admin@example.com"
        validate={(value) =>
          value === '' || webPushSubjectPattern.test(value)
            ? null
            : t('admin.settings.webPushSubjectInvalid')
        }
        value={subject}
        onSave={onSave}
      />
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
  onSave,
  children,
}: {
  enabled: boolean;
  imports: readonly AnimeImportItem[];
  onSave: (enabled: boolean) => Promise<boolean>;
  children?: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <Section title={t('admin.settings.anime')}>
      <Stack gap="sm">
        <AutoSaveSwitch
          size="sm"
          label={t('admin.settings.animeEnabled')}
          description={t('admin.settings.animeHelp')}
          checked={enabled}
          onSave={onSave}
          maw={560}
        />
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

type DramaTitleOverrideItem = {
  titleKey: string;
  tmdbSeriesId: number | null | undefined;
  updatedAt: string;
};

// DramaLinkingSettings は、TMDb との連携 (ドラマの番組と作品の結びつき) を使うかどうかを変え (#1651、設計書 3.9)、
// トークンの入力欄 tokenForm と、作品名ごとの上書きを出す。上書きは、番組名と TMDb の作品で足し、一覧から消せる。
function DramaLinkingSettings({
  enabled,
  tokenConfigured,
  tokenForm,
  overrides,
  onSave,
  onOverride,
}: {
  enabled: boolean;
  tokenConfigured: boolean;
  tokenForm: ReactNode;
  overrides: readonly DramaTitleOverrideItem[];
  onSave: (enabled: boolean) => Promise<boolean>;
  onOverride: (input: DramaTitleOverrideInput) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [programName, setProgramName] = useState('');
  const [series, setSeries] = useState('');
  const [saving, setSaving] = useState(false);
  const seriesId = parseTMDbSeriesId(series);
  const run = async (input: DramaTitleOverrideInput) => {
    setSaving(true);
    try {
      // 一覧から消したときは、入れかけの番組名と作品を残す。
      if ((await onOverride(input)) && input.programName != null) {
        setProgramName('');
        setSeries('');
      }
    } finally {
      setSaving(false);
    }
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (programName.trim() !== '' && seriesId !== null) {
      void run({ programName, tmdbSeriesId: seriesId });
    }
  };

  return (
    <Section title={t('admin.settings.drama')}>
      <Stack gap="sm">
        <AutoSaveSwitch
          size="sm"
          label={t('admin.settings.dramaEnabled')}
          description={t('admin.settings.dramaHelp')}
          checked={enabled}
          onSave={onSave}
          maw={560}
        />
        {enabled && !tokenConfigured && (
          <Alert color="amber" data-testid="drama-token-missing">
            {t('admin.settings.dramaTokenMissing')}
          </Alert>
        )}
        {tokenForm}
        <TMDbAttribution />
        <Title order={5}>{t('admin.settings.dramaOverrides')}</Title>
        <Text size="sm" c="dimmed">
          {t('admin.settings.dramaOverridesHelp')}
        </Text>
        {overrides.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.settings.dramaOverridesEmpty')}
          </Text>
        ) : (
          <Stack gap={4} data-testid="drama-overrides">
            {overrides.map((override) => (
              <Group key={override.titleKey} gap="xs" wrap="nowrap">
                <Text size="sm" fw={600} style={{ wordBreak: 'break-all' }}>
                  {override.titleKey}
                </Text>
                <Text size="sm">
                  {override.tmdbSeriesId == null ? (
                    t('admin.settings.dramaOverrideExcluded')
                  ) : (
                    <Anchor
                      href={`https://www.themoviedb.org/tv/${override.tmdbSeriesId}`}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {`TMDb ${override.tmdbSeriesId}`}
                    </Anchor>
                  )}
                </Text>
                <Button
                  size="compact-xs"
                  variant="subtle"
                  disabled={saving}
                  onClick={() => void run({ titleKey: override.titleKey, reset: true })}
                >
                  {t('admin.settings.dramaOverrideReset')}
                </Button>
              </Group>
            ))}
          </Stack>
        )}
        <form onSubmit={submit}>
          <Group align="flex-end" gap="xs" wrap="wrap">
            <TextInput
              size="xs"
              label={t('admin.settings.dramaOverrideProgram')}
              value={programName}
              onChange={(event) => setProgramName(event.currentTarget.value)}
              w={260}
            />
            <TextInput
              size="xs"
              label={t('admin.settings.dramaOverrideSeries')}
              placeholder="https://www.themoviedb.org/tv/123"
              value={series}
              onChange={(event) => setSeries(event.currentTarget.value)}
              error={series !== '' && seriesId === null ? t('drama.override.invalid') : undefined}
              w={260}
            />
            <Button
              size="compact-sm"
              type="submit"
              disabled={saving || programName.trim() === '' || seriesId === null}
            >
              {t('drama.override.save')}
            </Button>
            <Button
              size="compact-sm"
              variant="light"
              color="red"
              disabled={saving || programName.trim() === ''}
              onClick={() => void run({ programName, exclude: true })}
            >
              {t('drama.override.exclude')}
            </Button>
          </Group>
        </form>
      </Stack>
    </Section>
  );
}

// DramaTokenForm は、TMDb の API の読み取りアクセストークンを変える (#1651)。
// トークンはサーバーから返らないので、変えるときだけ入れる。入れたトークンは、末尾の数文字 hint だけで見分ける。
function DramaTokenForm({
  configured,
  hint,
  busy,
  onSave,
}: {
  configured: boolean;
  hint: string | null;
  busy: boolean;
  onSave: (token: string, onDone: () => void) => void;
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
            {t('admin.settings.dramaToken')}
          </Text>
          <Badge
            size="sm"
            variant="light"
            color={configured ? 'teal' : 'gray'}
            data-testid="drama-token"
          >
            {!configured
              ? t('admin.settings.dramaTokenNotConfigured')
              : hint === null
                ? t('admin.settings.dramaTokenConfigured')
                : t('admin.settings.dramaTokenConfiguredHint', { hint })}
          </Badge>
        </Group>
        <Text size="xs" c="dimmed" maw={560}>
          {t('admin.settings.dramaTokenHelp')}
        </Text>
        <Group align="flex-end" wrap="wrap">
          <PasswordInput
            size="sm"
            aria-label={t('admin.settings.dramaToken')}
            placeholder={configured ? t('admin.settings.dramaTokenKept') : undefined}
            value={token}
            onChange={(event) => setToken(event.currentTarget.value)}
            autoComplete="new-password"
            w={320}
          />
          <Button type="submit" size="compact-sm" disabled={token.trim() === '' || busy}>
            {t('admin.integrations.register')}
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
              {t('admin.settings.dramaTokenDelete')}
            </Button>
          )}
        </Group>
      </Stack>
    </form>
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
            {t('admin.integrations.register')}
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
            {t('admin.integrations.register')}
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
  onUpdate: (input: UpdateWebhookInput) => Promise<boolean>;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const knownFormat = formats.find((format) => format === webhook.format);

  if (editing) {
    return (
      <Stack gap={4} data-webhook={webhook.name}>
        <WebhookEditor webhook={webhook} onUpdate={onUpdate} />
        <Group>
          <Button size="compact-sm" variant="default" onClick={() => setEditing(false)}>
            {t('admin.webhooks.close')}
          </Button>
        </Group>
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

// WebhookEditor は、作った Webhook を変える欄。変えた項目だけを保存する (#1740)。名前と URL は、欄を離れたときか Enter で、
// 形式、種類、送るかどうかは、変えたらすぐ保存する。種類は 1 つ以上選ぶ。
function WebhookEditor({
  webhook,
  onUpdate,
}: {
  webhook: WebhookItem;
  onUpdate: (input: UpdateWebhookInput) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [selectedKinds, changeKinds] = useAutoSavedValue<readonly NotificationKindName[]>(
    knownKinds(webhook.kinds),
    (next) => onUpdate({ kinds: [...next] }),
    (a, b) => a.length === b.length && a.every((kind) => b.includes(kind)),
  );

  return (
    <Stack gap="xs">
      <Group gap="xs" align="flex-start">
        <AutoSaveTextInput
          size="xs"
          label={t('admin.webhooks.name')}
          required
          maxLength={100}
          value={webhook.name}
          onSave={(name) => onUpdate({ name })}
        />
        <AutoSaveSelect
          size="xs"
          w={160}
          label={t('admin.webhooks.format')}
          data={formats.map((format) => ({
            value: format,
            label: t(`admin.webhooks.formats.${format}`),
          }))}
          value={webhook.format}
          onSave={(value) => {
            const format = formats.find((known) => known === value);

            return format === undefined ? Promise.resolve(false) : onUpdate({ format });
          }}
        />
      </Group>
      <AutoSaveTextInput
        size="xs"
        type="url"
        label={t('admin.webhooks.url')}
        description={t('admin.webhooks.urlHelp')}
        required
        autoComplete="off"
        value={webhook.url}
        onSave={(url) => onUpdate({ url })}
      />
      <Checkbox.Group
        size="xs"
        label={t('admin.webhooks.kinds')}
        value={[...selectedKinds]}
        onChange={(values) => {
          const next = knownKinds(values);
          // 種類をすべて外すと送る先がなくなるので、保存しない。
          if (next.length > 0) {
            changeKinds(next);
          }
        }}
      >
        <Group gap="sm" mt={4}>
          {kinds.map((kind) => (
            <Checkbox key={kind} value={kind} label={t(notificationKinds[kind])} />
          ))}
        </Group>
      </Checkbox.Group>
      <AutoSaveSwitch
        size="xs"
        label={t('admin.webhooks.enabled')}
        checked={webhook.enabled}
        onSave={(enabled) => onUpdate({ enabled })}
      />
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
