import {
  Alert,
  Anchor,
  Button,
  Card,
  Code,
  Fieldset,
  Group,
  Image,
  PinInput,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import { renderSVG } from 'uqr';

import { authApi, type OIDCStatus } from '../../auth/api';
import { settingsErrorMessage } from '../../settings/errors';
import { PasskeyCancelledError, passkeyAvailable, registerPasskey } from '../../auth/passkey';
import { ApiTokens } from '../../settings/ApiTokens';
import { ErrorBoundary, PageLoader } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { SudoGate } from '../../settings/SudoGate';
import type { securityBeginTotpMutation } from './__generated__/securityBeginTotpMutation.graphql';
import type { securityConfirmTotpMutation } from './__generated__/securityConfirmTotpMutation.graphql';
import type { securityDeletePasskeyMutation } from './__generated__/securityDeletePasskeyMutation.graphql';
import type { securityDeleteTotpMutation } from './__generated__/securityDeleteTotpMutation.graphql';
import type { securityQuery } from './__generated__/securityQuery.graphql';
import type { securityRegenerateMutation } from './__generated__/securityRegenerateMutation.graphql';
import type { securityRenamePasskeyMutation } from './__generated__/securityRenamePasskeyMutation.graphql';
import { zonedFormat } from '../../futon';
import { useDataChanges } from '../../changes/DataChanges';

// IdP での再認証か紐付けに失敗して戻ってきたときの理由 (error=oidc_link_taken など) は、useReauthGate が出す。
type SecuritySearch = {
  // linked は、IdP との紐付けを終えて戻ってきたときに oidc になる。
  linked?: 'oidc';
};

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/settings/security';

export const Route = createFileRoute('/settings/security')({
  validateSearch: (search: Record<string, unknown>): SecuritySearch =>
    search['linked'] === 'oidc' ? { linked: 'oidc' } : {},
  // IdP での再認証と紐付けを出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: SecurityPage,
});

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// SecurityPage は、ログイン中のユーザーの、本人確認の手段と、ほかから使うための手段の設定 (設計書 10.3、#980)。
// パスワード、認証アプリ、パスキー、IdP との紐付け、API トークンを扱う。
function SecurityPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.pages.security.title')}</Title>
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
          <Security />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

const query = graphql`
  query securityQuery {
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
  }
`;

function Security() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  const search = Route.useSearch();
  // fetchKey を変えると、読み直す。登録や削除の後に、一覧を新しくするのに使う。
  // 読み直す間も画面を出したままにして、作り直したリカバリーコードなどの表示を消さないよう、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの端末で設定を変えたら、読み直す (#503)。
  const viewerChanges = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<securityQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${viewerChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const methods = reauthMethods(data.secondFactors, data.passkeys, oidc);
  const gate = useReauthGate(methods, path);
  const [error, setError] = useState<string | null>(null);
  const [linked, setLinked] = useState(search.linked === 'oidc');
  const fail = (caught: unknown) => setError(settingsErrorMessage(t, caught));
  // run は、前の操作のエラーを消してから、操作を行う。
  const run: Run = (operation, onError, onCancel, onWaiting) => {
    setError(null);
    gate.run(operation, onError, onCancel, onWaiting);
  };

  return (
    <Stack gap="md">
      {gate.modal}
      <SudoGate methods={methods} returnTo={path}>
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

            {/* 紐付いていないときだけ出す。紐付けを外す操作は、パスワードもパスキーもないユーザーを締め出しかねないので設けない。 */}
            {oidc.enabled && !oidc.linked && (
              <OIDCLink displayName={oidc.displayName} run={run} fail={fail} />
            )}
          </Stack>
        </Fieldset>
      </SudoGate>
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
  const link = () =>
    run(async () => {
      const { authorizationUrl } = await authApi.beginOIDCLink(path);
      window.location.assign(authorizationUrl);
    }, fail);

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.oidc', { name: displayName })}
        </Title>
        <Text size="sm">{t('settings.account.oidcDescription', { name: displayName })}</Text>
        <Group gap="xs">
          <Button size="compact-sm" onClick={link}>
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
      const response = await mutate<securityBeginTotpMutation>(
        environment,
        graphql`
          mutation securityBeginTotpMutation {
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
      const response = await mutate<securityConfirmTotpMutation>(
        environment,
        graphql`
          mutation securityConfirmTotpMutation($code: String!) {
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
      await mutate<securityDeleteTotpMutation>(
        environment,
        graphql`
          mutation securityDeleteTotpMutation {
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
      const response = await mutate<securityRegenerateMutation>(
        environment,
        graphql`
          mutation securityRegenerateMutation {
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

  const remove = (id: string) =>
    run(async () => {
      await mutate<securityDeletePasskeyMutation>(
        environment,
        graphql`
          mutation securityDeletePasskeyMutation($id: ID!) {
            deletePasskey(id: $id)
          }
        `,
        { id },
      );
      onChanged();
    }, fail);

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
                      await mutate<securityRenamePasskeyMutation>(
                        environment,
                        graphql`
                          mutation securityRenamePasskeyMutation($id: ID!, $name: String!) {
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
                onClick={() => remove(passkey.id)}
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
