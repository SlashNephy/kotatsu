import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  PasswordInput,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { authApi, type OIDCStatus } from '../../auth/api';
import { PasswordStrength } from '../../futon';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import type { Role, usersCreateMutation } from './__generated__/usersCreateMutation.graphql';
import type { usersDisableMutation } from './__generated__/usersDisableMutation.graphql';
import type { usersEnableMutation } from './__generated__/usersEnableMutation.graphql';
import type { usersQuery } from './__generated__/usersQuery.graphql';
import type { usersResetPasswordMutation } from './__generated__/usersResetPasswordMutation.graphql';
import type { usersUpdateMutation } from './__generated__/usersUpdateMutation.graphql';
import { zonedFormat } from '../../futon';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/users';

export const Route = createFileRoute('/admin/users')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: UsersPage,
});

const dateFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
});

const roles = ['ADMIN', 'MEMBER', 'VIEWER'] as const satisfies readonly Role[];

// UsersPage は、ユーザーを作り、表示名とロールを変え、無効にする、管理者向けの画面 (設計書 9.7、10.3)。
function UsersPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.users.title')}>
      <Users />
    </AdminPage>
  );
}

const query = graphql`
  query usersQuery {
    viewer {
      id
    }
    users {
      id
      username
      displayName
      role
      disabled
      createdAt
    }
    secondFactors {
      totp
      remainingRecoveryCodes
    }
    passkeys {
      id
    }
  }
`;

type Message = { ok: boolean; text: string };

function Users() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<usersQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const gate = useReauthGate(reauthMethods(data.secondFactors, data.passkeys, oidc), path);
  const [message, setMessage] = useState<Message | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const succeed = (text: string) => {
    setMessage({ ok: true, text });
    refresh();
  };

  return (
    <Stack gap="md">
      {gate.modal}
      {message !== null && (
        <Alert
          color={message.ok ? 'teal' : 'red'}
          role={message.ok ? 'status' : 'alert'}
          withCloseButton
          onClose={() => setMessage(null)}
        >
          {message.text}
        </Alert>
      )}

      <Card withBorder>
        <Stack gap="xs">
          {data.users.map((user) => (
            <UserRow
              key={`${user.id}/${user.displayName}/${user.role}/${String(user.disabled)}`}
              user={user}
              self={user.id === data.viewer.id}
              onUpdate={(input, onDone) =>
                gate.run(async () => {
                  await mutate<usersUpdateMutation>(
                    environment,
                    graphql`
                      mutation usersUpdateMutation($id: ID!, $input: UpdateUserInput!) {
                        updateUser(id: $id, input: $input) {
                          id
                          displayName
                          role
                        }
                      }
                    `,
                    { id: user.id, input },
                  );
                  onDone();
                  succeed(t('admin.saved'));
                }, fail)
              }
              onToggle={() =>
                gate.run(async () => {
                  if (user.disabled) {
                    await mutate<usersEnableMutation>(
                      environment,
                      graphql`
                        mutation usersEnableMutation($id: ID!) {
                          enableUser(id: $id) {
                            id
                            disabled
                          }
                        }
                      `,
                      { id: user.id },
                    );
                  } else {
                    await mutate<usersDisableMutation>(
                      environment,
                      graphql`
                        mutation usersDisableMutation($id: ID!) {
                          disableUser(id: $id) {
                            id
                            disabled
                          }
                        }
                      `,
                      { id: user.id },
                    );
                  }
                  succeed(t('admin.saved'));
                }, fail)
              }
              onResetPassword={(password, onDone) =>
                gate.run(async () => {
                  await mutate<usersResetPasswordMutation>(
                    environment,
                    graphql`
                      mutation usersResetPasswordMutation($id: ID!, $password: String!) {
                        resetUserPassword(id: $id, password: $password) {
                          id
                        }
                      }
                    `,
                    { id: user.id, password },
                  );
                  onDone();
                  succeed(t('admin.users.passwordReset', { username: user.username }));
                }, fail)
              }
            />
          ))}
        </Stack>
      </Card>

      <CreateUser
        onCreate={(input, onDone) =>
          gate.run(async () => {
            const response = await mutate<usersCreateMutation>(
              environment,
              graphql`
                mutation usersCreateMutation($input: CreateUserInput!) {
                  createUser(input: $input) {
                    id
                    username
                  }
                }
              `,
              { input },
            );
            onDone();
            succeed(t('admin.users.created', { username: response.createUser.username }));
          }, fail)
        }
      />
    </Stack>
  );
}

type UserItem = {
  id: string;
  username: string;
  displayName: string;
  role: Role;
  disabled: boolean;
  createdAt: string;
};

function useRoleOptions() {
  const { t } = useTranslation();

  return roles.map((role) => ({ value: role, label: t(`admin.users.roles.${role}`) }));
}

