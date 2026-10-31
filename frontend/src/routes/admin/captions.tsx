import {
  ActionIcon,
  Alert,
  Button,
  Card,
  Code,
  Group,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Trash2 } from 'lucide-react';
import { startTransition, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { useAction } from '../../admin/useAction';
import { SaveRow } from '../../admin/SaveRow';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import { validDRCSText } from '../../yunomi/drcs';
import type { captionsMutation } from './__generated__/captionsMutation.graphql';
import type { captionsQuery } from './__generated__/captionsQuery.graphql';

export const Route = createFileRoute('/admin/captions')({
  component: CaptionsPage,
});

// CaptionsPage は、字幕の外字 (DRCS) を文字に置き換える表を変える、管理者向けの画面 (#825、設計書 7.4、10.3)。
// 外字は、字幕を見ている画面の字幕の履歴からも登録できる。ここでは、登録した表を見直し、直したり消したりする。
function CaptionsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.captions.title')}>
      <DRCSReplacements />
    </AdminPage>
  );
}

const query = graphql`
  query captionsQuery {
    systemSettings {
      drcsReplacements {
        md5
        text
      }
      updatedAt
    }
  }
`;

const md5Pattern = /^[0-9a-f]{32}$/i;

function DRCSReplacements() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<captionsQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const action = useAction();
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const settings = data.systemSettings;
  const [rows, setRows] = useState(() => settings.drcsReplacements.map((row) => ({ ...row })));
  const [md5, setMD5] = useState('');
  const [text, setText] = useState('');
  const newValid =
    md5Pattern.test(md5) &&
    validDRCSText(text) &&
    !rows.some((row) => row.md5 === md5.toLowerCase());
  const valid = rows.every((row) => validDRCSText(row.text));

  const add = () => {
    setRows((current) =>
      [...current, { md5: md5.toLowerCase(), text }].sort((a, b) => a.md5.localeCompare(b.md5)),
    );
    setMD5('');
    setText('');
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setMessage(null);
    action.run(
      async () => {
        await mutate<captionsMutation>(
          environment,
          graphql`
            mutation captionsMutation($input: SystemSettingsInput!) {
              updateSystemSettings(input: $input) {
                updatedAt
              }
            }
          `,
          { input: { drcsReplacements: rows } },
        );
        setMessage({ ok: true, text: t('admin.saved') });
        startTransition(() => setFetchKey((key) => key + 1));
      },
      (caught) => setMessage({ ok: false, text: settingsErrorMessage(t, caught) }),
    );
  };

  return (
    <form onSubmit={submit}>
      <Stack gap="md">
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
            <Title order={3} size="h5">
              {t('admin.captions.drcs')}
            </Title>
            <Text size="sm" c="dimmed">
              {t('admin.captions.drcsHelp')}
            </Text>
            {rows.length === 0 ? (
              <Text size="sm" c="dimmed">
                {t('admin.captions.empty')}
              </Text>
            ) : (
              <Table>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>{t('admin.captions.md5')}</Table.Th>
                    <Table.Th>{t('admin.captions.text')}</Table.Th>
                    <Table.Th />
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {rows.map((row, index) => (
                    <Table.Tr key={row.md5}>
                      <Table.Td>
                        <Code fz="xs">{row.md5}</Code>
                      </Table.Td>
                      <Table.Td>
                        <TextInput
                          size="xs"
                          w={100}
                          aria-label={t('admin.captions.text')}
                          value={row.text}
                          error={!validDRCSText(row.text)}
                          onChange={(event) => {
                            const value = event.currentTarget.value;
                            setRows((current) =>
                              current.map((item, i) =>
                                i === index ? { ...item, text: value } : item,
                              ),
                            );
                          }}
                        />
                      </Table.Td>
                      <Table.Td>
                        <ActionIcon
                          variant="subtle"
                          color="red"
                          aria-label={t('admin.captions.remove')}
                          onClick={() =>
                            setRows((current) => current.filter((_, i) => i !== index))
                          }
                        >
                          <Trash2 size={16} />
                        </ActionIcon>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
            <Group gap="xs" align="flex-end">
              <TextInput
                size="xs"
                w={300}
                label={t('admin.captions.md5')}
                value={md5}
                onChange={(event) => setMD5(event.currentTarget.value.trim())}
              />
              <TextInput
                size="xs"
                w={100}
                label={t('admin.captions.text')}
                value={text}
                onChange={(event) => setText(event.currentTarget.value)}
              />
              <Button size="xs" variant="default" disabled={!newValid} onClick={add}>
                {t('admin.captions.add')}
              </Button>
            </Group>
          </Stack>
        </Card>

        <SaveRow updatedAt={settings.updatedAt} disabled={!valid || action.busy} />
      </Stack>
    </form>
  );
}
