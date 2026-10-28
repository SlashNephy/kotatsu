import { ActionIcon, Box, Group, Menu, Paper, ScrollArea, Stack, Text } from '@mantine/core';
import { useClipboard } from '@mantine/hooks';
import { Copy, EllipsisVertical, IdCard, UserX } from 'lucide-react';
import type { ReactNode } from 'react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

// CommentRow は、一覧に出す 1 行。label は、行の先頭に添える時刻。userId は、投稿したユーザーの匿名の ID (分からなければ省く)。
export type CommentRow = { key: number | string; label: string; content: string; userId?: string };

// stickThreshold は、一覧の末尾からこの画素の内にいれば、新しいコメントが届いたときに末尾へ追う範囲。
// 読み返すために上へ戻している間は、追わない。
const stickThreshold = 48;

type ListProps = {
  rows: readonly CommentRow[];
  // onMuteUser は、行から、そのコメントを書いたユーザーをミュートする (#767)。渡さなければ、メニューに出さない。
  onMuteUser?: (userId: string) => void;
  // height は、一覧の高さ。CSS の長さで渡す。
  height?: string | number;
};

// CommentList は、実況のコメントの一覧 (#489、設計書 7.4)。新しいものを下に足す。
// 各行のメニューから、コメントとユーザー ID をコピーし、そのユーザーをミュートできる (#767)。
export function CommentList({ rows, onMuteUser, height = 240 }: ListProps) {
  const { t } = useTranslation();
  const clipboard = useClipboard();
  // copiedKey は、最後にコピーした行。コピーしたことを、その行にしばらく出す。
  const [copiedKey, setCopiedKey] = useState<CommentRow['key'] | null>(null);
  const copy = (key: CommentRow['key'], text: string) => {
    clipboard.copy(text);
    setCopiedKey(key);
  };
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
      h={height}
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
                {clipboard.copied && copiedKey === row.key && (
                  <Text size="xs" c="dimmed" role="status" style={{ flexShrink: 0 }}>
                    {t('jikkyo.copied')}
                  </Text>
                )}
                <Menu position="bottom-end">
                  <Menu.Target>
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="gray"
                      aria-label={t('jikkyo.rowMenu', { content: row.content })}
                    >
                      <EllipsisVertical size={14} aria-hidden />
                    </ActionIcon>
                  </Menu.Target>
                  <Menu.Dropdown>
                    <Menu.Item
                      leftSection={<Copy size={14} aria-hidden />}
                      onClick={() => copy(row.key, row.content)}
                    >
                      {t('jikkyo.copy')}
                    </Menu.Item>
                    {row.userId !== undefined && (
                      <Menu.Item
                        leftSection={<IdCard size={14} aria-hidden />}
                        onClick={() => copy(row.key, row.userId!)}
                      >
                        {t('jikkyo.copyUserId')}
                      </Menu.Item>
                    )}
                    {row.userId !== undefined && onMuteUser !== undefined && (
                      <Menu.Item
                        color="red"
                        leftSection={<UserX size={14} aria-hidden />}
                        onClick={() => onMuteUser(row.userId!)}
                      >
                        {t('jikkyo.muteUser')}
                      </Menu.Item>
                    )}
                  </Menu.Dropdown>
                </Menu>
              </Group>
            </li>
          ))}
        </ul>
      )}
    </ScrollArea>
  );
}

// CommentPanel は、右の列に置く実況の枠 (設計書 7.4)。見出しと、流れていったコメントの一覧を、常に開いて出す。
// footer は、枠の下端に付けるもの (ライブ視聴の投稿の欄)。
export function CommentPanel({
  rows,
  onMuteUser,
  height,
  footer,
}: {
  rows: readonly CommentRow[];
  onMuteUser?: (userId: string) => void;
  height: string;
  footer?: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <Paper withBorder radius="md">
      <Stack gap={0}>
        <Text size="sm" fw={600} px="sm" py="xs">
          {t('jikkyo.toggle')}
        </Text>
        <CommentList
          rows={rows}
          height={height}
          {...(onMuteUser === undefined ? {} : { onMuteUser })}
        />
        {footer !== undefined && (
          <Box p="xs" style={{ borderTop: '1px solid var(--mantine-color-default-border)' }}>
            {footer}
          </Box>
        )}
      </Stack>
    </Paper>
  );
}
