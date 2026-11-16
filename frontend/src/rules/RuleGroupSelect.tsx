import { Button, Group, Select, Stack, TextInput } from '@mantine/core';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { graphQLErrorDetails } from '../relay/environment';
import type { RuleGroupSelectCreateMutation } from './__generated__/RuleGroupSelectCreateMutation.graphql';

// noGroup と newGroup は、選択肢のうち、グループに入れないこと (「その他」) と、新しいグループを作ることを表す値。
// グループの ID (RuleGroup:...) とは重ならない。
const noGroup = 'none';
const newGroup = 'new';

type Props = {
  // groups は、選べるグループ。一覧で並べる順に渡す。
  groups: readonly { readonly id: string; readonly name: string }[];
  // value は、選んだグループの ID。null ならどのグループにも入れない。
  value: string | null;
  onChange: (groupId: string | null) => void;
  label?: string;
  disabled?: boolean;
};

// RuleGroupSelect は、ルールを入れるグループを選ぶ入力 (#1493)。「新しいグループ」を選ぶと、その場で名前を入れて作り、選ぶ。
// グループは作ったときに保存する。ルールを保存しなくても、グループは残る。
export function RuleGroupSelect({ groups, value, onChange, label, disabled = false }: Props) {
  const { t } = useTranslation();
  // naming は、新しいグループの名前を入れているかどうか。
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [create, creating] = useMutation<RuleGroupSelectCreateMutation>(graphql`
    mutation RuleGroupSelectCreateMutation($name: String!) {
      createRuleGroup(name: $name) {
        id
        name
        position
      }
    }
  `);
  const submit = () => {
    if (name.trim() === '' || creating) {
      return;
    }
    setError(null);
    const failed = (cause: unknown) =>
      setError(
        graphQLErrorDetails(cause).code === 'BAD_USER_INPUT'
          ? t('rules.groups.nameInvalid')
          : t('rules.groups.error'),
      );
    create({
      variables: { name },
      // 作ったグループを、選択肢 (ルートの ruleGroups) の最後に足す。
      updater: (store) => {
        const created = store.getRootField('createRuleGroup');
        const root = store.getRoot();
        const current = root.getLinkedRecords('ruleGroups');
        if (current != null) {
          root.setLinkedRecords([...current, created], 'ruleGroups');
        }
      },
      onCompleted: (response, errors) => {
        if (errors != null && errors.length > 0) {
          failed({ source: { errors } });

          return;
        }
        onChange(response.createRuleGroup.id);
        setNaming(false);
        setName('');
      },
      onError: failed,
    });
  };

  return (
    <Stack gap={4}>
      <Select
        label={label ?? t('rules.groups.label')}
        data={[
          { value: noGroup, label: t('rules.groups.none') },
          ...groups.map((group) => ({ value: group.id, label: group.name })),
          { value: newGroup, label: t('rules.groups.new') },
        ]}
        value={naming ? newGroup : (value ?? noGroup)}
        allowDeselect={false}
        disabled={disabled}
        onChange={(selected) => {
          if (selected === newGroup) {
            setNaming(true);

            return;
          }
          setNaming(false);
          setError(null);
          onChange(selected === null || selected === noGroup ? null : selected);
        }}
      />
      {naming && (
        <Group gap="xs" align="flex-start" wrap="nowrap">
          <TextInput
            aria-label={t('rules.groups.newName')}
            placeholder={t('rules.groups.newName')}
            value={name}
            maxLength={100}
            error={error}
            disabled={disabled}
            onChange={(event) => setName(event.currentTarget.value)}
            // ルールの編集のフォームの中に置くので、Enter でルールを保存せず、グループを作る。
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
                event.preventDefault();
                submit();
              }
            }}
            style={{ flex: 1 }}
          />
          <Button
            variant="default"
            leftSection={<Plus size={16} aria-hidden />}
            loading={creating}
            disabled={disabled || name.trim() === ''}
            onClick={submit}
          >
            {t('rules.groups.create')}
          </Button>
        </Group>
      )}
    </Stack>
  );
}
