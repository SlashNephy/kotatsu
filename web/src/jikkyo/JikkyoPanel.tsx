import { Button, Group, Text, TextInput } from '@mantine/core';
import { Send } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { SessionStatus } from './session';

type Props = {
  channelName: string;
  status: SessionStatus | null;
  onPost: (text: string) => Promise<void>;
};

// maxLength は、投稿できるコメントの長さの上限。ニコニコ実況と同じにする。
const maxLength = 75;

// JikkyoPanel は、実況のつながり具合と、コメントを投稿する欄 (設計書 7.4)。
export function JikkyoPanel({ channelName, status, onPost }: Props) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [posting, setPosting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const connected = status?.kind === 'connected';

  const submit = async () => {
    const value = text.trim();
    if (value === '' || posting) {
      return;
    }
    setPosting(true);
    setError(null);
    try {
      await onPost(value);
      setText('');
    } catch {
      setError(t('jikkyo.postError'));
    } finally {
      setPosting(false);
    }
  };

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <Group gap="xs" wrap="nowrap" align="flex-start">
        <TextInput
          style={{ flex: 1 }}
          size="sm"
          aria-label={t('jikkyo.comment')}
          placeholder={t('jikkyo.placeholder', { channel: channelName })}
          maxLength={maxLength}
          value={text}
          disabled={!connected}
          error={error}
          onChange={(event) => {
            setText(event.currentTarget.value);
            setError(null);
          }}
        />
        <Button
          type="submit"
          size="sm"
          loading={posting}
          disabled={!connected || text.trim() === ''}
          leftSection={<Send size={16} aria-hidden />}
        >
          {t('jikkyo.post')}
        </Button>
      </Group>
      {/* つながっている間は、コメントが流れるのを見れば分かるので、何も出さない。 */}
      {!connected && (
        <Text size="xs" c="dimmed" mt={4} role="status">
          {status === null || status.kind === 'connecting'
            ? t('jikkyo.connecting')
            : t('jikkyo.disconnected', {
                seconds: status.kind === 'disconnected' ? status.retryIn : 0,
              })}
        </Text>
      )}
    </form>
  );
}
