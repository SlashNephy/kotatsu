import {
  Alert,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  PasswordInput,
  Stack,
  Text,
  TextInput,
} from '@mantine/core';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { zonedFormat } from '../futon';
import { GoogleDriveFolders, type SelectedFolder } from './GoogleDriveFolders';
import { Section } from './Section';

const dateFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// googleCallbackErrors は、Google から戻ってきたときに、連携の画面に付く理由 (error=) のうち、画面が知っているもの。
export const googleCallbackErrors = [
  'google_expired',
  'google_mismatch',
  'google_denied',
  'google_no_refresh_token',
  'google_unavailable',
  'google_failed',
] as const;

export type GoogleCallbackError = (typeof googleCallbackErrors)[number];

// GoogleIntegrationItem は、Google の OAuth クライアントと、Google Drive の接続 (GraphQL の GoogleIntegration)。
export type GoogleIntegrationItem = {
  clientId: string | null | undefined;
  configured: boolean;
  redirectUri: string;
  drive:
    | {
        accountName: string;
        connectedAt: string;
        revokedAt: string | null | undefined;
      }
    | null
    | undefined;
  // folders は、ライブラリとして見せると選んだフォルダ。
  folders: readonly SelectedFolder[];
};

// GoogleAppInput は、クライアントの登録の変更。clientSecret を省けば、登録したものを変えない。
export type GoogleAppInput = { clientId: string; clientSecret?: string };

// GoogleDriveSettings は、管理者の Google Drive を、ライブラリとして接続する設定 (設計書 5.8、#1902)。
// Google Cloud で作った OAuth クライアントのクライアント ID とシークレットを登録し、接続する。リダイレクト URI は、
// Google の連携で共有する /internal/auth/google/callback を出す。
export function GoogleDriveSettings({
  integration,
  busy,
  connected,
  callbackError,
  onDismiss,
  onSaveApp,
  onConnect,
  onDisconnect,
  onAddFolder,
  onRenameFolder,
  onRemoveFolder,
}: {
  integration: GoogleIntegrationItem;
  busy: boolean;
  // connected は、Google から接続して戻ってきたかどうか。callbackError は、失敗して戻ってきたときの理由。
  connected: boolean;
  callbackError: GoogleCallbackError | null;
  onDismiss: () => void;
  onSaveApp: (input: GoogleAppInput, onDone: () => void) => void;
  onConnect: () => void;
  onDisconnect: () => void;
  onAddFolder: (id: string, libraryName: string, onDone: () => void) => void;
  onRenameFolder: (id: string, libraryName: string, onDone: () => void) => void;
  onRemoveFolder: (id: string) => void;
}) {
  const { t } = useTranslation();
  const drive = integration.drive ?? null;
  const revokedAt = drive?.revokedAt ?? null;

  return (
    <Section title={t('admin.googleDrive.title')}>
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          {t('admin.googleDrive.help')}
        </Text>
        <Text size="sm" c="dimmed">
          {t('admin.googleDrive.familyNote')}
        </Text>
        <Text size="sm" c="dimmed">
          {t('admin.googleDrive.testingNote')}
        </Text>
        {callbackError !== null && (
          <Alert color="red" role="alert" withCloseButton onClose={onDismiss}>
            {t(`admin.googleDrive.callbackErrors.${callbackError}`)}
          </Alert>
        )}
        {connected && (
          <Alert color="green" role="status" withCloseButton onClose={onDismiss}>
            {t('admin.googleDrive.connected')}
          </Alert>
        )}
        <GoogleAppForm
          key={`${integration.clientId ?? ''}/${String(integration.configured)}`}
          integration={integration}
          busy={busy}
          onSave={onSaveApp}
        />
        <Stack gap="xs">
          {revokedAt !== null && (
            <Alert color="amber" role="alert">
              {t('admin.googleDrive.revoked', { date: dateFormat.format(new Date(revokedAt)) })}
            </Alert>
          )}
          <Group justify="space-between" wrap="wrap">
            <Text size="sm" c={drive === null ? 'dimmed' : undefined}>
              {drive === null
                ? integration.configured
                  ? t('admin.googleDrive.notConnected')
                  : t('admin.googleDrive.needsClient')
                : t('admin.googleDrive.connectedAs', {
                    account: drive.accountName,
                    date: dateFormat.format(new Date(drive.connectedAt)),
                  })}
            </Text>
            <Group gap="xs">
              {drive !== null && (
                <Button
                  variant="default"
                  size="xs"
                  color="red"
                  disabled={busy}
                  onClick={onDisconnect}
                >
                  {t('admin.googleDrive.disconnect')}
                </Button>
              )}
              <Button
                size="xs"
                variant={drive !== null && revokedAt === null ? 'default' : 'filled'}
                disabled={busy || !integration.configured}
                onClick={onConnect}
              >
                {drive === null ? t('admin.googleDrive.connect') : t('admin.googleDrive.reconnect')}
              </Button>
            </Group>
          </Group>
        </Stack>
        {/* 見せるフォルダは、接続していて、接続が切れていないときだけ選べる (#1902)。 */}
        {drive !== null && revokedAt === null && (
          <GoogleDriveFolders
            folders={integration.folders}
            busy={busy}
            onAdd={onAddFolder}
            onRename={onRenameFolder}
            onRemove={onRemoveFolder}
          />
        )}
      </Stack>
    </Section>
  );
}

