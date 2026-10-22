import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Code,
  Fieldset,
  Group,
  Image,
  PinInput,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
  Select,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import { renderSVG } from 'uqr';

import { authApi, type OIDCStatus } from '../../auth/api';
import { oidcErrorMessage } from '../../auth/messages';
import { settingsErrorMessage } from '../../settings/errors';
import { PasskeyCancelledError, passkeyAvailable, registerPasskey } from '../../auth/passkey';
import { ApiTokens } from '../../settings/ApiTokens';
import { OAuthGrants } from '../../settings/OAuthGrants';
import { deviceName } from '../../settings/device';
import { ErrorBoundary, PageLoader } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import {
  autoPictureInPictureSupported,
  useAutoPictureInPicture,
} from '../../yunomi/autoPictureInPicture';
import { GuideAppearanceSettings } from '../../guide/GuideAppearanceSettings';
import { CommentSettingsCard } from '../../jikkyo/CommentSettingsCard';
import { useDataBroadcastPreload } from '../../yunomi/dataBroadcastPreload';
import { captureFileName, captureFileNameMacros } from '../../yunomi/capture';
import { useCaptureFileNameTemplate } from '../../yunomi/captureFileNameTemplate';
import { useLowLatency } from '../../yunomi/liveCatchUp';
import { DataBroadcastStorageReset } from '../../yunomi/DataBroadcastStorageReset';
import { DataBroadcastZipcodeInput } from '../../yunomi/DataBroadcastZipcodeInput';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import type { accountBeginTotpMutation } from './__generated__/accountBeginTotpMutation.graphql';
import type { accountConfirmTotpMutation } from './__generated__/accountConfirmTotpMutation.graphql';
import type { accountDeletePasskeyMutation } from './__generated__/accountDeletePasskeyMutation.graphql';
import type { accountDeleteTotpMutation } from './__generated__/accountDeleteTotpMutation.graphql';
import type { accountQuery } from './__generated__/accountQuery.graphql';
import type { accountRegenerateMutation } from './__generated__/accountRegenerateMutation.graphql';
import type { accountRenamePasskeyMutation } from './__generated__/accountRenamePasskeyMutation.graphql';
import type { accountRevokeSessionMutation } from './__generated__/accountRevokeSessionMutation.graphql';
import { zonedFormat } from '../../futon';
import { useDataChanges } from '../../changes/DataChanges';

type AccountSearch = {
  // error は、IdP での再認証か紐付けに失敗して戻ってきたときの理由 (oidc_link_taken など)。
  error?: string;
  // linked は、IdP との紐付けを終えて戻ってきたときに oidc になる。
  linked?: 'oidc';
};

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/settings/account';

// captionBackgrounds は、字幕の背景の選択肢 (#491)。broadcast は放送の指定どおり、ほかは黒の不透明度。
const captionBackgrounds = ['broadcast', '0', '0.25', '0.5', '0.75', '1'];

// captureExampleTime は、キャプチャのファイル名の例に使う時刻。ファイル名は端末のローカル時刻で書くので、ローカル時刻で作る。
const captureExampleTime = new Date(2026, 9, 16, 21, 0, 5).getTime();

export const Route = createFileRoute('/settings/account')({
  validateSearch: (search: Record<string, unknown>): AccountSearch => ({
    ...(typeof search['error'] === 'string' && search['error'] !== ''
      ? { error: search['error'] }
      : {}),
    ...(search['linked'] === 'oidc' ? { linked: 'oidc' as const } : {}),
  }),
  // IdP での再認証と紐付けを出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: AccountPage,
});

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// AccountPage は、ログイン中のユーザーの個人の設定 (設計書 10.3)。プレイヤー、セッション、パスキー、第二要素を扱う。
function AccountPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.account.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Account />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

const query = graphql`
  query accountQuery {
    secondFactors {
      totp
      remainingRecoveryCodes
    }
    passkeys {
      id
      name
      createdAt
      lastUsedAt
    }
    apiTokens {
      id
      name
      createdAt
      lastUsedAt
    }
    oauthGrants {
      id
      clientId
      clientName
      scopes
      createdAt
      lastUsedAt
    }
    sessions {
      id
      current
      createdAt
      lastSeenAt
      userAgent
      sourceIp
    }
  }
`;

