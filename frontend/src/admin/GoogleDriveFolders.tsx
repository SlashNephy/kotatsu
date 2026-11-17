import { Anchor, Breadcrumbs, Button, Card, Group, Loader, Stack, Text } from '@mantine/core';
import { Folder } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';

import type { GoogleDriveFoldersChoicesQuery } from './__generated__/GoogleDriveFoldersChoicesQuery.graphql';

const choicesQuery = graphql`
  query GoogleDriveFoldersChoicesQuery($parentId: String) {
    googleDriveFolderChoices(parentId: $parentId) {
      id
      name
    }
  }
`;

// myDriveId は、マイドライブの根元の別名。名前は、サーバーが英語で返すので、訳して出す。
const myDriveId = 'root';

export type SelectedFolder = { id: string; name: string };

type Choice = { id: string; name: string };

// GoogleDriveFolders は、Google Drive のうち、ライブラリとして見せるフォルダを選ぶ (設計書 5.8、#1902)。選んだフォルダの下だけを
// 家族に見せる。フォルダは、マイドライブと共有ドライブからたどって選ぶ。1 つも選んでいなければ、ライブラリに Google Drive を出さない。
export function GoogleDriveFolders({
  folders,
  busy,
  onAdd,
  onRemove,
}: {
  folders: readonly SelectedFolder[];
  busy: boolean;
  onAdd: (id: string, onDone: () => void) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [choosing, setChoosing] = useState(false);
  const label = (folder: { id: string; name: string }) =>
    folder.id === myDriveId ? t('library.myDrive') : folder.name;

  return (
    <Stack gap="xs">
      <Text size="sm" fw={600}>
        {t('admin.googleDrive.folders.title')}
      </Text>
      <Text size="xs" c="dimmed">
        {t('admin.googleDrive.folders.help')}
      </Text>
      {folders.length === 0 ? (
        <Text size="sm" c="dimmed">
          {t('admin.googleDrive.folders.empty')}
        </Text>
      ) : (
        <Stack gap={4}>
          {folders.map((folder) => (
            <Group key={folder.id} justify="space-between" wrap="nowrap">
              <Group gap={6} wrap="nowrap">
                <Folder size={16} aria-hidden />
                <Text size="sm" style={{ wordBreak: 'break-all' }}>
                  {label(folder)}
                </Text>
              </Group>
              <Button
                size="compact-xs"
                variant="default"
                color="red"
                disabled={busy}
                onClick={() => onRemove(folder.id)}
              >
                {t('admin.googleDrive.folders.remove')}
              </Button>
            </Group>
          ))}
        </Stack>
      )}
      {choosing ? (
        <FolderPicker
          busy={busy}
          label={label}
          onCancel={() => setChoosing(false)}
          onChoose={(id) => onAdd(id, () => setChoosing(false))}
        />
      ) : (
        <Group>
          <Button size="xs" variant="default" disabled={busy} onClick={() => setChoosing(true)}>
            {t('admin.googleDrive.folders.add')}
          </Button>
        </Group>
      )}
    </Stack>
  );
}

// FolderPicker は、Drive のフォルダをたどって、見せるフォルダを 1 つ選ぶ。根元には、マイドライブと共有ドライブを並べる。
function FolderPicker({
  busy,
  label,
  onCancel,
  onChoose,
}: {
  busy: boolean;
  label: (folder: Choice) => string;
  onCancel: () => void;
  onChoose: (id: string) => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // trail は、根元から今のフォルダまでにたどったフォルダ。空なら根元。
  const [trail, setTrail] = useState<Choice[]>([]);
  const [choices, setChoices] = useState<readonly Choice[] | null>(null);
  const [failed, setFailed] = useState(false);
  const current = trail.at(-1);

  useEffect(() => {
    let active = true;
    setChoices(null);
    setFailed(false);
    fetchQuery<GoogleDriveFoldersChoicesQuery>(
      environment,
      choicesQuery,
      { parentId: current?.id ?? null },
      { fetchPolicy: 'network-only' },
    )
      .toPromise()
      .then(
        (data) => {
          if (active) {
            setChoices(data?.googleDriveFolderChoices ?? []);
          }
        },
        () => {
          if (active) {
            setFailed(true);
          }
        },
      );

    return () => {
      active = false;
    };
  }, [environment, current?.id]);

  return (
    <Card withBorder padding="sm">
      <Stack gap="xs">
        <Breadcrumbs separatorMargin={4}>
          <Anchor component="button" type="button" size="sm" onClick={() => setTrail([])}>
            {t('admin.googleDrive.folders.top')}
          </Anchor>
          {trail.map((folder, i) => (
            <Anchor
              key={folder.id}
              component="button"
              type="button"
              size="sm"
              onClick={() => setTrail(trail.slice(0, i + 1))}
            >
              {label(folder)}
            </Anchor>
          ))}
        </Breadcrumbs>
        {failed ? (
          <Text size="sm" c="red">
            {t('admin.googleDrive.folders.error')}
          </Text>
        ) : choices === null ? (
          <Loader size="sm" />
        ) : choices.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.googleDrive.folders.noSubfolders')}
          </Text>
        ) : (
          <Stack gap={2}>
            {choices.map((choice) => (
              <Anchor
                key={choice.id}
                component="button"
                type="button"
                size="sm"
                ta="left"
                onClick={() => setTrail([...trail, choice])}
              >
                <Group gap={6} wrap="nowrap">
                  <Folder size={16} aria-hidden />
                  <span style={{ wordBreak: 'break-all' }}>{label(choice)}</span>
                </Group>
              </Anchor>
            ))}
          </Stack>
        )}
        <Group justify="flex-end" gap="xs">
          <Button size="xs" variant="default" onClick={onCancel}>
            {t('admin.googleDrive.folders.cancel')}
          </Button>
          <Button
            size="xs"
            disabled={busy || current === undefined}
            onClick={() => current !== undefined && onChoose(current.id)}
          >
            {current === undefined
              ? t('admin.googleDrive.folders.chooseHint')
              : t('admin.googleDrive.folders.choose', { name: label(current) })}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
