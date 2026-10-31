import { Button, Code, Group, Image, Stack, Text, TextInput } from '@mantine/core';
import { useComputedColorScheme } from '@mantine/core';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { useAction } from '../admin/useAction';
import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import {
  drcsImage,
  drcsReplacements,
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
      {glyphs.map((glyph) => (
        <UnknownDRCSRow key={glyph.md5} glyph={glyph} />
      ))}
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

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    action.run(
      async () => {
        // 表は丸ごと送るので、今の表に足して送る。
        const rows = [...drcsReplacements].map(([md5, value]) => ({ md5, text: value }));
        rows.push({ md5: glyph.md5, text });
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
          { input: { drcsReplacements: rows } },
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
          src={drcsImage(glyph, scheme === 'dark' ? '#fff' : '#000')}
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