function Account() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  const search = Route.useSearch();
  // fetchKey を変えると、読み直す。登録や削除の後に、一覧を新しくするのに使う。
  // 読み直す間も画面を出したままにして、作り直したリカバリーコードなどの表示を消さないよう、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの端末で設定を変えたら、読み直す (#503)。
  const viewerChanges = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<accountQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${viewerChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const gate = useReauthGate(reauthMethods(data.secondFactors, data.passkeys, oidc), path);
  const player = usePlayerSettings();
  const [dataBroadcastPreload, setDataBroadcastPreload] = useDataBroadcastPreload();
  const [captureTemplate, setCaptureTemplate] = useCaptureFileNameTemplate();
  const [lowLatency, setLowLatency] = useLowLatency();
  const [autoPictureInPicture, setAutoPictureInPicture] = useAutoPictureInPicture();
  // 自動で別のウィンドウに出せないブラウザでは、設定の項目を出さない (#492)。
  const [autoPictureInPictureAvailable] = useState(autoPictureInPictureSupported);
  const [error, setError] = useState<string | null>(
    search.error === undefined ? null : oidcErrorMessage(t, search.error),
  );
  const [linked, setLinked] = useState(search.linked === 'oidc');
  const fail = (caught: unknown) => setError(settingsErrorMessage(t, caught));
  // run は、前の操作のエラーを消してから、操作を行う。
  const run: Run = (operation, onError) => {
    setError(null);
    gate.run(operation, onError);
  };

  return (
    <Stack gap="md">
      {gate.modal}
      {error !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {linked && (
        <Alert color="green" role="status" withCloseButton onClose={() => setLinked(false)}>
          {t('settings.account.oidcLinked', { name: oidc.displayName })}
        </Alert>
      )}

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('settings.account.player')}
          </Title>
          <Switch
            label={t('settings.account.jikkyo')}
            checked={player.jikkyoEnabled}
            onChange={(event) => player.setJikkyoEnabled(event.currentTarget.checked)}
          />
          <Switch
            label={t('settings.account.captions')}
            checked={player.captionsEnabled}
            onChange={(event) => player.setCaptionsEnabled(event.currentTarget.checked)}
          />
          {/* 字幕の見た目と、文字スーパーを出すかどうか (#491)。字幕を出さなければ、どれも効かない。 */}
          <Stack gap="xs" pl="lg">
            <Switch
              size="sm"
              label={t('settings.account.superimposeLive')}
              disabled={!player.captionsEnabled}
              checked={player.captions.superimposeLive}
              onChange={(event) =>
                player.setCaptions({
                  ...player.captions,
                  superimposeLive: event.currentTarget.checked,
                })
              }
            />
            <Switch
              size="sm"
              label={t('settings.account.superimposeRecorded')}
              disabled={!player.captionsEnabled}
              checked={player.captions.superimposeRecorded}
              onChange={(event) =>
                player.setCaptions({
                  ...player.captions,
                  superimposeRecorded: event.currentTarget.checked,
                })
              }
            />
            <Switch
              size="sm"
              label={t('settings.account.captionStroke')}
              disabled={!player.captionsEnabled}
              checked={player.captions.stroke}
              onChange={(event) =>
                player.setCaptions({ ...player.captions, stroke: event.currentTarget.checked })
              }
            />
            <Select
              size="xs"
              label={t('settings.account.captionBackground')}
              disabled={!player.captionsEnabled}
              allowDeselect={false}
              data={captionBackgrounds.map((value) => ({
                value,
                label:
                  value === 'broadcast'
                    ? t('settings.account.captionBackgroundBroadcast')
                    : `${Math.round(Number(value) * 100)}%`,
              }))}
              value={
                player.captions.backgroundOpacity === null
                  ? 'broadcast'
                  : String(player.captions.backgroundOpacity)
              }
              onChange={(value) =>
                value !== null &&
                player.setCaptions({
                  ...player.captions,
                  backgroundOpacity: value === 'broadcast' ? null : Number(value),
                })
              }
              maw={240}
            />
          </Stack>
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.dataBroadcastPreload')}
                {/* アカウントの設定と並ぶので、この端末だけの設定であることを示す。 */}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.dataBroadcastPreloadHelp')}
            checked={dataBroadcastPreload}
            onChange={(event) => setDataBroadcastPreload(event.currentTarget.checked)}
          />
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.lowLatency')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.lowLatencyHelp')}
            checked={lowLatency}
            onChange={(event) => setLowLatency(event.currentTarget.checked)}
          />
          <TextInput
            label={
              <Group gap={6} component="span">
                {t('settings.account.captureFileName')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.captureFileNameHelp', {
              macros: captureFileNameMacros.join(' '),
              example: captureFileName(
                captureExampleTime,
                'NHK総合1・東京',
                'ニュース７🈑',
                captureTemplate,
              ),
            })}
            placeholder={t('settings.account.captureFileNameDefault')}
            value={captureTemplate}
            onChange={(event) => setCaptureTemplate(event.currentTarget.value)}
            maw={480}
          />
          <DataBroadcastZipcodeInput />
          <DataBroadcastStorageReset />
          {autoPictureInPictureAvailable && (
            <Switch
              label={
                <Group gap={6} component="span">
                  {t('settings.account.autoPictureInPicture')}
                  <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                    {t('settings.account.deviceOnly')}
                  </Badge>
                </Group>
              }
              description={t('settings.account.autoPictureInPictureHelp')}
              checked={autoPictureInPicture}
              onChange={(event) => setAutoPictureInPicture(event.currentTarget.checked)}
            />
          )}
        </Stack>
      </Card>

      <CommentSettingsCard />

      <GuideAppearanceSettings />

      {/* 操作の途中は、ほかのボタンを押せないようにする。 */}
      <Fieldset variant="unstyled" disabled={gate.busy} p={0} m={0} miw={0}>
        <Stack gap="md">
          <TwoFactor
            totp={data.secondFactors.totp}
            remaining={data.secondFactors.remainingRecoveryCodes}
            onChanged={refresh}
            run={run}
            fail={fail}
            environment={environment}
          />

          <Passkeys
            passkeys={data.passkeys}
            onChanged={refresh}
            run={run}
            fail={fail}
            environment={environment}
          />

          <ApiTokens
            tokens={data.apiTokens}
            onChanged={refresh}
            run={run}
            fail={fail}
            environment={environment}
          />

          <OAuthGrants
            grants={data.oauthGrants}
            onChanged={refresh}
            run={run}
            fail={fail}
            environment={environment}
          />

          {/* 紐付いていないときだけ出す。紐付けを外す操作は、パスワードもパスキーもないユーザーを締め出しかねないので設けない。 */}
          {oidc.enabled && !oidc.linked && (
            <OIDCLink displayName={oidc.displayName} run={run} fail={fail} />
          )}

          <Card withBorder>
            <Stack gap="xs">
              <Title order={3} size="h5">
                {t('settings.account.sessions')}
              </Title>
              {data.sessions.map((session) => (
                <Group
                  key={session.id}
                  justify="space-between"
                  wrap="nowrap"
                  data-session={session.id}
                >
                  <Stack gap={0} miw={0} flex={1}>
                    <Group gap="xs">
                      <Text size="sm" truncate title={session.userAgent}>
                        {deviceName(session.userAgent) ??
                          (session.userAgent || t('settings.account.unknownDevice'))}
                      </Text>
                      {session.current && (
                        <Badge size="xs" variant="light">
                          {t('settings.account.thisSession')}
                        </Badge>
                      )}
                    </Group>
                    <Text size="xs" c="dimmed">
                      {t('settings.account.sessionTimes', {
                        lastSeen: dateTimeFormat.format(Date.parse(session.lastSeenAt)),
                        source: session.sourceIp ?? '-',
                      })}
                    </Text>
                  </Stack>
                  {!session.current && (
                    <Button
                      size="compact-sm"
                      variant="default"
                      flex="none"
                      onClick={() =>
                        run(async () => {
                          await mutate<accountRevokeSessionMutation>(
                            environment,
                            graphql`
                              mutation accountRevokeSessionMutation($id: ID!) {
                                revokeSession(id: $id)
                              }
                            `,
                            { id: session.id },
                          );
                          refresh();
                        }, fail)
                      }
                    >
                      {t('settings.account.revoke')}
                    </Button>
                  )}
                </Group>
              ))}
            </Stack>
          </Card>
        </Stack>
      </Fieldset>

      <Card withBorder>
        <Group justify="space-between" wrap="wrap">
          <Title order={3} size="h5">
            {t('auth.password')}
          </Title>
          <Button
            variant="default"
            size="xs"
            renderRoot={(props) => <Link {...props} to="/settings/password" />}
          >
            {t('viewer.changePassword')}
          </Button>
        </Group>
      </Card>
    </Stack>
  );
}