// GoogleAppForm は、Google の OAuth クライアントのクライアント ID とシークレットを変える。シークレットはサーバーから返らないので、
// 変えるときだけ入れる。クライアント ID を空にして保存すると、登録を消し、Google Drive の接続も外す。
function GoogleAppForm({
  integration,
  busy,
  onSave,
}: {
  integration: GoogleIntegrationItem;
  busy: boolean;
  onSave: (input: GoogleAppInput, onDone: () => void) => void;
}) {
  const { t } = useTranslation();
  const [clientId, setClientId] = useState(integration.clientId ?? '');
  const [clientSecret, setClientSecret] = useState('');
  const changed = clientId.trim() !== (integration.clientId ?? '') || clientSecret !== '';
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSave(
      clientSecret === ''
        ? { clientId: clientId.trim() }
        : { clientId: clientId.trim(), clientSecret },
      () => setClientSecret(''),
    );
  };

  return (
    <form onSubmit={submit}>
      <Stack gap="xs">
        <Group gap="xs">
          <Text size="sm" fw={600}>
            {t('admin.googleDrive.clientId')}
          </Text>
          <Badge size="sm" variant="light" color={integration.configured ? 'teal' : 'gray'}>
            {integration.configured
              ? t('admin.googleDrive.configured')
              : t('admin.googleDrive.notConfigured')}
          </Badge>
        </Group>
        <Stack gap={2}>
          <Text size="xs" c="dimmed">
            {t('admin.googleDrive.redirectUri')}
          </Text>
          <Group gap="xs" wrap="nowrap">
            <Code style={{ overflowWrap: 'anywhere' }}>{integration.redirectUri}</Code>
            <CopyButton value={integration.redirectUri}>
              {({ copied, copy }) => (
                <Button size="compact-xs" variant="default" onClick={copy}>
                  {copied ? t('settings.account.copied') : t('settings.account.copy')}
                </Button>
              )}
            </CopyButton>
          </Group>
        </Stack>
        <Group align="flex-end" wrap="wrap">
          <TextInput
            size="sm"
            label={t('admin.googleDrive.clientId')}
            value={clientId}
            onChange={(event) => setClientId(event.currentTarget.value)}
            autoComplete="off"
            w={260}
          />
          <PasswordInput
            size="sm"
            label={t('admin.googleDrive.clientSecret')}
            placeholder={integration.configured ? t('admin.googleDrive.secretKept') : undefined}
            value={clientSecret}
            onChange={(event) => setClientSecret(event.currentTarget.value)}
            autoComplete="new-password"
            w={260}
          />
          <Button type="submit" size="compact-sm" disabled={!changed || busy}>
            {t('admin.integrations.register')}
          </Button>
        </Group>
      </Stack>
    </form>
  );
}
