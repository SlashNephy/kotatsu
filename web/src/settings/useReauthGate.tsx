import { Modal } from '@mantine/core';
import { useState, type ReactNode } from 'react';
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

type Pending = { operation: () => Promise<void>; onError: (error: unknown) => void };

// useReauthGate は、重要な操作を、必要なら再認証してから行う。
// run に渡した操作が再認証を求められたら、再認証の画面を出し、終えたら同じ操作をもう一度行う。
export function useReauthGate(
  methods: ReauthenticationMethods,
  returnTo: string,
): {
  run: (operation: () => Promise<void>, onError: (error: unknown) => void) => void;
  modal: ReactNode;
} {
  const { t } = useTranslation();
  const [pending, setPending] = useState<Pending | null>(null);

  const run = (operation: () => Promise<void>, onError: (error: unknown) => void) => {
    operation().catch((error: unknown) => {
      if (needsReauthentication(error)) {
        setPending({ operation, onError });

        return;
      }
      onError(error);
    });
  };

  const modal = (
    <Modal
      opened={pending !== null}
      onClose={() => setPending(null)}
      title={t('settings.reauth.title')}
      closeButtonProps={{ 'aria-label': t('settings.reauth.close') }}
    >
      {pending !== null && (
        <Reauthenticate
          methods={methods}
          returnTo={returnTo}
          onDone={() => {
            const { operation, onError } = pending;
            setPending(null);
            operation().catch(onError);
          }}
        />
      )}
    </Modal>
  );

  return { run, modal };
}
