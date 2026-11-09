import { Avatar, Box, Button, Group, Indicator, Menu, Modal, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { ColorSchemeToggle } from '../futon';
import { authApi } from './api';
import { unsubscribeDevicePush } from './logout';
import { formatVersion } from './version';
import { mutate } from '../settings/mutate';
import {
  refreshOfflineRecordings,
  removeOfflineRecording,
  savedOfflineRecordings,
} from '../offline/store';
import type { UserMenuQuery } from './__generated__/UserMenuQuery.graphql';
import type { UserMenuUnsubscribeMutation } from './__generated__/UserMenuUnsubscribeMutation.graphql';

// UserMenu は、ヘッダーに出す、ログイン中のユーザーのメニュー。設定、配色の切り替え、ログアウトをまとめる。
// ログインしていなければ、読み込みが失敗するので、呼ぶ側の ErrorBoundary で出さないようにする。
export function UserMenu() {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<UserMenuQuery>(
    graphql`
      query UserMenuQuery {
        viewer {
          displayName
          username
          avatarUrl
        }
        version {
          version
          revision
          modified
          latestRelease {
            version
            url
          }
        }
      }
    `,
    {},
  );
  // 新しい版は、管理者にだけ届く (#506)。
  const latest = data.version.latestRelease;
  const environment = useRelayEnvironment();
  // 端末に保存した録画があるときに、消すかどうかを尋ねるダイアログ (#1365)。
  const [savedCount, setSavedCount] = useState<number | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);

  // logout は、この端末の購読を解除してからログアウトする。removeOffline なら、端末に保存した録画も消す。
  const logout = async (removeOffline: boolean) => {
    setLoggingOut(true);
    await unsubscribeDevicePush((endpoint) =>
      mutate<UserMenuUnsubscribeMutation>(
        environment,
        graphql`
          mutation UserMenuUnsubscribeMutation($endpoint: String!) {
            unsubscribeWebPush(endpoint: $endpoint)
          }
        `,
        { endpoint },
      ),
    );
    if (removeOffline) {
      await Promise.allSettled(
        savedOfflineRecordings().map((meta) => removeOfflineRecording(meta.id)),
      );
    }
    // ログアウトに失敗しても (セッションが切れているときなど)、ログインの画面へ移す。
    void authApi.logout().finally(() => window.location.assign(`${import.meta.env.BASE_URL}login`));
  };

  const requestLogout = () => {
    // 一覧を読み直してから、保存した録画の有無で分ける。読めない端末では、保存した録画はない。
    void refreshOfflineRecordings()
      .catch(() => {})
      .then(() => {
        const count = savedOfflineRecordings().length;
        if (count > 0) {
          setSavedCount(count);
        } else {
          void logout(false);
        }
      });
  };

  return (
    <>
      {savedCount !== null && (
        <Modal opened onClose={() => setSavedCount(null)} title={t('auth.logoutOfflineTitle')}>
          <Stack>
            <Text size="sm">{t('auth.logoutOfflineDescription', { count: savedCount })}</Text>
            <Group justify="flex-end">
              <Button variant="default" disabled={loggingOut} onClick={() => setSavedCount(null)}>
                {t('reservations.cancel')}
              </Button>
              <Button variant="default" disabled={loggingOut} onClick={() => void logout(false)}>
                {t('auth.logoutKeepOffline')}
              </Button>
              <Button color="red" loading={loggingOut} onClick={() => void logout(true)}>
                {t('auth.logoutRemoveOffline')}
              </Button>
            </Group>
          </Stack>
        </Modal>
      )}
      <Menu position="bottom-end" width={240} withinPortal>
        <Menu.Target>
          <Indicator disabled={latest == null} color="teal" size={8} offset={4}>
            <Button
              variant="subtle"
              size="compact-sm"
              maw={180}
              // ユーザーのアイコン (#778)。IdP の picture を tenban が中継する。画像がないか、読めなければ、
              // 表示名の頭文字を、名前ごとに決まる色で出す。
              leftSection={
                <Avatar
                  src={data.viewer.avatarUrl}
                  name={data.viewer.displayName}
                  color="initials"
                  size={22}
                  aria-hidden
                />
              }
            >
              <Text span size="sm" truncate="end">
                {data.viewer.displayName}
              </Text>
            </Button>
          </Indicator>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Label>
            <Text size="xs" c="dimmed">
              @{data.viewer.username}
            </Text>
          </Menu.Label>
          <Menu.Item component={Link} to="/captures">
            {t('nav.captures')}
          </Menu.Item>
          <Menu.Item component={Link} to="/offline">
            {t('nav.offline')}
          </Menu.Item>
          <Menu.Item component={Link} to="/remote">
            {t('nav.remote')}
          </Menu.Item>
          <Menu.Item component={Link} to="/settings">
            {t('nav.settings')}
          </Menu.Item>
          <Menu.Divider />
          <Menu.Label>{t('colorScheme.label')}</Menu.Label>
          {/* 配色はメニューを閉じずに切り替えられるよう、項目ではなく部品として置く。 */}
          <Box px="xs" pb="xs">
            <ColorSchemeToggle />
          </Box>
          <Menu.Divider />
          <Menu.Item color="red" onClick={requestLogout}>
            {t('auth.logout')}
          </Menu.Item>
          <Menu.Divider />
          {latest != null && (
            <Menu.Item
              component="a"
              href={latest.url}
              target="_blank"
              rel="noopener noreferrer"
              c="teal"
            >
              {t('app.newRelease', { version: latest.version })}
            </Menu.Item>
          )}
          <Menu.Label>
            <Text size="xs" c="dimmed">
              {t('app.version', { version: formatVersion(data.version) })}
            </Text>
          </Menu.Label>
        </Menu.Dropdown>
      </Menu>
    </>
  );
}
