import { Alert, Button, Group, Modal, Stack, Text, TextInput } from '@mantine/core';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { CreateRuleDialogMutation } from './__generated__/CreateRuleDialogMutation.graphql';
import type { ProgramConditions } from './conditions';

type Props = {
  conditions: ProgramConditions;
  defaultName: string;
  onClose: () => void;
};

// CreateRuleDialog は、番組の検索の条件から、名前を付けてルールを作る。条件の細かい編集は、ルールの画面で行う。
export function CreateRuleDialog({ conditions, defaultName, onClose }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState(defaultName);
  const [result, setResult] = useState<'created' | 'error' | null>(null);
  const [commit, inFlight] = useMutation<CreateRuleDialogMutation>(graphql`
    mutation CreateRuleDialogMutation($input: RuleInput!) {
      createRule(input: $input) {
        id
      }
    }
  `);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setResult(null);
    commit({
      variables: { input: { name: name.trim(), conditions } },
      // Relay は、data とともに返ったエラーを、onError ではなく onCompleted に渡す。
      onCompleted: (_response, errors) =>
        setResult(errors != null && errors.length > 0 ? 'error' : 'created'),
      onError: () => setResult('error'),
    });
  };

  return (
    <Modal opened onClose={onClose} title={t('search.rule.title')}>
      <form onSubmit={submit}>
        <Stack>
          <Text size="sm" c="dimmed">
            {t('search.rule.description')}
          </Text>
          <TextInput
            label={t('search.rule.name')}
            value={name}
            onChange={(e) => setName(e.currentTarget.value)}
            required
            maxLength={200}
            disabled={result === 'created'}
          />
          {result === 'created' && (
            <Alert color="green" role="status">
              {t('search.rule.created')}
            </Alert>
          )}
          {result === 'error' && (
            <Alert color="red" role="alert">
              {t('search.rule.error')}
            </Alert>
          )}
          <Group justify="flex-end">
            {result === 'created' ? (
              <Button onClick={onClose}>{t('search.rule.close')}</Button>
            ) : (
              <Button type="submit" loading={inFlight} disabled={name.trim() === ''}>
                {t('search.rule.submit')}
              </Button>
            )}
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
