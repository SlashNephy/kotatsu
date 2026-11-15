import { Alert, Button, Code, Group, Stack, Text, TextInput } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { authApi, type OIDCStatus } from '../auth/api';
import { AuthCard } from '../auth/AuthCard';
import { rememberReturnTo } from '../auth/returnTo';
import { normalizeUserCode } from '../auth/userCode';
import { ErrorBoundary, PageLoader } from '../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../relay/environment';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import { reauthMethods, useReauthGate } from '../settings/useReauthGate';
import { SudoGate } from '../settings/SudoGate';
import type { deviceApproveMutation } from './__generated__/deviceApproveMutation.graphql';
import type { deviceQuery } from './__generated__/deviceQuery.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };

type DeviceSearch = {
  // user_code は、端末が QR コードで渡す、コード付きの確認の URL のコード。手で打つときは無い。
  user_code?: string;
};

// 承認の画面 (#1527、設計書 9.9)。キーボードの無い端末が出した QR コードか URL を、ログイン済みのスマートフォンで開き、
// 端末の名前と user_code を見比べてから、ボタンで承認する。開いただけでは承認しない。ブラウザの先読みやリンクの下見で、
// 黙って承認されないためである。名前は端末が自分で名乗るものなので、見比べてもらうのは user_code である (RFC 8628 5.4)。
export const Route = createFileRoute('/device')({
  validateSearch: (search: Record<string, unknown>): DeviceSearch =>
    typeof search.user_code === 'string' ? { user_code: search.user_code } : {},
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: DevicePage,
});

// devicePath は、user_code の承認の画面のパス。ログインと再認証を終えたら、ここへ戻す。
function devicePath(userCode: string): string {
  return `/device?${new URLSearchParams({ user_code: userCode }).toString()}`;
}

function DevicePage() {
  const { t } = useTranslation();
  const search = Route.useSearch();
  const userCode = search.user_code === undefined ? null : normalizeUserCode(search.user_code);

  return (
    <AuthCard title={t('device.title')}>
      {userCode === null ? (
        <UserCodeForm
          // 打ち直すたびに、入力と誤りの表示を作り直す。
          key={search.user_code ?? ''}
          initial={search.user_code ?? ''}
          invalid={search.user_code !== undefined}
        />
      ) : (
        <ErrorBoundary
          resetKey={userCode}
          fallback={(error) => <DeviceError error={error} userCode={userCode} />}
        >
          <Suspense fallback={<PageLoader />}>
            {/* コードが変わったら、承認済みの表示と本人確認を作り直す。 */}
            <Confirm key={userCode} userCode={userCode} />
          </Suspense>
        </ErrorBoundary>
      )}
    </AuthCard>
  );
}

// UserCodeForm は、端末の画面に出ているコードを手で打ってもらう。打ったコードは、画面の URL に載せて確かめる。
function UserCodeForm({ initial, invalid }: { initial: string; invalid: boolean }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [value, setValue] = useState(initial);
  const [error, setError] = useState<string | null>(invalid ? t('device.invalidCode') : null);

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        const normalized = normalizeUserCode(value);
        if (normalized === null) {
          setError(t('device.invalidCode'));

          return;
        }
        void navigate({ to: '/device', search: { user_code: normalized } });
      }}
    >
      <Stack>
        <Text size="sm">{t('device.enterCode')}</Text>
        <TextInput
          label={t('device.code')}
          placeholder="XXXX-XXXX"
          autoComplete="one-time-code"
          autoCapitalize="characters"
          spellCheck={false}
          value={value}
          error={error}
          onChange={(event) => {
            setValue(event.currentTarget.value);
            setError(null);
          }}
        />
        <Group justify="flex-end">
          <Button type="submit">{t('device.next')}</Button>
        </Group>
      </Stack>
    </form>
  );
}

// DeviceError は、コードを引けなかったときの画面。ログインしていなければ、ログインの画面へ移り、終えたらこの画面へ戻る。
function DeviceError({ error, userCode }: { error: unknown; userCode: string }) {
  const { t } = useTranslation();
  const unauthenticated = isUnauthenticated(error);
  useEffect(() => {
    if (unauthenticated) {
      rememberReturnTo(devicePath(userCode));
      window.location.assign(`${import.meta.env.BASE_URL}login`);
    }
  }, [unauthenticated, userCode]);
  if (unauthenticated) {
    return <PageLoader />;
  }
  const { code } = graphQLErrorDetails(error);

  return (
    <Alert color="red" role="alert">
      {code === 'NOT_FOUND' ? t('device.disabled') : t('viewer.error')}
    </Alert>
  );
}

function Confirm({ userCode }: { userCode: string }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const navigate = useNavigate();
  const oidc = Route.useLoaderData();
  const data = useLazyLoadQuery<deviceQuery>(
    graphql`
      query deviceQuery($userCode: String!) {
        deviceAuthorization(userCode: $userCode) {
          deviceName
          userCode
          approved
        }
        secondFactors {
          totp
          remainingRecoveryCodes
        }
        passkeys {
          id
        }
      }
    `,
    { userCode },
    { fetchPolicy: 'network-only' },
  );
  const returnTo = devicePath(userCode);
  const methods = reauthMethods(data.secondFactors, data.passkeys, oidc);
  const gate = useReauthGate(methods, returnTo);
  const device = data.deviceAuthorization;
  const [approved, setApproved] = useState(device?.approved ?? false);
  const [error, setError] = useState<string | null>(null);
  const retry = () => void navigate({ to: '/device', search: {} });

  if (device == null) {
    return (
      <Stack>
        <Alert color="red" role="alert">
          {t('device.notFound', { code: userCode })}
        </Alert>
        <Group justify="flex-end">
          <Button variant="default" onClick={retry}>
            {t('device.enterAgain')}
          </Button>
        </Group>
      </Stack>
    );
  }

  const approve = () =>
    gate.run(
      async () => {
        const response = await mutate<deviceApproveMutation>(
          environment,
          graphql`
            mutation deviceApproveMutation($userCode: String!) {
              approveDeviceAuthorization(userCode: $userCode) {
                approved
              }
            }
          `,
          { userCode },
        );
        setApproved(response.approveDeviceAuthorization.approved);
      },
      (caught) =>
        setError(
          graphQLErrorDetails(caught).code === 'CONFLICT'
            ? t('device.approvedByOther')
            : settingsErrorMessage(t, caught),
        ),
    );

  return (
    <Stack>
      {gate.modal}
      <Stack gap={4}>
        <Text fw={700} size="lg">
          {device.deviceName === '' ? t('device.unnamed') : device.deviceName}
        </Text>
        <Code fz="xl" fw={700} ta="center" p="sm" aria-label={t('device.code')}>
          {device.userCode}
        </Code>
      </Stack>
      {approved ? (
        <Alert color="green" role="status">
          {t('device.approved')}
        </Alert>
      ) : (
        <>
          <Text size="sm">{t('device.description')}</Text>
          {/* どの端末への承認かは、本人確認の前にも出しておく。 */}
          <SudoGate methods={methods} returnTo={returnTo}>
            <Stack>
              {error !== null && (
                <Alert color="red" role="alert">
                  {error}
                </Alert>
              )}
              <Group justify="flex-end">
                <Button variant="default" onClick={retry} disabled={gate.busy}>
                  {t('device.cancel')}
                </Button>
                <Button onClick={approve} loading={gate.busy}>
                  {t('device.approve')}
                </Button>
              </Group>
            </Stack>
          </SudoGate>
        </>
      )}
    </Stack>
  );
}
