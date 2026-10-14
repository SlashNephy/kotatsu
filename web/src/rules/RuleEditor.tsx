import {
  Title,
  Box,
  Alert,
  Button,
  Card,
  Fieldset,
  Grid,
  Group,
  Modal,
  Select,
  NumberInput,
  Stack,
  Switch,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core';
import { Save, Trash2 } from 'lucide-react';
import { useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import classes from './RuleEditor.module.css';
import type { RuleEditorCreateMutation } from './__generated__/RuleEditorCreateMutation.graphql';
import type { RuleEditorDeleteMutation } from './__generated__/RuleEditorDeleteMutation.graphql';
import type { RuleEditorUpdateMutation } from './__generated__/RuleEditorUpdateMutation.graphql';
import { hasConditions } from '../conditions/form';
import { ConditionsFields, type ServiceOption } from '../conditions/ConditionsFields';
import { toInput, type RuleForm } from './form';
import { RulePreview } from './RulePreview';

// joinDefault は、分かれたファイルをつなぐかどうかの選択で、全体の既定値に従うことを表す値。
const joinDefault = 'default';

type Props = {
  // ruleId は、編集するルールの ID。新しいルールなら undefined。
  ruleId: string | undefined;
  initial: RuleForm;
  channelTypes: readonly { type: string; displayName: string }[];
  services: readonly ServiceOption[];
  // readOnly は、視聴者のように、ルールを変えられないかどうか。
  readOnly: boolean;
};

// RuleEditor は、ルールを編集するフォームと、条件に一致する番組のプレビュー (設計書 4.3)。
export function RuleEditor({ ruleId, initial, channelTypes, services, readOnly }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [form, setForm] = useState(initial);
  const [result, setResult] = useState<'saved' | 'error' | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [create, creating] = useMutation<RuleEditorCreateMutation>(graphql`
    mutation RuleEditorCreateMutation($input: RuleInput!) {
      createRule(input: $input) {
        id
      }
    }
  `);
  const [update, updating] = useMutation<RuleEditorUpdateMutation>(graphql`
    mutation RuleEditorUpdateMutation($id: ID!, $input: RuleInput!) {
      updateRule(id: $id, input: $input) {
        id
        name
        enabled
      }
    }
  `);
  const [remove, removing] = useMutation<RuleEditorDeleteMutation>(graphql`
    mutation RuleEditorDeleteMutation($id: ID!) {
      deleteRule(id: $id)
    }
  `);
  const input = toInput(form);
  const conditionsGiven = hasConditions(form);
  const set = (patch: Partial<RuleForm>) => {
    setResult(null);
    setForm((prev) => ({ ...prev, ...patch }));
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (readOnly || input.name === '' || !conditionsGiven || creating || updating) {
      return;
    }
    setResult(null);
    const onError = () => setResult('error');
    const failed = (errors: readonly unknown[] | null | undefined) =>
      errors != null && errors.length > 0;
    if (ruleId === undefined) {
      create({
        variables: { input },
        onCompleted: (response, errors) => {
          if (failed(errors)) {
            setResult('error');

            return;
          }
          void navigate({ to: '/rules/$ruleId', params: { ruleId: response.createRule.id } });
        },
        onError,
      });
    } else {
      update({
        variables: { id: ruleId, input },
        onCompleted: (_response, errors) => setResult(failed(errors) ? 'error' : 'saved'),
        onError,
      });
    }
  };

  return (
    <Grid>
      <Grid.Col span={{ base: 12, md: 7 }}>
        <form onSubmit={submit}>
          <Fieldset disabled={readOnly} variant="unstyled">
            <Stack>
              <Card withBorder>
                <Stack gap="sm">
                  <Title order={3} size="h5">
                    {t('rules.section.basic')}
                  </Title>
                  <TextInput
                    label={t('rules.name')}
                    value={form.name}
                    onChange={(e) => set({ name: e.currentTarget.value })}
                    required
                    maxLength={200}
                  />
                  <Textarea
                    label={t('rules.memo')}
                    value={form.memo}
                    onChange={(e) => set({ memo: e.currentTarget.value })}
                    maxLength={2000}
                    autosize
                    minRows={1}
                  />
                  <Switch
                    label={t('rules.enabled')}
                    checked={form.enabled}
                    onChange={(e) => set({ enabled: e.currentTarget.checked })}
                  />
                </Stack>
              </Card>
              <ConditionsFields
                value={form}
                onChange={set}
                channelTypes={channelTypes}
                services={services}
              />
              <Card withBorder>
                <Stack gap="sm">
                  <Title order={3} size="h5">
                    {t('rules.section.recording')}
                  </Title>
                  <Group align="flex-end">
                    <Switch
                      label={t('rules.avoidDuplicates')}
                      checked={form.avoidDuplicates}
                      onChange={(e) => set({ avoidDuplicates: e.currentTarget.checked })}
                    />
                    {form.avoidDuplicates && (
                      <NumberInput
                        aria-label={t('rules.duplicateWindowDays')}
                        placeholder={t('rules.duplicateWindowDaysPlaceholder')}
                        size="xs"
                        w={160}
                        value={form.duplicateWindowDays ?? ''}
                        onChange={(value) =>
                          set({ duplicateWindowDays: typeof value === 'number' ? value : null })
                        }
                        min={1}
                        max={3650}
                        allowDecimal={false}
                        suffix={t('rules.days')}
                      />
                    )}
                  </Group>
                  <Switch
                    label={t('rules.followEventRelay')}
                    checked={form.followEventRelay}
                    onChange={(e) => set({ followEventRelay: e.currentTarget.checked })}
                  />
                  <NumberInput
                    label={t('rules.priority')}
                    description={t('rules.priorityDescription')}
                    maw={360}
                    value={form.settings.priority ?? ''}
                    onChange={(value) =>
                      set({
                        settings: {
                          ...form.settings,
                          priority: typeof value === 'number' ? value : null,
                        },
                      })
                    }
                    min={-10}
                    max={10}
                    allowDecimal={false}
                  />
                  <Select
                    label={t('rules.joinSegments')}
                    maw={360}
                    data={[
                      { value: joinDefault, label: t('rules.joinSegmentsDefault') },
                      { value: 'true', label: t('rules.joinSegmentsOn') },
                      { value: 'false', label: t('rules.joinSegmentsOff') },
                    ]}
                    value={
                      form.settings.joinSegments === null
                        ? joinDefault
                        : String(form.settings.joinSegments)
                    }
                    onChange={(value) =>
                      set({
                        settings: {
                          ...form.settings,
                          joinSegments:
                            value === null || value === joinDefault ? null : value === 'true',
                        },
                      })
                    }
                    allowDeselect={false}
                  />
                </Stack>
              </Card>
              {!conditionsGiven && <Alert color="amber">{t('rules.noConditions')}</Alert>}
              {result === 'saved' && (
                <Alert color="green" role="status">
                  {t('rules.saved')}
                </Alert>
              )}
              {result === 'error' && (
                <Alert color="red" role="alert">
                  {t('rules.saveError')}
                </Alert>
              )}
              {!readOnly && (
                <Group justify="space-between">
                  {ruleId !== undefined ? (
                    <Button
                      type="button"
                      color="red"
                      variant="subtle"
                      onClick={() => setDeleting(true)}
                      leftSection={<Trash2 size={16} aria-hidden />}
                    >
                      {t('rules.delete')}
                    </Button>
                  ) : (
                    <span />
                  )}
                  <Button
                    type="submit"
                    loading={creating || updating}
                    disabled={input.name === '' || !conditionsGiven}
                    leftSection={<Save size={16} aria-hidden />}
                  >
                    {t('rules.save')}
                  </Button>
                </Group>
              )}
            </Stack>
          </Fieldset>
        </form>
      </Grid.Col>
      <Grid.Col span={{ base: 12, md: 5 }}>
        {/* 広い画面では、条件を変えながら結果を見られるよう、プレビューをスクロールしても見える位置に留める。 */}
        <Box className={classes.preview}>
          <RulePreview input={input} enabled={conditionsGiven} />
        </Box>
      </Grid.Col>
      {deleting && ruleId !== undefined && (
        <Modal opened onClose={() => setDeleting(false)} title={t('rules.deleteTitle')}>
          <Stack>
            <Text size="sm">{t('rules.deleteDescription')}</Text>
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setDeleting(false)}>
                {t('reservations.cancel')}
              </Button>
              <Button
                color="red"
                loading={removing}
                onClick={() =>
                  remove({
                    variables: { id: ruleId },
                    onCompleted: (_response, errors) => {
                      if (errors != null && errors.length > 0) {
                        setDeleting(false);
                        setResult('error');

                        return;
                      }
                      void navigate({ to: '/rules' });
                    },
                    onError: () => {
                      setDeleting(false);
                      setResult('error');
                    },
                  })
                }
              >
                {t('rules.delete')}
              </Button>
            </Group>
          </Stack>
        </Modal>
      )}
    </Grid>
  );
}
