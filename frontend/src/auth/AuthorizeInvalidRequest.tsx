import { Alert, Button, Group, Stack, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { clientHost } from '../settings/OAuthGrants';

// AuthorizeInvalidRequest は、同意の画面で、認可のリクエストが正しくなかったときに出す (#965、設計書 9.9)。
// errorRedirectUrl があれば、エラーを付けてクライアントへ戻せる。戻す先は、クライアントが自分の文書に書いたものなので、
// 誰でも好きなサイトを書ける。押さずに移ると、Kotatsu のリンクから任意のサイトへ飛ばせてしまう (#1014、RFC 9700 4.11.2)。
// 戻す先のホストを見せ、押してから戻す。
export function AuthorizeInvalidRequest({ errorRedirectUrl }: { errorRedirectUrl: string | null }) {
  const { t } = useTranslation();
  const [leaving, setLeaving] = useState(false);

  return (
    <Stack>
      <Alert color="red" role="alert">
        {t('oauth.invalidRequest')}
      </Alert>
      {errorRedirectUrl != null && (
        <>
          <Text size="sm">{t('oauth.errorRedirect', { host: clientHost(errorRedirectUrl) })}</Text>
          <Group justify="flex-end">
            <Button
              variant="default"
              loading={leaving}
              onClick={() => {
                setLeaving(true);
                window.location.assign(errorRedirectUrl);
              }}
            >
              {t('oauth.back')}
            </Button>
          </Group>
        </>
      )}
    </Stack>
  );
}
