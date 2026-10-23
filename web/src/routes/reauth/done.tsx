import { Alert, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { AuthCard } from '../../auth/AuthCard';
import { oidcErrorMessage } from '../../auth/messages';
import { reportOIDCReauthentication } from '../../auth/oidcReauth';

type DoneSearch = {
  // id は、元の画面が、どの再認証の結果かを見分けるのに使う。
  id?: string;
  // error は、IdP での再認証に失敗したときの理由 (oidc_denied など)。
  error?: string;
};

export const Route = createFileRoute('/reauth/done')({
  validateSearch: (search: Record<string, unknown>): DoneSearch => ({
    ...(typeof search['id'] === 'string' && search['id'] !== '' ? { id: search['id'] } : {}),
    ...(typeof search['error'] === 'string' && search['error'] !== ''
      ? { error: search['error'] }
      : {}),
  }),
  component: ReauthDonePage,
});

// ReauthDonePage は、別のウィンドウで IdP の再認証を終えて戻ってきた画面 (#916)。
// 元の画面に結果を知らせて、ウィンドウを閉じる。閉じられなければ、元の画面に戻るよう案内する。
function ReauthDonePage() {
  const { t } = useTranslation();
  const { id, error } = Route.useSearch();

  useEffect(() => {
    if (id === undefined) {
      return;
    }
    reportOIDCReauthentication({ id, error });
    window.close();
  }, [id, error]);

  return (
    <AuthCard title={t('settings.reauth.title')}>
      {error === undefined ? (
        <Text size="sm">{t('auth.reauth.done')}</Text>
      ) : (
        <Alert color="red" role="alert">
          {oidcErrorMessage(t, error)}
        </Alert>
      )}
    </AuthCard>
  );
}
