import { ActionIcon, Group, Menu, ScrollArea, Text } from '@mantine/core';
import { useClipboard } from '@mantine/hooks';
import { Copy, EllipsisVertical, IdCard, UserX } from 'lucide-react';
import { memo, useEffect, useRef, useState } from 'react';
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
  // frozen の間は、rows が変わっても描き直さず、最後に描いた行のまま置く。全画面で一覧が見えない間に使う。
  frozen?: boolean;
};

// CommentList は、実況のコメントの一覧 (#489、設計書 7.4)。新しいものを下に足す。
// 各行のメニューから、コメントとユーザー ID をコピーし、そのユーザーをミュートできる (#767)。
// 一覧は最大 500 行あり、コメントが届くたびに rows が変わる。描き直すのは、足した行と消えた行だけにする。
export function CommentList({ rows, onMuteUser, height = 240, frozen = false }: ListProps) {
  const { t } = useTranslation();
  const clipboard = useClipboard();
  // copiedKey は、最後にコピーした行。コピーしたことを、その行にしばらく出す。
  const [copiedKey, setCopiedKey] = useState<CommentRow['key'] | null>(null);
  // shown は、描いている行。frozen の間は、前に描いた行のまま置く。
  const [shown, setShown] = useState(rows);
  if (!frozen && shown !== rows) {
    setShown(rows);
  }
  // 行に渡す関数は、描き直しても変えない。変えると、すべての行を描き直す。
  // 行から呼ぶ関数は ref から読み、描き直したときの新しい関数を使う。
  const latest = useRef({ copy: clipboard.copy, mute: onMuteUser });
  useEffect(() => {
    latest.current = { copy: clipboard.copy, mute: onMuteUser };
  });
  const [actions] = useState<RowActions>(() => ({
    copy: (key, text) => {
      latest.current.copy(text);
      setCopiedKey(key);
    },
    mute: (userId) => latest.current.mute?.(userId),
  }));
  const canMute = onMuteUser !== undefined;
  const viewport = useRef<HTMLDivElement | null>(null);
  const stick = useRef(true);
  const last = shown.at(-1)?.key;
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
      {shown.length === 0 ? (
        <Text size="sm" c="dimmed" p="xs">
          {t('jikkyo.listEmpty')}
        </Text>
      ) : (
        <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
          {shown.map((row) => (
            <CommentListRow
              key={row.key}
              rowKey={row.key}
              label={row.label}
              content={row.content}
              userId={row.userId}
              copied={clipboard.copied && copiedKey === row.key}
              canMute={canMute}
              actions={actions}
            />
          ))}
        </ul>
      )}
    </ScrollArea>
  );
}

// RowActions は、行のメニューの操作。一覧が一度だけ作り、すべての行で同じものを使う。
type RowActions = {
  copy: (key: CommentRow['key'], text: string) => void;
  mute: (userId: string) => void;
};

type RowProps = {
  rowKey: CommentRow['key'];
  label: string;
  content: string;
  userId: string | undefined;
  // copied は、この行をコピーしたことを出すかどうか。
  copied: boolean;
  canMute: boolean;
  actions: RowActions;
};

// CommentListRow は、一覧の 1 行。props が変わらなければ、一覧を描き直しても描き直さない。
const CommentListRow = memo(function CommentListRow({
  rowKey,
  label,
  content,
  userId,
  copied,
  canMute,
  actions,
}: RowProps) {
  const { t } = useTranslation();

  return (
    <li>
      <Group gap="xs" wrap="nowrap" px="xs" py={2}>
        <Text size="xs" c="dimmed" ff="monospace" style={{ flexShrink: 0 }}>
          {label}
        </Text>
        <Text size="sm" style={{ flex: 1, overflowWrap: 'anywhere' }}>
          {content}
        </Text>
        {copied && (
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
              aria-label={t('jikkyo.rowMenu', { content })}
            >
              <EllipsisVertical size={14} aria-hidden />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item
              leftSection={<Copy size={14} aria-hidden />}
              onClick={() => actions.copy(rowKey, content)}
            >
              {t('jikkyo.copy')}
            </Menu.Item>
            {userId !== undefined && (
              <Menu.Item
                leftSection={<IdCard size={14} aria-hidden />}
                onClick={() => actions.copy(rowKey, userId)}
              >
                {t('jikkyo.copyUserId')}
              </Menu.Item>
            )}
            {userId !== undefined && canMute && (
              <Menu.Item
                color="red"
                leftSection={<UserX size={14} aria-hidden />}
                onClick={() => actions.mute(userId)}
              >
                {t('jikkyo.muteUser')}
              </Menu.Item>
            )}
          </Menu.Dropdown>
        </Menu>
      </Group>
    </li>
  );
});
