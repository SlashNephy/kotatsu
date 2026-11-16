import {
  ActionIcon,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  Stack,
  Text,
  TextInput,
  Title,
  UnstyledButton,
} from '@mantine/core';
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  ChevronRight,
  EllipsisVertical,
  FolderPlus,
  Pencil,
  Trash2,
} from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { graphQLErrorDetails } from '../relay/environment';
import type { RuleGroupHeaderCreateMutation } from './__generated__/RuleGroupHeaderCreateMutation.graphql';
import type { RuleGroupHeaderDeleteMutation } from './__generated__/RuleGroupHeaderDeleteMutation.graphql';
import type { RuleGroupHeaderRenameMutation } from './__generated__/RuleGroupHeaderRenameMutation.graphql';
import type { RuleGroupHeaderReorderMutation } from './__generated__/RuleGroupHeaderReorderMutation.graphql';

type RuleGroup = { readonly id: string; readonly name: string };

type HeaderProps = {
  // group は、見出しのグループ。null なら「その他」(どのグループにも入らないルール) の見出し。
  group: RuleGroup | null;
  // count は、見出しの下に出すルールの数 (絞り込んだ後の数)。
  count: number;
  // collapsible は、折りたためるかどうか。探す語や絞り込みがあるときは、当たったルールを隠さないよう、開いたままにする。
  collapsible: boolean;
  collapsed: boolean;
  onToggle: () => void;
  // groups は、すべてのグループを並べる順に並べたもの。上と下へ移すときに使う。
  groups: readonly RuleGroup[];
  canEdit: boolean;
  // onError は、グループの操作に失敗したことを知らせる。
  onError: () => void;
};

// RuleGroupHeader は、ルールの一覧の、グループの見出し (#1493)。押すと折りたたむ。
// 変えられるユーザーには、グループの名前の変更、並べ替え、削除のメニューを出す。
export function RuleGroupHeader({
  group,
  count,
  collapsible,
  collapsed,
  onToggle,
  groups,
  canEdit,
  onError,
}: HeaderProps) {
  const { t } = useTranslation();
  const name = group?.name ?? t('rules.groups.others');
  const [renaming, setRenaming] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [rename, renamingGroup] = useMutation<RuleGroupHeaderRenameMutation>(graphql`
    mutation RuleGroupHeaderRenameMutation($id: ID!, $name: String!) {
      renameRuleGroup(id: $id, name: $name) {
        id
        name
      }
    }
  `);
  const [reorder] = useMutation<RuleGroupHeaderReorderMutation>(graphql`
    mutation RuleGroupHeaderReorderMutation($ids: [ID!]!) {
      reorderRuleGroups(ids: $ids) {
        id
        name
        position
      }
    }
  `);
  const [remove, removing] = useMutation<RuleGroupHeaderDeleteMutation>(graphql`
    mutation RuleGroupHeaderDeleteMutation($id: ID!) {
      deleteRuleGroup(id: $id)
    }
  `);
  const index = group === null ? -1 : groups.findIndex((item) => item.id === group.id);
  // move は、グループを offset だけ上 (負) か下 (正) へ移す。
  const move = (offset: number) => {
    const ids = groups.map((item) => item.id);
    const target = index + offset;
    if (index < 0 || target < 0 || target >= ids.length) {
      return;
    }
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder({
      variables: { ids },
      // 並べ直したグループを、一覧の並び (ルートの ruleGroups) に写す。
      updater: (store) => {
        const reordered = store.getPluralRootField('reorderRuleGroups');
        if (reordered != null) {
          store.getRoot().setLinkedRecords(reordered, 'ruleGroups');
        }
      },
      onCompleted: (_response, errors) => {
        if (errors != null && errors.length > 0) {
          onError();
        }
      },
      onError,
    });
  };
  const deleteGroup = () => {
    if (group === null) {
      return;
    }
    remove({
      variables: { id: group.id },
      // 消したグループを一覧から外し、中のルールを「その他」に戻す。
      updater: (store) => {
        const root = store.getRoot();
        root.setLinkedRecords(
          (root.getLinkedRecords('ruleGroups') ?? []).filter(
            (record) => record.getDataID() !== group.id,
          ),
          'ruleGroups',
        );
        for (const rule of root.getLinkedRecords('rules') ?? []) {
          if (rule.getLinkedRecord('group')?.getDataID() === group.id) {
            rule.setValue(null, 'group');
          }
        }
      },
      onCompleted: (_response, errors) => {
        setConfirmingDelete(false);
        if (errors != null && errors.length > 0) {
          onError();
        }
      },
      onError: () => {
        setConfirmingDelete(false);
        onError();
      },
    });
  };
  const Chevron = collapsed ? ChevronRight : ChevronDown;

  return (
    <Group justify="space-between" wrap="nowrap" gap="xs" mt="xs">
      <Title order={3} size="h5" style={{ minWidth: 0, flex: 1 }}>
        {collapsible ? (
          <UnstyledButton
            onClick={onToggle}
            aria-expanded={!collapsed}
            style={{ display: 'flex', alignItems: 'center', gap: 6, maxWidth: '100%' }}
          >
            <Chevron size={16} aria-hidden style={{ flexShrink: 0 }} />
            <Text span fw={700} inherit truncate>
              {name}
            </Text>
            <Badge variant="light" color="gray" size="sm" style={{ flexShrink: 0 }}>
              {count}
            </Badge>
          </UnstyledButton>
        ) : (
          <Group gap={6} wrap="nowrap">
            <Text span fw={700} inherit truncate>
              {name}
            </Text>
            <Badge variant="light" color="gray" size="sm" style={{ flexShrink: 0 }}>
              {count}
            </Badge>
          </Group>
        )}
      </Title>
      {canEdit && group !== null && (
        <>
          <Menu position="bottom-end" withinPortal>
            <Menu.Target>
              <ActionIcon
                variant="subtle"
                color="gray"
                aria-label={t('rules.groups.menu', { name })}
              >
                <EllipsisVertical size={16} aria-hidden />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item
                leftSection={<Pencil size={14} aria-hidden />}
                onClick={() => setRenaming(true)}
              >
                {t('rules.groups.rename')}
              </Menu.Item>
              <Menu.Item
                leftSection={<ArrowUp size={14} aria-hidden />}
                disabled={index <= 0}
                onClick={() => move(-1)}
              >
                {t('rules.groups.moveUp')}
              </Menu.Item>
              <Menu.Item
                leftSection={<ArrowDown size={14} aria-hidden />}
                disabled={index < 0 || index >= groups.length - 1}
                onClick={() => move(1)}
              >
                {t('rules.groups.moveDown')}
              </Menu.Item>
              <Menu.Item
                color="red"
                leftSection={<Trash2 size={14} aria-hidden />}
                onClick={() => setConfirmingDelete(true)}
              >
                {t('rules.groups.delete')}
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
          <RuleGroupNameDialog
            opened={renaming}
            title={t('rules.groups.renameTitle')}
            submitLabel={t('rules.groups.rename')}
            initialName={group.name}
            saving={renamingGroup}
            onClose={() => setRenaming(false)}
            onSubmit={(newName, failed) =>
              rename({
                variables: { id: group.id, name: newName },
                onCompleted: (_response, errors) =>
                  errors != null && errors.length > 0 ? failed(errors) : setRenaming(false),
                onError: failed,
              })
            }
          />
          <Modal
            opened={confirmingDelete}
            onClose={() => setConfirmingDelete(false)}
            title={t('rules.groups.deleteTitle')}
          >
            <Stack>
              <Text size="sm">{t('rules.groups.deleteConfirm', { name: group.name })}</Text>
              <Group justify="flex-end">
                <Button variant="default" onClick={() => setConfirmingDelete(false)}>
                  {t('rules.cancel')}
                </Button>
                <Button color="red" loading={removing} onClick={deleteGroup}>
                  {t('rules.groups.delete')}
                </Button>
              </Group>
            </Stack>
          </Modal>
        </>
      )}
    </Group>
  );
}

