import { Alert, Modal } from '@mantine/core';
import { useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { AuthError } from '../auth/api';
import { oidcErrorMessage } from '../auth/messages';
import { Reauthenticate, type ReauthenticationMethods } from '../auth/Reauthenticate';
import { graphQLErrorDetails } from '../relay/environment';

// needsReauthentication は、直近の認証から時間が経っていて、もう一度確かめる必要があるというエラーかどうかを返す (設計書 9.5)。
export function needsReauthentication(error: unknown): boolean {
  return (
    (error instanceof AuthError && error.code === 'REAUTHENTICATION_REQUIRED') ||
    graphQLErrorDetails(error).code === 'REAUTHENTICATION_REQUIRED'
  );
}

type Pending = {
  operation: () => Promise<void>;
  onError: (error: unknown) => void;
  onCancel: (() => void) | undefined;
  onWaiting: (() => void) | undefined;
};

// ReauthRun は、重要な操作を、必要なら再認証してから行う関数。onCancel は、再認証をやめたときに呼ぶ。
// onWaiting は、再認証を求められ、再認証の画面を出して待ち始めたときに呼ぶ。
// 操作が出した読み込み中の表示を、待っている間は消すのに使う (#915)。再認証を終えたら、同じ操作をもう一度行う。
export type ReauthRun = (
  operation: () => Promise<void>,
  onError: (error: unknown) => void,
  onCancel?: () => void,
  onWaiting?: () => void,
) => void;

// returnedError は、IdP での再認証に失敗して戻ってきたときの理由を読む (設計書 9.4)。
function returnedError(): string | null {
  const error = new URLSearchParams(window.location.search).get('error');

  return error === null || error === '' ? null : error;
}

// useReauthGate は、重要な操作を、必要なら再認証してから行う。
// 重要な操作のある画面は、開いたときに SudoGate で再認証させておく (#981)。これは、開いている間に直近の認証の
// 期限 (10 分) が切れたときのためのもの。run に渡した操作が再認証を求められたら、再認証の画面を出し、終えたら
// 同じ操作をもう一度行う。再認証の画面を閉じたら、操作をやめて onCancel を呼ぶ。
// IdP で再認証するときは、ページを離れて、この画面に戻ってくる。戻ってきた画面は、開いたときと同じく SudoGate が
// 確かめるので、待っていた操作は続けず、利用者にもう一度操作してもらう。
// modal は、再認証の画面と、IdP での再認証に失敗して戻ってきたときの理由。
export function useReauthGate(
  methods: ReauthenticationMethods,
  returnTo: string,
): {
  run: ReauthRun;
  busy: boolean;
  modal: ReactNode;
} {
  const { t } = useTranslation();
  const [pending, setPending] = useState<Pending | null>(null);
  // running は、操作の途中かどうか。続けて押されても、同じ操作を重ねて送らないようにする。
  // 押した直後の 2 回目の押下も止めるため、描画を待たない ref で持つ。
  const running = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(returnedError);

  const execute: ReauthRun = (operation, onError, onCancel, onWaiting) => {
    if (running.current) {
      return;
    }
    running.current = true;
    setBusy(true);
    operation()
      .catch((caught: unknown) => {
        if (needsReauthentication(caught)) {
          onWaiting?.();
          setPending({ operation, onError, onCancel, onWaiting });

          return;
        }
        onError(caught);
      })
      .finally(() => {
        running.current = false;
        setBusy(false);
      });
  };

  const modal = (
    <>
      {error !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
          {oidcErrorMessage(t, error)}
        </Alert>
      )}
      <Modal
        opened={pending !== null}
        onClose={() => {
          pending?.onCancel?.();
          setPending(null);
        }}
        title={t('settings.reauth.title')}
        closeButtonProps={{ 'aria-label': t('settings.reauth.close') }}
      >
        {pending !== null && (
          <Reauthenticate
            methods={methods}
            returnTo={returnTo}
            onDone={() => {
              const { operation, onError, onCancel, onWaiting } = pending;
              setPending(null);
              execute(operation, onError, onCancel, onWaiting);
            }}
          />
        )}
      </Modal>
    </>
  );

  return { run: execute, busy, modal };
}

// reauthMethods は、第二要素とパスキーの登録の状況から、再認証に使える方法を返す。
export function reauthMethods(
  secondFactors: { readonly totp: boolean; readonly remainingRecoveryCodes: number },
  passkeys: readonly unknown[],
  oidc: ReauthenticationMethods['oidc'],
): ReauthenticationMethods {
  return {
    totp: secondFactors.totp,
    recoveryCode: secondFactors.remainingRecoveryCodes > 0,
    passkey: passkeys.length > 0,
    oidc,
  };
}
