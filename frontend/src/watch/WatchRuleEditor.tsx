import {
  Alert,
  Box,
  Button,
  Card,
  Grid,
  Group,
  Modal,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { useNavigate } from '@tanstack/react-router';
import { Save, Trash2 } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { ConditionsFields, type ServiceOption } from '../conditions/ConditionsFields';
import { hasConditions } from '../conditions/form';
import { graphQLErrorDetails } from '../relay/environment';
import classes from '../rules/RuleEditor.module.css';
import { RulePreview } from '../rules/RulePreview';
import { useSyncedForm } from '../rules/useSyncedForm';
import type { WatchRuleEditorCreateMutation } from './__generated__/WatchRuleEditorCreateMutation.graphql';
import type { WatchRuleEditorDeleteMutation } from './__generated__/WatchRuleEditorDeleteMutation.graphql';
import type { WatchRuleEditorUpdateMutation } from './__generated__/WatchRuleEditorUpdateMutation.graphql';
import { toPreviewInput, toWatchRuleInput, type WatchRuleForm } from './watchRuleForm';
import { maxWatchRules } from './WatchRuleList';

type Props = {
  // watchRuleId は、編集する視聴ルールの ID。新しい視聴ルールなら undefined。
  watchRuleId: string | undefined;
  initial: WatchRuleForm;
  channelTypes: readonly { type: string; displayName: string }[];
  services: readonly ServiceOption[];
};

// watchRuleEditorFragment は、編集の画面が読む視聴ルールの項目。保存の結果でも同じ項目を受け取り、ストアに古い値を残さない。
export const watchRuleEditorFragment = graphql`
  fragment WatchRuleEditor_watchRule on WatchRule {
    id
    name
    enabled
    conditions {
      keywords {
        text
        regexp
        caseSensitive
        fields
      }
      keywordMatch
      excludeKeywords {
        text
        regexp
        caseSensitive
        fields
      }
      channelTypes
      services {
        id
      }
      excludeServices {
        id
      }
      genres {
        lv1
        lv2
      }
      excludeGenres {
        lv1
        lv2
      }
      timeRanges {
        weekdays
        startMinute
        durationMinutes
      }
      minDurationSeconds
      maxDurationSeconds
      freeOnly
      marks
      excludeMarks
    }
  }
`;

// sameWatchRule は、2 つの入力が、保存すると同じ視聴ルールになるかどうかを返す。
function sameWatchRule(a: WatchRuleForm, b: WatchRuleForm) {
  return JSON.stringify(toWatchRuleInput(a)) === JSON.stringify(toWatchRuleInput(b));
}

// WatchRuleEditor は、視聴ルールを編集するフォームと、条件に一致する番組のプレビュー (設計書 4.8)。
// 条件の入力とプレビューは、録画のルールの編集の画面と同じものを使う。
export function WatchRuleEditor({ watchRuleId, initial, channelTypes, services }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { form, outdated, set: setFields, reload } = useSyncedForm(initial, sameWatchRule);
  const [result, setResult] = useState<'saved' | 'error' | 'limit' | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [create, creating] = useMutation<WatchRuleEditorCreateMutation>(graphql`
    mutation WatchRuleEditorCreateMutation($input: WatchRuleInput!) {
      createWatchRule(input: $input) {
        id
      }
    }
  `);
  const [update, updating] = useMutation<WatchRuleEditorUpdateMutation>(graphql`
    mutation WatchRuleEditorUpdateMutation($id: ID!, $input: WatchRuleInput!) {
      updateWatchRule(id: $id, input: $input) {
        ...WatchRuleEditor_watchRule
      }
    }
  `);
  const [remove, removing] = useMutation<WatchRuleEditorDeleteMutation>(graphql`
    mutation WatchRuleEditorDeleteMutation($id: ID!) {
      deleteWatchRule(id: $id)
    }
  `);
  const input = toWatchRuleInput(form);
  const conditionsGiven = hasConditions(form);
  const set = (patch: Partial<WatchRuleForm>) => {
    setResult(null);
    setFields(patch);
  };
  const failed = (errors: readonly unknown[] | null | undefined) =>
    errors != null && errors.length > 0;
  // onError は、保存できなかった理由を示す。上限に達していたら (CONFLICT)、そのことを示す。
  const onError = (error: Error) =>
    setResult(graphQLErrorDetails(error).code === 'CONFLICT' ? 'limit' : 'error');

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (input.name === '' || !conditionsGiven || creating || updating) {
      return;
    }
    setResult(null);
    if (watchRuleId === undefined) {
      create({
        variables: { input },
        onCompleted: (response, errors) => {
          if (failed(errors)) {
            setResult('error');

            return;
          }
          void navigate({
            to: '/rules/watch/$watchRuleId',
            params: { watchRuleId: response.createWatchRule.id },
          });
        },
        onError,
      });
    } else {
      update({
        variables: { id: watchRuleId, input },
        onCompleted: (_response, errors) => setResult(failed(errors) ? 'error' : 'saved'),
        onError,
      });
    }
  };

  return (
    <Grid>
      <Grid.Col span={{ base: 12, md: 7 }}>
        <form onSubmit={submit}>
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
                <Switch
                  label={t('rules.enabled')}
                  checked={form.enabled}
                  onChange={(e) => set({ enabled: e.currentTarget.checked })}
                />
                <Text size="xs" c="dimmed">
                  {t('watchRules.editorDescription')}
                </Text>
              </Stack>
            </Card>
            <ConditionsFields
              value={form}
              onChange={set}
              channelTypes={channelTypes}
              services={services}
            />
            {!conditionsGiven && <Alert color="amber">{t('rules.noConditions')}</Alert>}
            {outdated && (
              <Alert color="amber" role="status">
                <Group justify="space-between" gap="xs">
                  <Text size="sm">{t('watchRules.outdated')}</Text>
                  <Button
                    size="compact-sm"
                    variant="light"
                    onClick={() => {
                      setResult(null);
                      reload();
                    }}
                  >
                    {t('rules.reload')}
                  </Button>
                </Group>
              </Alert>
            )}
            {result === 'saved' && (
              <Alert color="green" role="status">
                {t('watchRules.saved')}
              </Alert>
            )}
            {result === 'error' && (
              <Alert color="red" role="alert">
                {t('rules.saveError')}
              </Alert>
            )}
            {result === 'limit' && (
              <Alert color="red" role="alert">
                {t('watchRules.limit', { count: maxWatchRules })}
              </Alert>
            )}
            <Group justify="space-between">
              {watchRuleId !== undefined ? (
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
          </Stack>
        </form>
      </Grid.Col>
      <Grid.Col span={{ base: 12, md: 5 }}>
        <Box className={classes.preview}>
          <RulePreview input={toPreviewInput(form)} enabled={conditionsGiven} />
        </Box>
      </Grid.Col>
      {deleting && watchRuleId !== undefined && (
        <Modal opened onClose={() => setDeleting(false)} title={t('watchRules.deleteTitle')}>
          <Stack>
            <Text size="sm">{t('watchRules.deleteDescription')}</Text>
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setDeleting(false)}>
                {t('reservations.cancel')}
              </Button>
              <Button
                color="red"
                loading={removing}
                onClick={() =>
                  remove({
                    variables: { id: watchRuleId },
                    onCompleted: (_response, errors) => {
                      if (failed(errors)) {
                        setDeleting(false);
                        setResult('error');

                        return;
                      }
                      void navigate({ to: '/rules', search: { watch: true } });
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
