import {
  Anchor,
  Breadcrumbs,
  Button,
  Card,
  Group,
  Loader,
  Stack,
  Text,
  TextInput,
} from '@mantine/core';
import { Folder } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';

import { suggestLibraryName } from '../library/library';
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

// SelectedFolder は、ライブラリとして見せると選んだフォルダ。libraryName は、そのライブラリの名前。
export type SelectedFolder = { id: string; name: string; libraryName: string };

type Choice = { id: string; name: string };

// GoogleDriveFolders は、Google Drive のうち、ライブラリとして見せるフォルダを選ぶ (設計書 5.8、#1902)。選んだフォルダごとに
// ライブラリが 1 つでき、その下だけを家族に見せる。ライブラリの名前は、選ぶときに決め、後で変えられる。
export function GoogleDriveFolders({
  folders,
  busy,
  onAdd,
  onRename,
  onRemove,
}: {
  folders: readonly SelectedFolder[];
  busy: boolean;
  onAdd: (id: string, libraryName: string, onDone: () => void) => void;
  onRename: (id: string, libraryName: string, onDone: () => void) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [choosing, setChoosing] = useState(false);
  const label = (folder: Choice) => (folder.id === myDriveId ? t('library.myDrive') : folder.name);

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
        <Stack gap={6}>
          {folders.map((folder) => (
            <FolderRow
              key={`${folder.id}/${folder.libraryName}`}
              folder={folder}
              busy={busy}
              onRename={(name, onDone) => onRename(folder.id, name, onDone)}
              onRemove={() => onRemove(folder.id)}
            />
          ))}
        </Stack>
      )}
      {choosing ? (
        <FolderPicker
          busy={busy}
          label={label}
          onCancel={() => setChoosing(false)}
          onChoose={(id, name) => onAdd(id, name, () => setChoosing(false))}
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

// FolderRow は、選んだフォルダ 1 つ。ライブラリの名前を変えられる。
function FolderRow({
  folder,
  busy,
  onRename,
  onRemove,
}: {
  folder: SelectedFolder;
  busy: boolean;
  onRename: (name: string, onDone: () => void) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(folder.libraryName);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onRename(name.trim(), () => setEditing(false));
  };

  return (
    <Group justify="space-between" wrap="nowrap" align="flex-start">
      <Group gap={6} wrap="nowrap" align="flex-start">
        <Folder size={16} aria-hidden style={{ marginTop: 4 }} />
        {editing ? (
          <form onSubmit={submit}>
            <Group gap="xs" wrap="nowrap">
              <TextInput
                size="xs"
                aria-label={t('admin.googleDrive.folders.libraryName')}
                value={name}
                maxLength={63}
                onChange={(event) => setName(event.currentTarget.value)}
              />
              <Button type="submit" size="compact-xs" disabled={busy || name.trim() === ''}>
                {t('admin.googleDrive.folders.save')}
              </Button>
              <Button
                size="compact-xs"
                variant="subtle"
                onClick={() => {
                  setName(folder.libraryName);
                  setEditing(false);
                }}
              >
                {t('admin.googleDrive.folders.cancel')}
              </Button>
            </Group>
          </form>
        ) : (
          <Stack gap={0}>
            <Text size="sm" fw={500}>
              {folder.libraryName}
            </Text>
            <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>
              {t('admin.googleDrive.folders.from', { name: folder.name })}
            </Text>
          </Stack>
        )}
      </Group>
      {!editing && (
        <Group gap="xs" wrap="nowrap">
          <Button
            size="compact-xs"
            variant="default"
            disabled={busy}
            onClick={() => setEditing(true)}
          >
            {t('admin.googleDrive.folders.rename')}
          </Button>
          <Button
            size="compact-xs"
            variant="default"
            color="red"
            disabled={busy}
            onClick={onRemove}
          >
            {t('admin.googleDrive.folders.remove')}
          </Button>
        </Group>
      )}
    </Group>
  );
}

// FolderPicker は、Drive のフォルダをたどって、見せるフォルダを 1 つ選び、ライブラリの名前を決める。根元には、マイドライブと共有ドライブを並べる。
function FolderPicker({
  busy,
  label,
  onCancel,
  onChoose,
}: {
  busy: boolean;
  label: (folder: Choice) => string;
  onCancel: () => void;
  onChoose: (id: string, libraryName: string) => void;
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
        {current === undefined ? (
          <Group justify="flex-end" gap="xs">
            <Text size="xs" c="dimmed">
              {t('admin.googleDrive.folders.chooseHint')}
            </Text>
            <Button size="xs" variant="default" onClick={onCancel}>
              {t('admin.googleDrive.folders.cancel')}
            </Button>
          </Group>
        ) : (
          <ChooseForm
            key={current.id}
            folder={current}
            label={label(current)}
            busy={busy}
            onCancel={onCancel}
            onChoose={(name) => onChoose(current.id, name)}
          />
        )}
      </Stack>
    </Card>
  );
}

// ChooseForm は、開いているフォルダを、ライブラリの名前を決めて選ぶ。名前の案は、フォルダの名前から作る。
function ChooseForm({
  folder,
  label,
  busy,
  onCancel,
  onChoose,
}: {
  folder: Choice;
  label: string;
  busy: boolean;
  onCancel: () => void;
  onChoose: (libraryName: string) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(suggestLibraryName(folder.name));

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onChoose(name.trim());
      }}
    >
      <Group align="flex-end" justify="flex-end" gap="xs" wrap="wrap">
        <TextInput
          size="xs"
          label={t('admin.googleDrive.folders.libraryName')}
          description={t('admin.googleDrive.folders.libraryNameHelp')}
          value={name}
          maxLength={63}
          onChange={(event) => setName(event.currentTarget.value)}
          style={{ flex: 1, minWidth: 200 }}
        />
        <Button size="xs" variant="default" onClick={onCancel}>
          {t('admin.googleDrive.folders.cancel')}
        </Button>
        <Button type="submit" size="xs" disabled={busy || name.trim() === ''}>
          {t('admin.googleDrive.folders.choose', { name: label })}
        </Button>
      </Group>
    </form>
  );
}
