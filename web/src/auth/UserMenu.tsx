import { Avatar, Box, Button, Indicator, Menu, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { ColorSchemeToggle } from '../futon';
import { authApi } from './api';
import { formatVersion } from './version';
import type { UserMenuQuery } from './__generated__/UserMenuQuery.graphql';

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

  return (
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
        <Menu.Item component={Link} to="/settings/account">
          {t('nav.account')}
        </Menu.Item>
        <Menu.Item component={Link} to="/captures">
          {t('nav.captures')}
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
        <Menu.Item
          color="red"
          onClick={() => {
            // ログアウトに失敗しても (セッションが切れているときなど)、ログインの画面へ移す。
            void authApi
              .logout()
              .finally(() => window.location.assign(`${import.meta.env.BASE_URL}login`));
          }}
        >
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
  );
}
