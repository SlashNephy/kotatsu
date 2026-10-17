import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Select,
  Slider,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { usePlayerSettings, type CommentSettingsInput } from '../yunomi/usePlayerSettings';
import type { KeywordMuteMatch } from './comments';

const matches: readonly KeywordMuteMatch[] = ['PARTIAL', 'PREFIX', 'SUFFIX', 'EXACT', 'REGEX'];

const isMatch = (value: string): value is KeywordMuteMatch =>
  (matches as readonly string[]).includes(value);

// validRegExp は、正規表現として読めるかを返す。読めないものはミュートに使われないので、足す前に知らせる。
const validRegExp = (pattern: string) => {
  try {
    new RegExp(pattern, 'i');

    return true;
  } catch {
    return false;
  }
};

// CommentSettingsCard は、実況のコメントの表示とミュートの設定 (#488)。ユーザーごとに持ち、端末をまたいで同期する。
export function CommentSettingsCard() {
  const { t } = useTranslation();
  const { comments, setComments } = usePlayerSettings();
  const [failed, setFailed] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [match, setMatch] = useState<KeywordMuteMatch>('PARTIAL');
  const [user, setUser] = useState('');
  // スライダーは、動かしている間は手元の値を出し、離したときに保存する。保存した値が届くまでは、手元の値を出し続ける。
  // すぐに保存前の値へ戻すと、キーボードで続けて動かしたときに、戻った値から数え直して、動かした分が消える。
  const [draft, setDraft] = useState<{ opacity?: number; speed?: number; scale?: number }>({});
  useEffect(() => {
    setDraft((d) => {
      const next = { ...d };
      for (const key of ['opacity', 'speed', 'scale'] as const) {
        if (next[key] !== undefined && Math.abs(next[key] - comments[key]) < 1e-9) {
          delete next[key];
        }
      }

      return next;
    });
  }, [comments]);

  const current = (): CommentSettingsInput => ({
    opacity: comments.opacity,
    speed: comments.speed,
    scale: comments.scale,
    showAll: comments.showAll,
    mutedKeywords: comments.mutedKeywords.flatMap((k) =>
      isMatch(k.match) ? [{ text: k.text, match: k.match }] : [],
    ),
    mutedUsers: [...comments.mutedUsers],
  });
  const save = (change: Partial<CommentSettingsInput>) => {
    setFailed(false);
    setComments({ ...current(), ...change }, () => {
      setFailed(true);
      setDraft({});
    });
  };

  const keywordText = keyword.trim();
  const keywordInvalid = match === 'REGEX' && keywordText !== '' && !validRegExp(keywordText);
  const addKeyword = () => {
    if (keywordText === '' || keywordInvalid) {
      return;
    }
    save({ mutedKeywords: [...current().mutedKeywords, { text: keywordText, match }] });
    setKeyword('');
  };
  const userText = user.trim();
  const addUser = () => {
    if (userText === '' || comments.mutedUsers.includes(userText)) {
      return;
    }
    save({ mutedUsers: [...comments.mutedUsers, userText] });
    setUser('');
  };

  const slider = (
    key: 'opacity' | 'speed' | 'scale',
    min: number,
    max: number,
    format: (value: number) => string,
  ) => (
    <Stack gap={4}>
      <Group justify="space-between">
        <Text size="sm">{t(`settings.comments.${key}`)}</Text>
        <Text size="sm" c="dimmed">
          {format(draft[key] ?? comments[key])}
        </Text>
      </Group>
      <Slider
        aria-label={t(`settings.comments.${key}`)}
        min={min}
        max={max}
        step={0.05}
        value={draft[key] ?? comments[key]}
        label={null}
        onChange={(value) => setDraft((d) => ({ ...d, [key]: value }))}
        onChangeEnd={(value) => {
          const rounded = Math.round(value * 100) / 100;
          setDraft((d) => ({ ...d, [key]: rounded }));
          save({ [key]: rounded });
        }}
      />
    </Stack>
  );

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Title order={3} size="h5">
          {t('settings.comments.title')}
        </Title>
        {failed && (
          <Alert color="red" role="alert" withCloseButton onClose={() => setFailed(false)}>
            {t('settings.comments.saveError')}
          </Alert>
        )}
        {slider('opacity', 0.1, 1, (v) => `${Math.round(v * 100)}%`)}
        {slider('speed', 0.5, 2, (v) => `×${v.toFixed(2)}`)}
        {slider('scale', 0.5, 1.5, (v) => `×${v.toFixed(2)}`)}
        <Switch
          label={t('settings.comments.showAll')}
          description={t('settings.comments.showAllHelp')}
          checked={comments.showAll}
          onChange={(event) => save({ showAll: event.currentTarget.checked })}
        />

        <Stack gap={4}>
          <Text size="sm" fw={500}>
            {t('settings.comments.mutedKeywords')}
          </Text>
          <Group gap="xs" align="flex-start" wrap="nowrap">
            <TextInput
              aria-label={t('settings.comments.keyword')}
              placeholder={t('settings.comments.keyword')}
              value={keyword}
              maxLength={100}
              error={keywordInvalid ? t('settings.comments.invalidRegex') : undefined}
              onChange={(event) => setKeyword(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
                  addKeyword();
                }
              }}
              flex={1}
            />
            <Select
              aria-label={t('settings.comments.match')}
              data={matches.map((value) => ({
                value,
                label: t(`settings.comments.matches.${value}`),
              }))}
              value={match}
              allowDeselect={false}
              onChange={(value) => value !== null && isMatch(value) && setMatch(value)}
              w={140}
            />
            <Button
              variant="default"
              onClick={addKeyword}
              disabled={keywordText === '' || keywordInvalid}
            >
              {t('settings.comments.add')}
            </Button>
          </Group>
          <Group gap={6}>
            {comments.mutedKeywords.map((k, i) => (
              <Badge
                key={`${k.match}:${k.text}:${i}`}
                variant="light"
                color="gray"
                tt="none"
                rightSection={
                  <ActionIcon
                    size="xs"
                    variant="transparent"
                    color="gray"
                    aria-label={t('settings.comments.remove', { name: k.text })}
                    onClick={() =>
                      save({ mutedKeywords: current().mutedKeywords.filter((_, j) => j !== i) })
                    }
                  >
                    <X size={12} />
                  </ActionIcon>
                }
              >
                {k.text}
                <Text span size="xs" c="dimmed">
                  {' '}
                  ({isMatch(k.match) ? t(`settings.comments.matches.${k.match}`) : k.match})
                </Text>
              </Badge>
            ))}
          </Group>
        </Stack>

        <Stack gap={4}>
          <Text size="sm" fw={500}>
            {t('settings.comments.mutedUsers')}
          </Text>
          <Text size="xs" c="dimmed">
            {t('settings.comments.mutedUsersHelp')}
          </Text>
          <Group gap="xs" wrap="nowrap">
            <TextInput
              aria-label={t('settings.comments.userId')}
              placeholder={t('settings.comments.userId')}
              value={user}
              maxLength={64}
              onChange={(event) => setUser(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
                  addUser();
                }
              }}
              flex={1}
            />
            <Button variant="default" onClick={addUser} disabled={userText === ''}>
              {t('settings.comments.add')}
            </Button>
          </Group>
          <Group gap={6}>
            {comments.mutedUsers.map((id) => (
              <Badge
                key={id}
                variant="light"
                color="gray"
                tt="none"
                ff="monospace"
                rightSection={
                  <ActionIcon
                    size="xs"
                    variant="transparent"
                    color="gray"
                    aria-label={t('settings.comments.remove', { name: id })}
                    onClick={() =>
                      save({ mutedUsers: comments.mutedUsers.filter((u) => u !== id) })
                    }
                  >
                    <X size={12} />
                  </ActionIcon>
                }
              >
                {id}
              </Badge>
            ))}
          </Group>
        </Stack>
      </Stack>
    </Card>
  );
}