type Run = ReturnType<typeof useReauthGate>['run'];

// OIDCLink は、ログイン中のユーザーを IdP のユーザーと紐付ける (設計書 9.4)。再認証してから、IdP へ送り出す。
// IdP から戻ると、この画面に linked=oidc か error=<理由> が付く。
function OIDCLink({
  displayName,
  run,
  fail,
}: {
  displayName: string;
  run: Run;
  fail: (error: unknown) => void;
}) {
  const { t } = useTranslation();

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.oidc', { name: displayName })}
        </Title>
        <Text size="sm">{t('settings.account.oidcDescription', { name: displayName })}</Text>
        <Group gap="xs">
          <Button
            size="compact-sm"
            onClick={() =>
              run(async () => {
                const { authorizationUrl } = await authApi.beginOIDCLink(path);
                window.location.assign(authorizationUrl);
              }, fail)
            }
          >
            {t('settings.account.oidcLink', { name: displayName })}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}

type SectionProps = {
  onChanged: () => void;
  run: Run;
  fail: (error: unknown) => void;
  environment: ReturnType<typeof useRelayEnvironment>;
};

// TwoFactor は、認証アプリ (TOTP) とリカバリーコード。登録と削除、リカバリーコードの作り直しは、再認証してから行う。
function TwoFactor({
  totp,
  remaining,
  onChanged,
  run,
  fail,
  environment,
}: SectionProps & { totp: boolean; remaining: number }) {
  const { t } = useTranslation();
  const [enrollment, setEnrollment] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<readonly string[] | null>(null);

  const begin = () =>
    run(async () => {
      const response = await mutate<accountBeginTotpMutation>(
        environment,
        graphql`
          mutation accountBeginTotpMutation {
            beginTotpEnrollment {
              secret
              uri
            }
          }
        `,
        {},
      );
      setEnrollment(response.beginTotpEnrollment);
      setCode('');
      setCodes(null);
    }, fail);
  const confirm = () =>
    run(async () => {
      const response = await mutate<accountConfirmTotpMutation>(
        environment,
        graphql`
          mutation accountConfirmTotpMutation($code: String!) {
            confirmTotpEnrollment(code: $code) {
              codes
            }
          }
        `,
        { code },
      );
      setEnrollment(null);
      setCodes(response.confirmTotpEnrollment.codes);
      onChanged();
    }, fail);
  const remove = () =>
    run(async () => {
      await mutate<accountDeleteTotpMutation>(
        environment,
        graphql`
          mutation accountDeleteTotpMutation {
            deleteTotp {
              totp
            }
          }
        `,
        {},
      );
      // 認証アプリの登録を消すと、リカバリーコードも使えなくなるので、表示していたものを消す。
      setCodes(null);
      onChanged();
    }, fail);
  const regenerate = () =>
    run(async () => {
      const response = await mutate<accountRegenerateMutation>(
        environment,
        graphql`
          mutation accountRegenerateMutation {
            regenerateRecoveryCodes {
              codes
            }
          }
        `,
        {},
      );
      setCodes(response.regenerateRecoveryCodes.codes);
      onChanged();
    }, fail);

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.twoFactor')}
        </Title>
        <Text size="sm">
          {totp
            ? t('settings.account.totpEnabled', { count: remaining })
            : t('settings.account.totpDisabled')}
        </Text>
        <Group gap="xs">
          {totp ? (
            <>
              <Button size="compact-sm" variant="default" onClick={regenerate}>
                {t('settings.account.regenerate')}
              </Button>
              <Button size="compact-sm" variant="default" color="red" onClick={remove}>
                {t('settings.account.deleteTotp')}
              </Button>
            </>
          ) : (
            enrollment === null && (
              <Button size="compact-sm" onClick={begin}>
                {t('settings.account.enrollTotp')}
              </Button>
            )
          )}
        </Group>
        {enrollment !== null && (
          <Stack gap="xs">
            <Text size="sm">{t('auth.enroll.description')}</Text>
            <Image
              src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(renderSVG(enrollment.uri))}`}
              alt={t('auth.enroll.qr')}
              w={180}
              h={180}
            />
            <Code>{enrollment.secret}</Code>
            <PinInput
              length={6}
              type="number"
              oneTimeCode
              value={code}
              onChange={setCode}
              aria-label={t('auth.enroll.code')}
            />
            <Group gap="xs">
              <Button size="compact-sm" disabled={code.length !== 6} onClick={confirm}>
                {t('settings.account.confirmTotp')}
              </Button>
              <Button size="compact-sm" variant="subtle" onClick={() => setEnrollment(null)}>
                {t('settings.password.cancel')}
              </Button>
            </Group>
          </Stack>
        )}
        {codes !== null && (
          <Alert color="amber" title={t('settings.account.recoveryCodes')}>
            <Text size="sm">{t('settings.account.recoveryCodesNote')}</Text>
            <SimpleGrid cols={2} spacing={4} mt="xs">
              {codes.map((recoveryCode) => (
                <Code key={recoveryCode}>{recoveryCode}</Code>
              ))}
            </SimpleGrid>
          </Alert>
        )}
      </Stack>
    </Card>
  );
}

type PasskeyItem = {
  id: string;
  name: string;
  createdAt: string;
  lastUsedAt: string | null | undefined;
};

// Passkeys は、パスキーの一覧と、名前の変更、削除、登録。削除と登録は、再認証してから行う。
function Passkeys({
  passkeys,
  onChanged,
  run,
  fail,
  environment,
}: SectionProps & { passkeys: readonly PasskeyItem[] }) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(null);
  const available = passkeyAvailable();

  const add = () =>
    run(
      async () => {
        await registerPasskey(name.trim());
        setName('');
        onChanged();
      },
      (caught) => {
        // 認証器の画面を閉じただけなら、何も示さない。
        if (!(caught instanceof PasskeyCancelledError)) {
          fail(caught);
        }
      },
    );

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.passkeys')}
        </Title>
        {passkeys.length === 0 && (
          <Text size="sm" c="dimmed">
            {t('settings.account.noPasskeys')}
          </Text>
        )}
        {passkeys.map((passkey) => (
          <Group key={passkey.id} justify="space-between" wrap="nowrap">
            {editing?.id === passkey.id ? (
              <TextInput
                size="xs"
                miw={0}
                flex={1}
                aria-label={t('settings.account.passkeyName')}
                value={editing.name}
                onChange={(event) =>
                  setEditing({ id: passkey.id, name: event.currentTarget.value })
                }
              />
            ) : (
              <Stack gap={0} miw={0} flex={1}>
                <Text size="sm" truncate>
                  {passkey.name}
                </Text>
                <Text size="xs" c="dimmed">
                  {t('settings.account.passkeyTimes', {
                    created: dateTimeFormat.format(Date.parse(passkey.createdAt)),
                    lastUsed:
                      passkey.lastUsedAt == null
                        ? '-'
                        : dateTimeFormat.format(Date.parse(passkey.lastUsedAt)),
                  })}
                </Text>
              </Stack>
            )}
            <Group gap="xs" wrap="nowrap" flex="none">
              {editing?.id === passkey.id ? (
                <Button
                  size="compact-sm"
                  disabled={editing.name.trim() === ''}
                  onClick={() =>
                    run(async () => {
                      await mutate<accountRenamePasskeyMutation>(
                        environment,
                        graphql`
                          mutation accountRenamePasskeyMutation($id: ID!, $name: String!) {
                            renamePasskey(id: $id, name: $name) {
                              id
                              name
                            }
                          }
                        `,
                        { id: passkey.id, name: editing.name.trim() },
                      );
                      setEditing(null);
                      onChanged();
                    }, fail)
                  }
                >
                  {t('settings.account.save')}
                </Button>
              ) : null}
              {editing?.id === passkey.id ? (
                <Button size="compact-sm" variant="subtle" onClick={() => setEditing(null)}>
                  {t('settings.password.cancel')}
                </Button>
              ) : (
                <Button
                  size="compact-sm"
                  variant="default"
                  onClick={() => setEditing({ id: passkey.id, name: passkey.name })}
                >
                  {t('settings.account.rename')}
                </Button>
              )}
              <Button
                size="compact-sm"
                variant="default"
                color="red"
                onClick={() =>
                  run(async () => {
                    await mutate<accountDeletePasskeyMutation>(
                      environment,
                      graphql`
                        mutation accountDeletePasskeyMutation($id: ID!) {
                          deletePasskey(id: $id)
                        }
                      `,
                      { id: passkey.id },
                    );
                    onChanged();
                  }, fail)
                }
              >
                {t('settings.account.delete')}
              </Button>
            </Group>
          </Group>
        ))}
        {available ? (
          <Group gap="xs" align="flex-end">
            <TextInput
              size="xs"
              label={t('settings.account.newPasskeyName')}
              placeholder={t('settings.account.newPasskeyPlaceholder')}
              value={name}
              onChange={(event) => setName(event.currentTarget.value)}
            />
            <Button size="compact-sm" disabled={name.trim() === ''} onClick={add}>
              {t('settings.account.addPasskey')}
            </Button>
          </Group>
        ) : (
          <Text size="xs" c="dimmed">
            {t('settings.account.passkeyUnavailable')}
          </Text>
        )}
      </Stack>
    </Card>
  );
}
