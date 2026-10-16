import { Alert, Button, CopyButton, Group, Stack, Text, TextInput } from '@mantine/core';
import { Check, Copy, ExternalLink } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { ExternalStreamURLMutation } from './__generated__/ExternalStreamURLMutation.graphql';

const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

type Props = {
  // target は、URL を発行するサービス (ライブ視聴) か録画の ID。
  target: string;
  // live は、ライブ視聴の URL かどうか。ライブは、開いている間チューナーを使うことを添える。
  live?: boolean;
};

// ExternalStreamURL は、VLC や mpv で開く、署名付きの映像の URL を発行し、コピーできるようにする (設計書 7.6)。
export function ExternalStreamURL({ target, live = false }: Props) {
  const { t } = useTranslation();
  const [create, creating] = useMutation<ExternalStreamURLMutation>(graphql`
    mutation ExternalStreamURLMutation($target: ID!) {
      createStreamURL(target: $target) {
        url
        expiresAt
      }
    }
  `);
  const [issued, setIssued] = useState<{ url: string; expiresAt: string } | null>(null);
  const [error, setError] = useState(false);
  const issue = () => {
    setError(false);
    create({
      variables: { target },
      onCompleted: (response, errors) => {
        if (errors != null && errors.length > 0) {
          setError(true);

          return;
        }
        setIssued({
          url: new URL(response.createStreamURL.url, window.location.origin).href,
          expiresAt: response.createStreamURL.expiresAt,
        });
      },
      onError: () => setError(true),
    });
  };

  return (
    <Stack gap={4}>
      <Group>
        <Button
          variant="default"
          leftSection={<ExternalLink size={16} aria-hidden />}
          onClick={issue}
          loading={creating}
        >
          {t('streamURL.issue')}
        </Button>
      </Group>
      {error && (
        <Alert color="red" role="alert">
          {t('streamURL.error')}
        </Alert>
      )}
      {issued !== null && (
        <>
          <Text size="xs" c="dimmed">
            {t(live ? 'streamURL.descriptionLive' : 'streamURL.description', {
              until: timeFormat.format(Date.parse(issued.expiresAt)),
            })}
          </Text>
          <Group gap="xs" wrap="nowrap">
            <TextInput
              value={issued.url}
              readOnly
              style={{ flex: 1 }}
              size="xs"
              aria-label={t('streamURL.label')}
            />
            <CopyButton value={issued.url}>
              {({ copied, copy }) => (
                <Button
                  size="xs"
                  variant="default"
                  leftSection={
                    copied ? <Check size={14} aria-hidden /> : <Copy size={14} aria-hidden />
                  }
                  onClick={copy}
                >
                  {copied ? t('streamURL.copied') : t('streamURL.copy')}
                </Button>
              )}
            </CopyButton>
          </Group>
        </>
      )}
    </Stack>
  );
}
