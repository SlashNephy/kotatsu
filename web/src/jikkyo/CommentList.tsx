import {
  ActionIcon,
  Button,
  CopyButton,
  Group,
  ScrollArea,
  Stack,
  Text,
  Tooltip,
} from '@mantine/core';
import { Check, Copy, List } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

// CommentRow は、一覧に出す 1 行。label は、行の先頭に添える時刻。
export type CommentRow = { key: number | string; label: string; content: string };

// stickThreshold は、一覧の末尾からこの画素の内にいれば、新しいコメントが届いたときに末尾へ追う範囲。
// 読み返すために上へ戻している間は、追わない。
const stickThreshold = 48;

// CommentList は、実況のコメントの一覧 (#489、設計書 7.4)。新しいものを下に足し、各行からコメントをコピーできる。
export function CommentList({ rows }: { rows: readonly CommentRow[] }) {
  const { t } = useTranslation();
  const viewport = useRef<HTMLDivElement | null>(null);
  const stick = useRef(true);
  const last = rows.at(-1)?.key;
  useEffect(() => {
    const element = viewport.current;
    if (element !== null && stick.current) {
      element.scrollTop = element.scrollHeight;
    }
  }, [last]);

  return (
    <ScrollArea
      h={240}
      type="auto"
      viewportRef={viewport}
      onScrollPositionChange={() => {
        const element = viewport.current;
        if (element !== null) {
          stick.current =
            element.scrollHeight - element.scrollTop - element.clientHeight < stickThreshold;
        }
      }}
      aria-label={t('jikkyo.list')}
    >
      {rows.length === 0 ? (
        <Text size="sm" c="dimmed" p="xs">
          {t('jikkyo.listEmpty')}
        </Text>
      ) : (
        <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
          {rows.map((row) => (
            <li key={row.key}>
              <Group gap="xs" wrap="nowrap" px="xs" py={2}>
                <Text size="xs" c="dimmed" ff="monospace" style={{ flexShrink: 0 }}>
                  {row.label}
                </Text>
                <Text size="sm" style={{ flex: 1, overflowWrap: 'anywhere' }}>
                  {row.content}
                </Text>
                <CopyButton value={row.content}>
                  {({ copied, copy }) => (
                    <Tooltip
                      label={copied ? t('jikkyo.copied') : t('jikkyo.copy')}
                      withinPortal={false}
                    >
                      <ActionIcon
                        size="sm"
                        variant="subtle"
                        color="gray"
                        aria-label={t('jikkyo.copyComment', { content: row.content })}
                        onClick={copy}
                      >
                        {copied ? <Check size={14} aria-hidden /> : <Copy size={14} aria-hidden />}
                      </ActionIcon>
                    </Tooltip>
                  )}
                </CopyButton>
              </Group>
            </li>
          ))}
        </ul>
      )}
    </ScrollArea>
  );
}

// CommentListToggle は、コメントの一覧を開閉するボタンと、一覧。場所を取るので、既定では閉じる。
// rows は、開いている間だけ求める。
export function CommentListToggle({ rows }: { rows: () => readonly CommentRow[] }) {
  const { t } = useTranslation();
  const [opened, setOpened] = useState(false);

  return (
    <Stack gap="xs">
      <Group>
        <Button
          size="compact-sm"
          variant="default"
          leftSection={<List size={14} aria-hidden />}
          aria-expanded={opened}
          onClick={() => setOpened(!opened)}
        >
          {opened ? t('jikkyo.hideList') : t('jikkyo.showList')}
        </Button>
      </Group>
      {opened && <CommentList rows={rows()} />}
    </Stack>
  );
}
