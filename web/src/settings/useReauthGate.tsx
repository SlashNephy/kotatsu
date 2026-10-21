import { Modal } from '@mantine/core';
import { useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { AuthError } from '../auth/api';
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

// useReauthGate は、重要な操作を、必要なら再認証してから行う。
// run に渡した操作が再認証を求められたら、再認証の画面を出し、終えたら同じ操作をもう一度行う。
// 再認証の画面を閉じたら、操作をやめて onCancel を呼ぶ。
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

  const execute: ReauthRun = (operation, onError, onCancel, onWaiting) => {
    if (running.current) {
      return;
    }
    running.current = true;
    setBusy(true);
    operation()
      .catch((error: unknown) => {
        if (needsReauthentication(error)) {
          onWaiting?.();
          setPending({ operation, onError, onCancel, onWaiting });

          return;
        }
        onError(error);
      })
      .finally(() => {
        running.current = false;
        setBusy(false);
      });
  };

  const modal = (
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
