import { Alert, Modal } from '@mantine/core';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi, AuthError } from '../auth/api';
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

// ReauthResume は、IdP での再認証から戻ってきたときに、待っていた操作を続けるための手がかり (#916)。
// IdP での再認証はページを離れるので、操作 (関数) は持ち越せない。代わりに、操作の種類 key と、
// 対象の ID などの params を覚えておき、戻ってきた画面で useReauthResume に登録した続け方を呼ぶ。
// 入力した値は持ち越さない。入力のある操作は、入力を始める前に prepare で再認証させる。
export type ReauthResume = { key: string; params?: string };

type Pending = {
  operation: () => Promise<void>;
  onError: (error: unknown) => void;
  onCancel: (() => void) | undefined;
  onWaiting: (() => void) | undefined;
  resume: ReauthResume | undefined;
};

// ReauthRun は、重要な操作を、必要なら再認証してから行う関数。onCancel は、再認証をやめたときに呼ぶ。
// onWaiting は、再認証を求められ、再認証の画面を出して待ち始めたときに呼ぶ。
// 操作が出した読み込み中の表示を、待っている間は消すのに使う (#915)。再認証を終えたら、同じ操作をもう一度行う。
// resume を渡すと、IdP で再認証して戻ってきたときにも、useReauthResume に登録した続け方で操作を続ける。
export type ReauthRun = (
  operation: () => Promise<void>,
  onError: (error: unknown) => void,
  onCancel?: () => void,
  onWaiting?: () => void,
  resume?: ReauthResume,
) => void;

// ReauthPrepare は、入力のある重要な操作の、入力を始める前に、必要なら再認証させる関数。
// 再認証が要らないか、終えたら onReady を呼ぶ。IdP で再認証して戻ってきたときは、useReauthResume に登録した続け方を呼ぶ。
export type ReauthPrepare = (resume: ReauthResume, onReady: () => void) => void;

const resumeStorageKey = 'kotatsu.reauth.resume';
// resumeLifetime は、覚えておいた手がかりを使える長さ。IdP で手間取っても、これを過ぎたら続けない。
const resumeLifetime = 10 * 60 * 1000;

// StoredResume は、IdP へ移る前に sessionStorage に覚えておく手がかり。key が null なら、続け方がない。
type StoredResume = { key: string | null; params?: string; path: string; savedAt: number };

function saveResume(resume: ReauthResume | undefined) {
  const stored: StoredResume = {
    key: resume?.key ?? null,
    ...(resume?.params === undefined ? {} : { params: resume.params }),
    path: window.location.pathname,
    savedAt: Date.now(),
  };
  try {
    window.sessionStorage.setItem(resumeStorageKey, JSON.stringify(stored));
  } catch {
    // 覚えておけなくても、戻ってきたときに、もう一度操作してもらうだけで済む。
  }
}

function clearResume() {
  try {
    window.sessionStorage.removeItem(resumeStorageKey);
  } catch {
    // 消せなくても、覚えておいた時刻から時間が経てば使わない。
  }
}

// takeResume は、IdP での再認証から、この画面に戻ってきたときだけ、覚えておいた手がかりを返し、消す。
// 同じ操作を 2 回続けないよう、返す前に消す (読み直しや、開発時の StrictMode で、もう一度呼ばれても続けない)。
// このページを読み込んだ後に覚えたものは、まだ IdP へ移っていないので返さない。戻ってきた URL のまま
// (reauthenticated=oidc が付いたまま) 次の操作で再認証を求められても、再認証せずに続けないようにする。
function takeResume(match: (stored: StoredResume) => boolean): StoredResume | null {
  if (new URLSearchParams(window.location.search).get('reauthenticated') !== 'oidc') {
    return null;
  }
  let stored: StoredResume;
  try {
    const raw = window.sessionStorage.getItem(resumeStorageKey);
    if (raw === null) {
      return null;
    }
    stored = JSON.parse(raw) as StoredResume;
  } catch {
    return null;
  }
  if (
    stored.path !== window.location.pathname ||
    stored.savedAt >= performance.timeOrigin ||
    Date.now() - stored.savedAt > resumeLifetime ||
    !match(stored)
  ) {
    return null;
  }
  clearResume();

  return stored;
}

// useReauthResume は、IdP での再認証から戻ってきたときに、key の操作を続ける方法を登録する。
// handler には、run や prepare に渡した params を渡す。
export function useReauthResume(key: string, handler: (params: string | undefined) => void) {
  const handlerRef = useRef(handler);
  useEffect(() => {
    handlerRef.current = handler;
  });
  useEffect(() => {
    const stored = takeResume((s) => s.key === key);
    if (stored !== null) {
      handlerRef.current(stored.params);
    }
  }, [key]);
}

