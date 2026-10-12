import { Box, Button, Menu, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ColorSchemeToggle } from '../futon';
import { authApi } from './api';
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
        }
      }
    `,
    {},
  );

  return (
    <Menu position="bottom-end" width={240} withinPortal>
      <Menu.Target>
        <Button variant="subtle" size="compact-sm" aria-label={t('nav.userMenu')}>
          {data.viewer.displayName}
        </Button>
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
            void authApi.logout().then(() => window.location.assign('/login'));
          }}
        >
          {t('auth.logout')}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  );
}
