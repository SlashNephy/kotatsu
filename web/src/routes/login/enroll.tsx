import {
  Alert,
  Button,
  Code,
  CopyButton,
  Image,
  Loader,
  PinInput,
  SimpleGrid,
  Stack,
  Text,
} from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { renderSVG } from 'uqr';

import { AuthError, authApi, type TOTPEnrollment } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { authErrorMessage } from '../../auth/messages';

export const Route = createFileRoute('/login/enroll')({
  component: Enroll,
});

// Enroll は、第二要素を 1 つも登録していないユーザーが、ログインの途中で認証アプリを登録する画面 (設計書 9.3)。
// 登録を終えると、リカバリーコードを 1 回だけ見せる。
function Enroll() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [enrollment, setEnrollment] = useState<TOTPEnrollment | null>(null);
  const [code, setCode] = useState('');
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    authApi.beginEnrollment().then(setEnrollment, async (caught: unknown) => {
      if (caught instanceof AuthError && caught.code === 'LOGIN_REQUIRED') {
        await navigate({ to: '/login' });

        return;
      }
      setError(authErrorMessage(t, caught));
    });
  }, [navigate, t]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const completed = await authApi.confirmEnrollment(code);
      setRecoveryCodes(completed.recoveryCodes ?? []);
    } catch (caught) {
      setError(authErrorMessage(t, caught));
      setCode('');
    } finally {
      setSubmitting(false);
    }
  };

  if (recoveryCodes !== null) {
    return (
      <AuthCard
        title={t('auth.enroll.recoveryTitle')}
        description={t('auth.enroll.recoveryDescription')}
      >
        <SimpleGrid cols={2} spacing="xs">
          {recoveryCodes.map((recoveryCode) => (
            <Code key={recoveryCode} ta="center" fz="md">
              {recoveryCode}
            </Code>
          ))}
        </SimpleGrid>
        <CopyButton value={recoveryCodes.join('\n')}>
          {({ copied, copy }) => (
            <Button variant="light" onClick={copy}>
              {copied ? t('auth.enroll.copied') : t('auth.enroll.copy')}
            </Button>
          )}
        </CopyButton>
        <Button onClick={() => window.location.assign('/')}>{t('auth.enroll.continue')}</Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard title={t('auth.enroll.title')} description={t('auth.enroll.description')}>
      {enrollment === null ? (
        error === null && <Loader mx="auto" />
      ) : (
        <>
          <Image
            src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(renderSVG(enrollment.uri))}`}
            alt={t('auth.enroll.qr')}
            w={200}
            h={200}
            mx="auto"
            bg="white"
            p="xs"
            radius="md"
          />
          <Stack gap={4}>
            <Text size="sm">{t('auth.enroll.secret')}</Text>
            <Code block>{enrollment.secret.match(/.{1,4}/g)?.join(' ')}</Code>
          </Stack>
          <form onSubmit={(event) => void submit(event)}>
            <Stack>
              <Stack gap={4}>
                <Text size="sm" ta="center">
                  {t('auth.enroll.code')}
                </Text>
                <PinInput
                  aria-label={t('auth.enroll.code')}
                  length={6}
                  type="number"
                  oneTimeCode
                  placeholder=""
                  my="md"
                  styles={{ root: { justifyContent: 'center' } }}
                  value={code}
                  onChange={setCode}
                />
              </Stack>
              <Button type="submit" loading={submitting} disabled={code.length !== 6}>
                {t('auth.enroll.submit')}
              </Button>
            </Stack>
          </form>
        </>
      )}
      {error !== null && (
        <Alert color="red" role="alert">
          {error}
        </Alert>
      )}
    </AuthCard>
  );
}