// useReauthBeforeInput は、入力のある重要な操作の入力欄に、最初にフォーカスしたときに、必要なら再認証させる。
// IdP で再認証して戻ってきたら、その入力欄にフォーカスし直す。返した ref と onFocus を入力欄に渡す。
// 再認証をやめても、入力は止めない (操作のときに、改めて再認証を求められる)。
export function useReauthBeforeInput(prepare: ReauthPrepare, key: string) {
  const ref = useRef<HTMLInputElement>(null);
  // checked は、確かめたかどうか。再認証の画面を閉じて入力欄にフォーカスが戻ったときに、また確かめないようにする。
  const checked = useRef(false);
  useReauthResume(key, () => {
    // 戻ってきたときは再認証を終えているので、フォーカスし直しても、もう一度は確かめない。
    checked.current = true;
    ref.current?.focus();
  });
  const onFocus = () => {
    if (checked.current) {
      return;
    }
    checked.current = true;
    prepare({ key }, () => undefined);
  };

  return { ref, onFocus };
}

// returnedError は、IdP での再認証に失敗して戻ってきたときの理由を読む (設計書 9.4)。
function returnedError(): string | null {
  const error = new URLSearchParams(window.location.search).get('error');

  return error === null || error === '' ? null : error;
}

// useReauthGate は、重要な操作を、必要なら再認証してから行う。
// run に渡した操作が再認証を求められたら、再認証の画面を出し、終えたら同じ操作をもう一度行う。
// 再認証の画面を閉じたら、操作をやめて onCancel を呼ぶ。
// prepare は、入力のある操作の、入力を始める前に再認証させる。IdP での再認証はページを離れ、入力が消えるため (#916)。
// modal は、再認証の画面と、IdP での再認証から戻ってきたときの知らせ。続け方のない操作を待っていたときは、
// もう一度操作するよう知らせる。失敗して戻ってきたときは、理由を出す。
export function useReauthGate(
  methods: ReauthenticationMethods,
  returnTo: string,
): {
  run: ReauthRun;
  prepare: ReauthPrepare;
  busy: boolean;
  modal: ReactNode;
} {
  const { t } = useTranslation();
  const [pending, setPending] = useState<Pending | null>(null);
  // running は、操作の途中かどうか。続けて押されても、同じ操作を重ねて送らないようにする。
  // 押した直後の 2 回目の押下も止めるため、描画を待たない ref で持つ。
  const running = useRef(false);
  const [busy, setBusy] = useState(false);
  const [returned, setReturned] = useState<{ reauthenticated: true } | { error: string } | null>(
    null,
  );

  // 続け方を登録した操作は、useReauthResume が手がかりを取る。続け方のない操作を待っていたときだけ、
  // もう一度操作するよう知らせる。
  useEffect(() => {
    const error = returnedError();
    const stored = takeResume((s) => s.key === null);
    if (error !== null) {
      clearResume();
      setReturned({ error });
    } else if (stored !== null) {
      setReturned({ reauthenticated: true });
    }
  }, []);

  const wait = (next: Pending) => {
    next.onWaiting?.();
    // IdP で再認証するとページを離れるので、続けるための手がかりを先に覚えておく。
    saveResume(next.resume);
    setPending(next);
  };

  const execute: ReauthRun = (operation, onError, onCancel, onWaiting, resume) => {
    if (running.current) {
      return;
    }
    running.current = true;
    setBusy(true);
    operation()
      .catch((error: unknown) => {
        if (needsReauthentication(error)) {
          wait({ operation, onError, onCancel, onWaiting, resume });

          return;
        }
        onError(error);
      })
      .finally(() => {
        running.current = false;
        setBusy(false);
      });
  };

  const prepare: ReauthPrepare = (resume, onReady) => {
    authApi.reauthenticationStatus().then(
      ({ recent }) => {
        if (recent) {
          onReady();

          return;
        }
        wait({
          operation: () => {
            onReady();

            return Promise.resolve();
          },
          onError: () => undefined,
          onCancel: undefined,
          onWaiting: undefined,
          resume,
        });
      },
      // 確かめられなくても、入力は始めさせる。必要なら、操作のときに再認証を求められる。
      () => onReady(),
    );
  };

  const modal = (
    <>
      {returned !== null &&
        ('error' in returned ? (
          <Alert color="red" role="alert" withCloseButton onClose={() => setReturned(null)}>
            {oidcErrorMessage(t, returned.error)}
          </Alert>
        ) : (
          <Alert color="green" role="status" withCloseButton onClose={() => setReturned(null)}>
            {t('settings.reauth.returned')}
          </Alert>
        ))}
      <Modal
        opened={pending !== null}
        onClose={() => {
          pending?.onCancel?.();
          clearResume();
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
              const { operation, onError, onCancel, onWaiting, resume } = pending;
              clearResume();
              setPending(null);
              execute(operation, onError, onCancel, onWaiting, resume);
            }}
          />
        )}
      </Modal>
    </>
  );

  return { run: execute, prepare, busy, modal };
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