// UserRow は、ユーザー 1 人の行。表示名とロールの変更、無効と有効の切り替え、パスワードの設定し直しができる。
// 自分自身は、無効にできないので、ボタンを出さない。
function UserRow({
  user,
  self,
  onUpdate,
  onToggle,
  onResetPassword,
}: {
  user: UserItem;
  self: boolean;
  onUpdate: (input: { displayName: string; role: Role }, onDone: () => void) => void;
  onToggle: () => void;
  onResetPassword: (password: string, onDone: () => void) => void;
}) {
  const { t } = useTranslation();
  const roleOptions = useRoleOptions();
  const [mode, setMode] = useState<'view' | 'edit' | 'password'>('view');
  const [displayName, setDisplayName] = useState(user.displayName);
  const [role, setRole] = useState<Role>(user.role);
  const [password, setPassword] = useState('');
  // close は、入力を捨てて、行を閉じる。やめたときと、保存したときに使う。
  const close = () => {
    setDisplayName(user.displayName);
    setRole(user.role);
    setPassword('');
    setMode('view');
  };
  // サーバーが新しいロールを足したときは、名前をそのまま出す。
  const knownRole = roles.find((known) => known === user.role);

  return (
    <Stack gap={4} data-user={user.username}>
      <Group justify="space-between" wrap="nowrap" align="flex-start">
        <Stack gap={0} miw={0} flex={1}>
          <Group gap="xs">
            <Text fw={600} truncate>
              {user.displayName}
            </Text>
            <Text size="sm" c="dimmed">
              @{user.username}
            </Text>
            <Badge size="sm" variant="light" color={user.role === 'ADMIN' ? 'orange' : 'gray'}>
              {knownRole === undefined ? user.role : t(`admin.users.roles.${knownRole}`)}
            </Badge>
            {self && (
              <Badge size="sm" variant="outline">
                {t('admin.users.you')}
              </Badge>
            )}
            {user.disabled && (
              <Badge size="sm" variant="light" color="red">
                {t('admin.users.disabled')}
              </Badge>
            )}
          </Group>
          <Text size="xs" c="dimmed">
            {t('admin.users.createdAt', { at: dateFormat.format(Date.parse(user.createdAt)) })}
          </Text>
        </Stack>
        {mode === 'view' && (
          <Group gap="xs" justify="flex-end" flex="none" maw="60%">
            <Button size="compact-sm" variant="default" onClick={() => setMode('edit')}>
              {t('admin.users.edit')}
            </Button>
            <Button size="compact-sm" variant="default" onClick={() => setMode('password')}>
              {t('admin.users.resetPassword')}
            </Button>
            {!self && (
              <Button
                size="compact-sm"
                variant="default"
                color={user.disabled ? undefined : 'red'}
                onClick={onToggle}
              >
                {user.disabled ? t('admin.users.enable') : t('admin.users.disable')}
              </Button>
            )}
          </Group>
        )}
      </Group>
      {mode === 'edit' && (
        <Group gap="xs" align="flex-end">
          <TextInput
            size="xs"
            label={t('admin.users.displayName')}
            value={displayName}
            onChange={(event) => setDisplayName(event.currentTarget.value)}
          />
          <Select
            size="xs"
            w={140}
            label={t('admin.users.role')}
            data={roleOptions}
            value={role}
            allowDeselect={false}
            onChange={(value) => value !== null && setRole(value as Role)}
          />
          <Button
            size="compact-sm"
            disabled={displayName.trim() === ''}
            onClick={() => onUpdate({ displayName: displayName.trim(), role }, close)}
          >
            {t('admin.save')}
          </Button>
          <Button size="compact-sm" variant="subtle" onClick={close}>
            {t('admin.users.cancel')}
          </Button>
        </Group>
      )}
      {mode === 'password' && (
        <Stack gap={4}>
          <Group gap="xs" align="flex-end">
            <PasswordInput
              size="xs"
              w={240}
              label={t('admin.users.newPassword')}
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.currentTarget.value)}
            />
            <Button
              size="compact-sm"
              disabled={password === ''}
              onClick={() => onResetPassword(password, close)}
            >
              {t('admin.save')}
            </Button>
            <Button size="compact-sm" variant="subtle" onClick={close}>
              {t('admin.users.cancel')}
            </Button>
          </Group>
          <PasswordStrength password={password} userInputs={[user.username, user.displayName]} />
        </Stack>
      )}
    </Stack>
  );
}

// CreateUser は、ユーザーを作るフォーム。パスワードを省くと、パスワードを持たないユーザーにする。
function CreateUser({
  onCreate,
}: {
  onCreate: (
    input: { username: string; displayName: string; role: Role; password: string | null },
    onDone: () => void,
  ) => void;
}) {
  const { t } = useTranslation();
  const roleOptions = useRoleOptions();
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [role, setRole] = useState<Role>('MEMBER');
  const [password, setPassword] = useState('');

  const submit = (event: FormEvent) => {
    event.preventDefault();
    // 空白だけのユーザー名は、required を通っても送らない。
    if (username.trim() === '') {
      return;
    }
    onCreate(
      {
        username: username.trim(),
        displayName: displayName.trim() || username.trim(),
        role,
        password: password === '' ? null : password,
      },
      () => {
        setUsername('');
        setDisplayName('');
        setRole('MEMBER');
        setPassword('');
      },
    );
  };

  return (
    <Card withBorder>
      <form onSubmit={submit}>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.users.create')}
          </Title>
          <Group gap="xs" align="flex-start">
            <TextInput
              size="xs"
              label={t('admin.users.username')}
              required
              autoComplete="off"
              value={username}
              onChange={(event) => setUsername(event.currentTarget.value)}
            />
            <TextInput
              size="xs"
              label={t('admin.users.displayName')}
              placeholder={username}
              value={displayName}
              onChange={(event) => setDisplayName(event.currentTarget.value)}
            />
            <Select
              size="xs"
              w={140}
              label={t('admin.users.role')}
              data={roleOptions}
              value={role}
              allowDeselect={false}
              onChange={(value) => value !== null && setRole(value as Role)}
            />
          </Group>
          <Text size="xs" c="dimmed">
            {t('admin.users.roleHelp')}
          </Text>
          <PasswordInput
            size="xs"
            maw={320}
            label={t('admin.users.password')}
            description={t('admin.users.passwordHelp')}
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.currentTarget.value)}
          />
          {password !== '' && (
            <PasswordStrength password={password} userInputs={[username, displayName]} />
          )}
          <Group>
            <Button type="submit" size="compact-sm" disabled={username.trim() === ''}>
              {t('admin.users.create')}
            </Button>
          </Group>
        </Stack>
      </form>
    </Card>
  );
}
