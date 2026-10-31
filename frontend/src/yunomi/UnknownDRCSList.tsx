import { Box, Button, Code, Group, Image, Stack, Text, TextInput } from '@mantine/core';
import { useComputedColorScheme } from '@mantine/core';
import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { useAction } from '../admin/useAction';
import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import {
  drcsImage,
  setDRCSReplacements,
  useUnknownDRCS,
  validDRCSText,
  type DRCSGlyph,
} from './drcs';
import type { UnknownDRCSListMutation } from './__generated__/UnknownDRCSListMutation.graphql';

// UnknownDRCSList は、字幕に出たが、置き換えの表に無い外字の一覧 (#825)。管理者だけに出す。
// 外字の画像を見て、置き換える文字を書けば、その場で表に登録する。
export function UnknownDRCSList() {
  const { t } = useTranslation();
  const glyphs = useUnknownDRCS();
  if (glyphs.length === 0) {
    return null;
  }

  return (
    <Stack gap={4} p="xs">
      <Text size="xs" c="dimmed">
        {t('drcs.unknown', { count: glyphs.length })}
      </Text>
      {/* 登録しない外字 (ロゴなど) が溜まっても、字幕の履歴を押し下げすぎないよう、高さを抑える。 */}
      <Box mah={200} style={{ overflowY: 'auto' }}>
        <Stack gap={4}>
          {glyphs.map((glyph) => (
            <UnknownDRCSRow key={glyph.md5} glyph={glyph} />
          ))}
        </Stack>
      </Box>
    </Stack>
  );
}

function UnknownDRCSRow({ glyph }: { glyph: DRCSGlyph }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const scheme = useComputedColorScheme('light');
  const action = useAction();
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const valid = validDRCSText(text);
  // 字幕が出るたびに描き直さないよう、画像は外字と色が変わったときだけ作る。
  const image = useMemo(
    () => drcsImage(glyph, scheme === 'dark' ? '#fff' : '#000'),
    [glyph, scheme],
  );

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    action.run(
      async () => {
        // この外字だけを送る。サーバーが今の表に足すので、続けて登録しても、前の登録を消さない。
        const response = await mutate<UnknownDRCSListMutation>(
          environment,
          graphql`
            mutation UnknownDRCSListMutation($input: SystemSettingsInput!) {
              updateSystemSettings(input: $input) {
                drcsReplacements {
                  md5
                  text
                }
              }
            }
          `,
          { input: { drcsReplacements: [{ md5: glyph.md5, text }] } },
        );
        setDRCSReplacements(response.updateSystemSettings.drcsReplacements);
      },
      (caught) => setError(settingsErrorMessage(t, caught)),
    );
  };

  return (
    <form onSubmit={submit}>
      <Group gap="xs" wrap="nowrap" align="center">
        <Image
          src={image}
          alt={t('drcs.glyph')}
          w={glyph.width * 2}
          h={glyph.height * 2}
          fit="contain"
          style={{ imageRendering: 'pixelated', flexShrink: 0 }}
        />
        <Code fz="xs" style={{ flexShrink: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>
          {glyph.md5}
        </Code>
        <TextInput
          size="xs"
          w={80}
          aria-label={t('drcs.text')}
          placeholder={t('drcs.text')}
          value={text}
          onChange={(event) => setText(event.currentTarget.value)}
          error={error}
        />
        <Button size="xs" type="submit" disabled={!valid || action.busy}>
          {t('drcs.register')}
        </Button>
      </Group>
    </form>
  );
}
