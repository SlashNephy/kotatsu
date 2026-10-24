import { Group, List, Progress, Stack, Text } from '@mantine/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { Strength } from '../password/zxcvbn';

type Props = {
  password: string;
  // userInputs は、ユーザー名のように、パスワードに使われると推測しやすい語。
  userInputs: string[];
};

// minScore は、サーバーの password.MinScore と同じ閾値。これ未満は、サーバーが拒否する。
const minScore = 2;

const colors = ['red', 'red', 'amber', 'mikan', 'green'] as const;
const labels = [
  'password.strength.score0',
  'password.strength.score1',
  'password.strength.score2',
  'password.strength.score3',
  'password.strength.score4',
] as const;

// PasswordStrength は、入力中のパスワードの強度を、zxcvbn-ts (日本語の辞書入り) で判定して出す (設計書 2.2)。
// 判定は入力中のフィードバックで、拒否するかどうかはサーバーが決める。辞書は大きいので、使うときに読み込む。
export function PasswordStrength({ password, userInputs }: Props) {
  const { t } = useTranslation();
  const [strength, setStrength] = useState<Strength | null>(null);
  const inputs = userInputs.join('\n');

  useEffect(() => {
    if (password === '') {
      setStrength(null);

      return;
    }
    let canceled = false;
    void import('../password/zxcvbn').then(({ checkStrength }) => {
      if (!canceled) {
        setStrength(
          checkStrength(
            password,
            inputs.split('\n').filter((input) => input !== ''),
          ),
        );
      }
    });

    return () => {
      canceled = true;
    };
  }, [password, inputs]);

  if (strength === null) {
    return null;
  }
  const score = Math.max(0, Math.min(4, strength.score));
  const label = labels[score] ?? labels[0];

  return (
    <Stack gap={4} aria-live="polite">
      <Group justify="space-between">
        <Text size="sm">{t('password.strength.label')}</Text>
        <Text size="sm" fw={700} c={colors[score]}>
          {t(label)}
        </Text>
      </Group>
      <Progress value={((score + 1) / 5) * 100} color={colors[score]} aria-hidden />
      {score < minScore && (
        <Text size="sm" c="red">
          {t('password.strength.tooWeak')}
        </Text>
      )}
      {strength.warning !== null && <Text size="sm">{strength.warning}</Text>}
      {strength.suggestions.length > 0 && (
        <List size="sm">
          {strength.suggestions.map((suggestion) => (
            <List.Item key={suggestion}>{suggestion}</List.Item>
          ))}
        </List>
      )}
    </Stack>
  );
}
