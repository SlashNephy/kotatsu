import {
  ActionIcon,
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
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { UpdatedAt } from '../../admin/UpdatedAt';
import { AutoSaveTextInput } from '../../settings/AutoSaveFields';
import { mutate } from '../../settings/mutate';
import { useAutoSave } from '../../settings/useAutoSave';
import { maxDRCSText, validDRCSText } from '../../yunomi/drcs';
import { settingsErrorMessage } from '../../settings/errors';
import type { captionsBackfillMutation } from './__generated__/captionsBackfillMutation.graphql';
import type { captionsBackfillQuery } from './__generated__/captionsBackfillQuery.graphql';
import type { captionsMutation } from './__generated__/captionsMutation.graphql';
import type { captionsQuery } from './__generated__/captionsQuery.graphql';

export const Route = createFileRoute('/admin/captions')({
  component: CaptionsPage,
});

// CaptionsPage は、字幕を扱う、管理者向けの画面 (#825、#2218、設計書 7.4、10.3)。
// 字幕の外字 (DRCS) を文字に置き換えるテーブルを変え、既にある録画の放送の字幕の文を取り出す。
// 外字は、字幕を見ている画面の字幕の履歴からも登録できる。ここでは、登録したテーブルを見直し、修正したり消したりする。
function CaptionsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.captions.title')}>
      <Stack gap="md">
        <DRCSReplacements />
        <ARIBCaptionBackfill />
      </Stack>
    </AdminPage>
  );
}

const backfillQuery = graphql`
  query captionsBackfillQuery {
    aribCaptionBackfill {
      pending
      queued
    }
  }
`;

// ARIBCaptionBackfill は、放送の字幕の文 (ARIB 字幕) を取り出していない録画の数を出し、取り出すジョブをまとめて入れる (#2218)。
// 取り出すジョブを足す前に録った録画も、字幕の文で探せるようにするためである。kotatsu admin extract-arib-captions と同じ操作。
function ARIBCaptionBackfill() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<captionsBackfillQuery>(
    backfillQuery,
    {},
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const { pending, queued } = data.aribCaptionBackfill;
  // queuing は、入れている途中かどうか。続けて押して、重ねて送らないようにする。
  const [queuing, setQueuing] = useState(false);
  // result は、最後に押した結果。入れた数か、失敗したときの文。
  const [result, setResult] = useState<{ queued: number } | { error: string } | null>(null);

  const extract = () => {
    setQueuing(true);
    setResult(null);
    mutate<captionsBackfillMutation>(
      environment,
      graphql`
        mutation captionsBackfillMutation {
          extractARIBCaptions
        }
      `,
      {},
    )
      .then((response) => setResult({ queued: response.extractARIBCaptions }))
      .catch((error: unknown) => setResult({ error: settingsErrorMessage(t, error) }))
      .finally(() => {
        setQueuing(false);
        startTransition(() => setFetchKey((key) => key + 1));
      });
  };

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('admin.captions.backfill')}
        </Title>
        <Text size="sm" c="dimmed">
          {t('admin.captions.backfillHelp')}
        </Text>
        <Text size="sm">
          {t('admin.captions.backfillPending', { count: pending })}
          {queued > 0 && t('admin.captions.backfillWaiting', { count: queued })}
        </Text>
        <Group gap="xs">
          {/* ジョブを待っていない録画がなければ、押しても入れるものがないので押させない。 */}
          <Button
            size="xs"
            variant="default"
            disabled={pending - queued <= 0 || queuing}
            onClick={extract}
          >
            {t('admin.captions.backfillRun')}
          </Button>
        </Group>
        {result !== null &&
          ('error' in result ? (
            <Text size="sm" c="red" role="alert">
              {result.error}
            </Text>
          ) : (
            <Text size="sm" role="status">
              {t('admin.captions.backfillQueued', { count: result.queued })}
            </Text>
          ))}
      </Stack>
    </Card>
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

// DRCSReplacements は、外字の置き換えのテーブル。置き換える文字は欄を離れたときか Enter で、消すのと足すのは押したらすぐ、
// その外字だけを保存する (#1740)。テーブルを丸ごと送らないのは、ほかの画面 (字幕の履歴) や管理者が登録したものを、
// 古いテーブルで上書きして消さないためである。
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
  const { save } = useAutoSave();
  const settings = data.systemSettings;
  const rows = settings.drcsReplacements;
  const [md5, setMD5] = useState('');
  const [text, setText] = useState('');
  // adding は、足している途中かどうか。続けて押して、同じ外字を重ねて送らないようにする。
  const [adding, setAdding] = useState(false);
  const newValid =
    md5Pattern.test(md5) &&
    validDRCSText(text) &&
    !rows.some((row) => row.md5 === md5.toLowerCase());
  const textError = (value: string) =>
    validDRCSText(value) ? null : t('admin.captions.textInvalid', { max: maxDRCSText });

  // saveChange は、外字 1 つの置き換えを保存する。text が null なら、消す。
  const saveChange = (change: { md5: string; text: string | null }) =>
    save(async () => {
      await mutate<captionsMutation>(
        environment,
        graphql`
          mutation captionsMutation($input: SystemSettingsInput!) {
            updateSystemSettings(input: $input) {
              updatedAt
            }
          }
        `,
        { input: { drcsReplacements: [change] } },
      );
      startTransition(() => setFetchKey((key) => key + 1));
    });

  const add = () => {
    setAdding(true);
    void saveChange({ md5: md5.toLowerCase(), text }).then((ok) => {
      setAdding(false);
      // 足せなかったら、入れた値を残し、やり直せるようにする。
      if (ok) {
        setMD5('');
        setText('');
      }
    });
  };

  return (
    <Stack gap="md">
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
                {rows.map((row) => (
                  <Table.Tr key={row.md5}>
                    <Table.Td>
                      <Code fz="xs">{row.md5}</Code>
                    </Table.Td>
                    <Table.Td>
                      <AutoSaveTextInput
                        size="xs"
                        w={160}
                        aria-label={t('admin.captions.text')}
                        trim={false}
                        validate={textError}
                        value={row.text}
                        onSave={(value) => saveChange({ md5: row.md5, text: value })}
                      />
                    </Table.Td>
                    <Table.Td>
                      <ActionIcon
                        variant="subtle"
                        color="red"
                        aria-label={t('admin.captions.remove')}
                        onClick={() => void saveChange({ md5: row.md5, text: null })}
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
            <Button size="xs" variant="default" disabled={!newValid || adding} onClick={add}>
              {t('admin.captions.add')}
            </Button>
          </Group>
        </Stack>
      </Card>

      <UpdatedAt updatedAt={settings.updatedAt} />
    </Stack>
  );
}
