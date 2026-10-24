import {
  Alert,
  Anchor,
  Button,
  Divider,
  PasswordInput,
  PinInput,
  Stack,
  Text,
  TextInput,
} from '@mantine/core';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi, type OIDCStatus } from './api';
import { authErrorMessage } from './messages';
import { passkeyAvailable, verifyWithPasskey } from './passkey';

// ReauthenticationMethods は、ユーザーが再認証に使える方法。
export type ReauthenticationMethods = {
  // totp は、認証アプリを登録しているかどうか。
  totp: boolean;
  // recoveryCode は、使っていないリカバリーコードがあるかどうか。
  recoveryCode: boolean;
  passkey: boolean;
  oidc: OIDCStatus;
};

type Props = {
  methods: ReauthenticationMethods;
  // returnTo は、IdP で再認証したときに戻す、この画面のパス。
  returnTo: string;
  // description は、なぜ確かめるのかの説明。省くと、操作の前に確かめる説明を出す。
  description?: string;
  onDone: () => void;
};

// Reauthenticate は、パスワードの変更のような重要な操作の前に、ログイン中のユーザーをもう一度確かめる (設計書 9.3)。
// パスワードと第二要素、パスキー、IdP のうち、ユーザーが使えるものを出す。
export function Reauthenticate({ methods, returnTo, description, onDone }: Props) {
  const { t } = useTranslation();
  const hasCode = methods.totp || methods.recoveryCode;
  // IdP で再認証できるのは、IdP のユーザーと紐付いているときだけ。
  const oidc = methods.oidc.enabled && methods.oidc.linked;
  const [useRecovery, setUseRecovery] = useState(!methods.totp);
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  // passkeyError と oidcError は、それぞれのボタンの近くに出す。
  const [passkeyError, setPasskeyError] = useState<string | null>(null);
  const [oidcError, setOIDCError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const run = async (verify: () => Promise<unknown>, report: (message: string) => void) => {
    setSubmitting(true);
    setError(null);
    setPasskeyError(null);
    setOIDCError(null);
    try {
      await verify();
      onDone();
    } catch (caught) {
      report(authErrorMessage(t, caught));
      setCode('');
    } finally {
      setSubmitting(false);
    }
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    void run(
      () =>
        authApi.reauthenticate(
          password,
          useRecovery ? 'recovery_code' : 'totp',
          useRecovery ? code.trim() : code,
        ),
      setError,
    );
  };

  const verifyPasskey = () =>
    void run(
      () =>
        verifyWithPasskey(
          authApi.beginPasskeyReauthentication,
          authApi.confirmPasskeyReauthentication,
        ),
      setPasskeyError,
    );

  // IdP から戻ると、画面を読み直すことになるので、ここでは onDone を呼ばない。
  const verifyWithOIDC = async () => {
    setSubmitting(true);
    setOIDCError(null);
    try {
      const { authorizationUrl } = await authApi.beginOIDCReauthentication(returnTo);
      window.location.assign(authorizationUrl);
    } catch (caught) {
      setOIDCError(authErrorMessage(t, caught));
      setSubmitting(false);
    }
  };

  if (!hasCode && !methods.passkey && !oidc) {
    return (
      <Alert color="amber" role="alert">
        {t('auth.reauth.unavailable')}
      </Alert>
    );
  }

  return (
    <Stack>
      <Text size="sm">{description ?? t('auth.reauth.description')}</Text>
      {methods.passkey && (
        <Stack gap="xs">
          <Button loading={submitting} disabled={!passkeyAvailable()} onClick={verifyPasskey}>
            {t('auth.reauth.passkey')}
          </Button>
          {!passkeyAvailable() && (
            <Text size="xs" c="dimmed">
              {t('auth.login.passkeyUnavailable')}
            </Text>
          )}
          {passkeyError !== null && (
            <Alert color="red" role="alert">
              {passkeyError}
            </Alert>
          )}
        </Stack>
      )}
      {oidc && (
        <Stack gap="xs">
          <Button variant="default" loading={submitting} onClick={() => void verifyWithOIDC()}>
            {t('auth.reauth.oidc', { name: methods.oidc.displayName })}
          </Button>
          {oidcError !== null && (
            <Alert color="red" role="alert">
              {oidcError}
            </Alert>
          )}
        </Stack>
      )}
      {hasCode && (methods.passkey || oidc) && <Divider label={t('auth.reauth.or')} />}
      {hasCode && (
        <form onSubmit={submit}>
          <Stack>
            <PasswordInput
              label={t('auth.reauth.currentPassword')}
              value={password}
              onChange={(e) => setPassword(e.currentTarget.value)}
              required
              autoComplete="current-password"
            />
            {useRecovery ? (
              <TextInput
                label={t('auth.secondFactor.recoveryCode')}
                value={code}
                onChange={(e) => setCode(e.currentTarget.value)}
                placeholder="xxxxx-xxxxx"
                autoComplete="off"
                required
              />
            ) : (
              <Stack gap={4}>
                <Text size="sm">{t('auth.secondFactor.totpDescription')}</Text>
                <PinInput
                  aria-label={t('auth.secondFactor.totp')}
                  length={6}
                  type="number"
                  oneTimeCode
                  value={code}
                  onChange={setCode}
                />
              </Stack>
            )}
            {error !== null && (
              <Alert color="red" role="alert">
                {error}
              </Alert>
            )}
            <Button
              type="submit"
              loading={submitting}
              disabled={password === '' || (useRecovery ? code.trim() === '' : code.length < 6)}
            >
              {t('auth.reauth.submit')}
            </Button>
            {methods.totp && methods.recoveryCode && (
              <Anchor
                component="button"
                type="button"
                size="sm"
                onClick={() => {
                  setUseRecovery(!useRecovery);
                  setCode('');
                  setError(null);
                }}
              >
                {useRecovery
                  ? t('auth.secondFactor.useTOTP')
                  : t('auth.secondFactor.useRecoveryCode')}
              </Anchor>
            )}
          </Stack>
        </form>
      )}
    </Stack>
  );
}
