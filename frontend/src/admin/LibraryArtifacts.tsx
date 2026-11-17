import { Alert, Button, Card, Group, Modal, Stack, Text, Title } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';

import { formatBytes } from '../futon';
import { graphQLErrorDetails } from '../relay/environment';
import { mutate } from '../settings/mutate';
import type { LibraryArtifactsClearMutation } from './__generated__/LibraryArtifactsClearMutation.graphql';
import type { LibraryArtifactsQuery } from './__generated__/LibraryArtifactsQuery.graphql';

const query = graphql`
  query LibraryArtifactsQuery {
    libraryArtifacts {
      size
      files
    }
  }
`;

const clearMutation = graphql`
  mutation LibraryArtifactsClearMutation {
    clearLibraryArtifacts {
      size
      files
    }
  }
`;

type Usage = { size: number; files: number };

// LibraryArtifacts は、ライブラリのファイルの生成物 (索引、画像、CM の区間、自動生成した字幕) を、データディレクトリから丸ごと片付ける
// (設計書 5.8、#1896)。生成物を選んで消すのではない。消す前に、消える量を数えて見せ、確かめてから消す。
// ファイルが多いと数えるのに時間がかかるので、画面を開いただけでは数えず、ボタンを押したときに数える。
export function LibraryArtifacts() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // measured は、確かめるために数えた量。null なら、確かめる画面を出さない。
  const [measured, setMeasured] = useState<Usage | null>(null);
  const [measuring, setMeasuring] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);

  const measure = async () => {
    setMeasuring(true);
    setMessage(null);
    try {
      const data = await fetchQuery<LibraryArtifactsQuery>(
        environment,
        query,
        {},
        { fetchPolicy: 'network-only' },
      ).toPromise();
      setMeasured(data?.libraryArtifacts ?? null);
    } catch (error) {
      setMessage({
        ok: false,
        text:
          graphQLErrorDetails(error).code === 'CONFLICT'
            ? t('admin.libraryArtifacts.noDataDir')
            : t('admin.libraryArtifacts.measureError'),
      });
    } finally {
      setMeasuring(false);
    }
  };

  const clear = async () => {
    setMeasured(null);
    setClearing(true);
    try {
      const { clearLibraryArtifacts: cleared } = await mutate<LibraryArtifactsClearMutation>(
        environment,
        clearMutation,
        {},
      );
      setMessage({
        ok: true,
        text:
          cleared.files === 0
            ? t('admin.libraryArtifacts.nothing')
            : t('admin.libraryArtifacts.cleared', {
                count: cleared.files,
                size: formatBytes(cleared.size),
              }),
      });
    } catch (error) {
      setMessage({
        ok: false,
        text:
          graphQLErrorDetails(error).code === 'CONFLICT'
            ? t('admin.libraryArtifacts.busy')
            : t('admin.libraryArtifacts.clearError'),
      });
    } finally {
      setClearing(false);
    }
  };

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('admin.libraryArtifacts.title')}
          </Title>
          <Button
            size="xs"
            variant="default"
            loading={measuring || clearing}
            onClick={() => void measure()}
          >
            {t('admin.libraryArtifacts.clear')}
          </Button>
        </Group>
        <Text size="xs" c="dimmed">
          {t('admin.libraryArtifacts.description')}
        </Text>
        {message !== null && (
          <Alert color={message.ok ? 'teal' : 'red'} role={message.ok ? 'status' : 'alert'}>
            {message.text}
          </Alert>
        )}
      </Stack>
      <Modal
        opened={measured !== null}
        onClose={() => setMeasured(null)}
        title={t('admin.libraryArtifacts.confirmTitle')}
      >
        {measured !== null && (
          <Stack>
            {measured.files === 0 ? (
              <Text size="sm">{t('admin.libraryArtifacts.nothing')}</Text>
            ) : (
              <>
                <Text size="sm">
                  {t('admin.libraryArtifacts.confirm', {
                    count: measured.files,
                    size: formatBytes(measured.size),
                  })}
                </Text>
                <Text size="sm" c="dimmed">
                  {t('admin.libraryArtifacts.confirmPlaying')}
                </Text>
              </>
            )}
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setMeasured(null)}>
                {t('admin.libraryArtifacts.cancel')}
              </Button>
              <Button color="red" disabled={measured.files === 0} onClick={() => void clear()}>
                {t('admin.libraryArtifacts.confirmButton')}
              </Button>
            </Group>
          </Stack>
        )}
      </Modal>
    </Card>
  );
}
