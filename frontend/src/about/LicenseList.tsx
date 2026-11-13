import { Accordion, Anchor, Badge, Code, Group, Stack, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { SoftwareEntry } from './licenses';

// LicenseList は、ソフトウェアとデータの一覧を、開くとライセンスの文書が出る行で並べる。
// 文書は合わせて数百 KB になるので、開いた行の文書だけを描く。
export function LicenseList({ entries }: { entries: readonly SoftwareEntry[] }) {
  const { t } = useTranslation();
  const [opened, setOpened] = useState<string[]>([]);

  if (entries.length === 0) {
    return (
      <Text size="sm" c="dimmed">
        {t('about.noMatch')}
      </Text>
    );
  }

  return (
    <Accordion multiple variant="separated" value={opened} onChange={setOpened}>
      {entries.map((entry) => {
        const key = `${entry.name}@${entry.version ?? ''}`;

        return (
          <Accordion.Item key={key} value={key}>
            <Accordion.Control>
              <Group gap="xs" wrap="wrap">
                <Text size="sm" fw={500}>
                  {entry.name}
                </Text>
                {entry.version !== null && (
                  <Text size="xs" c="dimmed">
                    {entry.version}
                  </Text>
                )}
                {entry.license !== '' && (
                  <Badge size="sm" variant="light">
                    {entry.license}
                  </Badge>
                )}
              </Group>
            </Accordion.Control>
            <Accordion.Panel>
              {opened.includes(key) && (
                <Stack gap="xs">
                  {entry.url !== null && (
                    <Anchor href={entry.url} target="_blank" rel="noopener noreferrer" size="sm">
                      {entry.url}
                    </Anchor>
                  )}
                  {entry.note !== null && (
                    <Text size="sm" c="dimmed">
                      {entry.note}
                    </Text>
                  )}
                  {entry.licenses.map((file) => (
                    <Stack key={file.name} gap={4}>
                      <Text size="xs" c="dimmed">
                        {file.name}
                      </Text>
                      <Code block style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                        {file.text}
                      </Code>
                    </Stack>
                  ))}
                </Stack>
              )}
            </Accordion.Panel>
          </Accordion.Item>
        );
      })}
    </Accordion>
  );
}
