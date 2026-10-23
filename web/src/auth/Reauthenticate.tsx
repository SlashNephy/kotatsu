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
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi, type OIDCStatus } from './api';
import { authErrorMessage, oidcErrorMessage } from './messages';
import {
  oidcReauthReturnTo,
  openOIDCReauthWindow,
  waitForOIDCReauthentication,
} from './oidcReauth';
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
  onDone: () => void;
};

// Reauthenticate は、パスワードの変更のような重要な操作の前に、ログイン中のユーザーをもう一度確かめる (設計書 9.3)。
// パスワードと第二要素、パスキー、IdP のうち、ユーザーが使えるものを出す。
export function Reauthenticate({ methods, returnTo, onDone }: Props) {
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
  // oidcWaiting は、別のウィンドウで IdP の再認証を始め、結果を待っているかどうか。
  const [oidcWaiting, setOIDCWaiting] = useState(false);
  // stopWaiting は、別のウィンドウでの再認証の結果を待つのをやめる関数。
  const stopWaiting = useRef<(() => void) | null>(null);
  useEffect(() => () => stopWaiting.current?.(), []);

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

  // IdP での再認証は、別のウィンドウで行い、この画面を離れない (#916)。終えたと知らされたら onDone を呼ぶ。
  // ウィンドウを開けなければ、この画面のまま IdP へ送り出す。戻ると画面を読み直すので、待っていた操作は続けられない。
  const verifyWithOIDC = async () => {
    setSubmitting(true);
    setOIDCError(null);
    // ブラウザーがポップアップを止めないよう、IdP の URL を待たずに、押した直後に開く。
    const popup = openOIDCReauthWindow();
    try {
      if (popup === null) {
        const { authorizationUrl } = await authApi.beginOIDCReauthentication(returnTo);
        window.location.assign(authorizationUrl);

        return;
      }
      const id = crypto.randomUUID();
      // IdP へ送り出す前の認証の日時を読んでおき、変わったら終えたとみなす。
      const [{ authorizationUrl }, { authenticatedAt }] = await Promise.all([
        authApi.beginOIDCReauthentication(oidcReauthReturnTo(id)),
        authApi.reauthenticationStatus(),
      ]);
      stopWaiting.current?.();
      stopWaiting.current = waitForOIDCReauthentication(
        id,
        authenticatedAt,
        authApi.reauthenticationStatus,
        ({ error: reason }) => {
          stopWaiting.current = null;
          setOIDCWaiting(false);
          if (reason === undefined) {
            onDone();
          } else {
            setOIDCError(oidcErrorMessage(t, reason));
          }
        },
      );
      // もう一度押したときは、IdP を開いたままの (ほかのオリジンの) ウィンドウが返る。
      // ほかのオリジンの Location では assign を呼べないので、href に入れる。
      popup.location.href = authorizationUrl;
      setOIDCWaiting(true);
      setSubmitting(false);
    } catch (caught) {
      popup?.close();
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
      <Text size="sm">{t('auth.reauth.description')}</Text>
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
          {oidcWaiting && (
            <Text size="sm" c="dimmed">
              {t('auth.reauth.oidcWaiting', { name: methods.oidc.displayName })}
            </Text>
          )}
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
