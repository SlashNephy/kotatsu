import { Alert, Button, Group, Modal, Stack, Text, TextInput, Textarea } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { saveBlob } from '../../yunomi/capture';
import { fitURL, fullReport, reportFileName, urlLimit, type IssueFields } from './report';

// IssueDialog は、伏せた後の各欄をそのまま見せる確認の窓 (#1521)。利用者は、どの欄も書き換えたり消したりできる。
// 「GitHub で開く」は、利用者のブラウザで Issue のフォームを開くだけで、サーバーからは何も送らない。
// 「コピー」と「ダウンロード」は、ログを 1 行も削らない全部入りの報告にする。
export function IssueDialog({
  initial,
  full,
  onClose,
}: {
  // initial は、URL の上限に入るように選んだ欄。
  initial: IssueFields;
  // full は、ログを削らない全部入りの欄。
  full: IssueFields;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [fields, setFields] = useState(initial);
  const [copied, setCopied] = useState(false);
  const { url, trimmed } = fitURL(fields);
  const set = (key: keyof IssueFields) => (event: { currentTarget: { value: string } }) =>
    setFields({ ...fields, [key]: event.currentTarget.value });
  // 全部入りの報告は、ログ以外の欄だけ、書き換えたものを使う。
  const report = fullReport({
    ...full,
    title: fields.title,
    version: fields.version,
    environment: fields.environment,
  });

  const copy = async () => {
    await navigator.clipboard.writeText(report);
    setCopied(true);
  };
  const download = () =>
    saveBlob(
      new Blob([report], { type: 'text/markdown;charset=utf-8' }),
      reportFileName(new Date()),
    );

  return (
    <Modal opened onClose={onClose} title={t('admin.logs.issue.title')} size="xl">
      <Stack gap="sm">
        <Text size="sm">{t('admin.logs.issue.description')}</Text>
        <TextInput
          label={t('admin.logs.issue.fieldTitle')}
          value={fields.title}
          onChange={set('title')}
        />
        <TextInput
          label={t('admin.logs.issue.fieldVersion')}
          value={fields.version}
          onChange={set('version')}
        />
        <Textarea
          label={t('admin.logs.issue.fieldEnvironment')}
          value={fields.environment}
          onChange={set('environment')}
          autosize
          minRows={3}
          maxRows={8}
        />
        <Textarea
          label={t('admin.logs.issue.fieldLogs')}
          value={fields.logs}
          onChange={set('logs')}
          autosize
          minRows={6}
          maxRows={16}
          styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12 } }}
          data-testid="issue-logs"
        />
        <Text size="xs" c="dimmed">
          {t('admin.logs.issue.length', { length: url.length, limit: urlLimit })}
        </Text>
        {trimmed > 0 && (
          <Alert color="amber" p="xs">
            <Text size="sm">{t('admin.logs.issue.trimmed', { count: trimmed })}</Text>
          </Alert>
        )}
        <Group justify="space-between">
          <Group gap="xs">
            <Button variant="default" onClick={() => void copy()}>
              {copied ? t('admin.logs.issue.copied') : t('admin.logs.issue.copy')}
            </Button>
            <Button variant="default" onClick={download}>
              {t('admin.logs.issue.download')}
            </Button>
          </Group>
          <Group gap="xs">
            <Button variant="default" onClick={onClose}>
              {t('admin.logs.issue.cancel')}
            </Button>
            <Button
              component="a"
              href={url}
              target="_blank"
              rel="noopener noreferrer"
              data-testid="issue-open"
            >
              {t('admin.logs.issue.open')}
            </Button>
          </Group>
        </Group>
      </Stack>
    </Modal>
  );
}