type NameDialogProps = {
  opened: boolean;
  title: string;
  submitLabel: string;
  initialName: string;
  saving: boolean;
  onClose: () => void;
  // onSubmit は、名前 name でグループを作るか名前を変える。失敗したら failed を呼ぶ。
  onSubmit: (name: string, failed: (cause: unknown) => void) => void;
};

// RuleGroupNameDialog は、グループの名前を入れるダイアログ。グループを作るときと、名前を変えるときに使う。
function RuleGroupNameDialog({
  opened,
  title,
  submitLabel,
  initialName,
  saving,
  onClose,
  onSubmit,
}: NameDialogProps) {
  const { t } = useTranslation();
  const [name, setName] = useState(initialName);
  const [error, setError] = useState<string | null>(null);
  // 開き直したら、今の名前から入れ直す。
  const [wasOpened, setWasOpened] = useState(opened);
  if (opened !== wasOpened) {
    setWasOpened(opened);
    if (opened) {
      setName(initialName);
      setError(null);
    }
  }
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (name.trim() === '' || saving) {
      return;
    }
    setError(null);
    onSubmit(name, (cause) =>
      setError(
        graphQLErrorDetails(Array.isArray(cause) ? { source: { errors: cause } } : cause).code ===
          'BAD_USER_INPUT'
          ? t('rules.groups.nameInvalid')
          : t('rules.groups.error'),
      ),
    );
  };

  return (
    <Modal opened={opened} onClose={onClose} title={title}>
      <form onSubmit={submit}>
        <Stack>
          <TextInput
            label={t('rules.groups.name')}
            value={name}
            maxLength={100}
            required
            data-autofocus
            error={error}
            onChange={(event) => setName(event.currentTarget.value)}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              {t('rules.cancel')}
            </Button>
            <Button type="submit" loading={saving} disabled={name.trim() === ''}>
              {submitLabel}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}

// CreateRuleGroupButton は、ルールの一覧で、空のグループを作るボタン (#1493)。作ったグループは、一覧の最後に並ぶ。
export function CreateRuleGroupButton() {
  const { t } = useTranslation();
  const [opened, setOpened] = useState(false);
  const [create, creating] = useMutation<RuleGroupHeaderCreateMutation>(graphql`
    mutation RuleGroupHeaderCreateMutation($name: String!) {
      createRuleGroup(name: $name) {
        id
        name
        position
      }
    }
  `);

  return (
    <>
      <Button
        variant="default"
        leftSection={<FolderPlus size={16} aria-hidden />}
        onClick={() => setOpened(true)}
      >
        {t('rules.groups.createButton')}
      </Button>
      <RuleGroupNameDialog
        opened={opened}
        title={t('rules.groups.createTitle')}
        submitLabel={t('rules.groups.create')}
        initialName=""
        saving={creating}
        onClose={() => setOpened(false)}
        onSubmit={(name, failed) =>
          create({
            variables: { name },
            // 作ったグループを、一覧の並び (ルートの ruleGroups) の最後に足す。
            updater: (store) => {
              const created = store.getRootField('createRuleGroup');
              if (created == null) {
                return;
              }
              const root = store.getRoot();
              root.setLinkedRecords(
                [...(root.getLinkedRecords('ruleGroups') ?? []), created],
                'ruleGroups',
              );
            },
            onCompleted: (_response, errors) =>
              errors != null && errors.length > 0 ? failed(errors) : setOpened(false),
            onError: failed,
          })
        }
      />
    </>
  );
}
