import { ActionIcon, Popover, TextInput, Tooltip } from '@mantine/core';
import { useNavigate } from '@tanstack/react-router';
import { Search } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { emptyConditions, emptyKeyword } from '../conditions/form';

// HeaderSearch は、ヘッダーに出す、番組検索の入り口 (#1627)。
// 場所を取らないよう虫眼鏡のアイコンにし、押すと入力欄をポップオーバーで出す。Enter で、打った言葉を番組名のキーワードにして
// 番組検索の画面へ移る。何も打たずに Enter を押したら、条件を入れずに番組検索の画面を開く。
// 通知のベル (NotificationBell) と同じく、名前はツールチップと読み上げで補う。
export function HeaderSearch() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [opened, setOpened] = useState(false);
  const [text, setText] = useState('');

  const submit = () => {
    const keyword = text.trim();
    setOpened(false);
    setText('');
    void navigate({
      to: '/search',
      state:
        keyword === ''
          ? {}
          : {
              searchConditions: {
                ...emptyConditions(),
                keywords: [{ ...emptyKeyword(), text: keyword }],
              },
              // 番組の詳細から同じ名前で探すときと違い、これから放送する番組を探す。過去の放送は、画面で含められる。
              searchArchive: false,
            },
    });
  };

  return (
    <Popover
      opened={opened}
      onChange={setOpened}
      position="bottom-end"
      width="min(320px, calc(100vw - 32px))"
      shadow="md"
      withinPortal
      trapFocus
    >
      <Tooltip label={t('nav.search')} disabled={opened}>
        <Popover.Target>
          <ActionIcon
            variant="subtle"
            color="gray"
            size="lg"
            aria-label={t('nav.search')}
            onClick={() => setOpened((value) => !value)}
          >
            <Search size={20} aria-hidden />
          </ActionIcon>
        </Popover.Target>
      </Tooltip>
      <Popover.Dropdown p="xs">
        <form
          role="search"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <TextInput
            value={text}
            onChange={(event) => setText(event.currentTarget.value)}
            placeholder={t('search.headerPlaceholder')}
            aria-label={t('search.headerLabel')}
            leftSection={<Search size={16} aria-hidden />}
            data-autofocus
          />
        </form>
      </Popover.Dropdown>
    </Popover>
  );
}
