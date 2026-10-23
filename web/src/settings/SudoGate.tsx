import { Card, Stack, Title } from '@mantine/core';
import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi } from '../auth/api';
import { Reauthenticate, type ReauthenticationMethods } from '../auth/Reauthenticate';
import { PageLoader } from '../futon';

type Props = {
  methods: ReauthenticationMethods;
  // returnTo は、IdP で再認証したときに戻す、この画面のパス。
  returnTo: string;
  children: ReactNode;
};

// SudoGate は、重要な操作のある画面 (か、その一部) を、開いたときに、必要なら本人確認してから出す (設計書 9.5、#981)。
// 入力の途中で再認証を求めると、IdP での再認証はページを離れるので、入力が消えてしまう。開いたときに確かめておけば、
// 入力を始める前に済む。直近 10 分以内に認証していれば、そのまま出す。
// 開いている間に期限が切れたときは、操作のときに useReauthGate が改めて再認証を求める。
export function SudoGate({ methods, returnTo, children }: Props) {
  const { t } = useTranslation();
  const [state, setState] = useState<'checking' | 'required' | 'ready'>('checking');

  useEffect(() => {
    let active = true;
    authApi.reauthenticationStatus().then(
      ({ recent }) => {
        if (active) {
          setState(recent ? 'ready' : 'required');
        }
      },
      // 確かめられなくても、中身は出す。必要なら、操作のときに再認証を求められる。
      () => {
        if (active) {
          setState('ready');
        }
      },
    );

    return () => {
      active = false;
    };
  }, []);

  if (state === 'checking') {
    return <PageLoader />;
  }
  if (state === 'required') {
    return (
      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('settings.reauth.title')}
          </Title>
          <Reauthenticate
            methods={methods}
            returnTo={returnTo}
            description={t('settings.reauth.sudoDescription')}
            onDone={() => setState('ready')}
          />
        </Stack>
      </Card>
    );
  }

  return <>{children}</>;
}
