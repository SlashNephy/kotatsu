import {
  Title,
  Box,
  ActionIcon,
  Alert,
  Button,
  Card,
  Checkbox,
  Chip,
  Fieldset,
  Grid,
  Group,
  Modal,
  MultiSelect,
  Select,
  NumberInput,
  Stack,
  Switch,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core';
import { useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { genreKey } from '../guide/genres';
import { decodeGenre, encodeGenre } from '../guide/guideView';
import classes from './RuleEditor.module.css';
import type { RuleEditorCreateMutation } from './__generated__/RuleEditorCreateMutation.graphql';
import type { RuleEditorDeleteMutation } from './__generated__/RuleEditorDeleteMutation.graphql';
import type { RuleEditorUpdateMutation } from './__generated__/RuleEditorUpdateMutation.graphql';
import {
  hasConditions,
  toInput,
  type KeywordField,
  type KeywordRow,
  type RuleForm,
  type TimeRangeRow,
} from './form';
import { RulePreview } from './RulePreview';
import { ServiceLabel } from '../guide/ServiceLabel';

// joinDefault は、分かれたファイルをつなぐかどうかの選択で、全体の既定値に従うことを表す値。
const joinDefault = 'default';

type Props = {
  // ruleId は、編集するルールの ID。新しいルールなら undefined。
  ruleId: string | undefined;
  initial: RuleForm;
  channelTypes: readonly { type: string; displayName: string }[];
  services: readonly {
    id: string;
    name: string;
    logoUrl: string | null | undefined;
    serviceId: number;
    channelType: { displayName: string };
  }[];
  // readOnly は、視聴者のように、ルールを変えられないかどうか。
  readOnly: boolean;
};

const keywordFields: KeywordField[] = ['NAME', 'DESCRIPTION', 'EXTENDED'];
const selectableGenres = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 15];
const weekdays = [0, 1, 2, 3, 4, 5, 6];

const fieldKeys = {
  NAME: 'rules.field.NAME',
  DESCRIPTION: 'rules.field.DESCRIPTION',
  EXTENDED: 'rules.field.EXTENDED',
} as const;
const weekdayKeys = [
  'rules.weekday.0',
  'rules.weekday.1',
  'rules.weekday.2',
  'rules.weekday.3',
  'rules.weekday.4',
  'rules.weekday.5',
  'rules.weekday.6',
] as const;

// toTime と fromTime は、0 時からの分と、HH:MM を行き来させる。
const toTime = (minute: number) =>
  `${String(Math.floor(minute / 60)).padStart(2, '0')}:${String(minute % 60).padStart(2, '0')}`;
const fromTime = (time: string) => {
  const [hour = '0', minute = '0'] = time.split(':');

  return Number(hour) * 60 + Number(minute);
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

  const genreOptions = [
    ...selectableGenres.map((lv1) => ({ lv1, lv2: null })),
    ...initial.genres,
    ...initial.excludeGenres,
  ]
    .map(encodeGenre)
    .filter((value, index, all) => all.indexOf(value) === index)
    .map((value) => {
      const genre = decodeGenre(value);
      const label = t(genreKey(genre.lv1));

      return {
        value,
        label:
          genre.lv2 == null ? label : t('guide.view.subGenre', { genre: label, lv2: genre.lv2 }),
      };
    });

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
              <Card withBorder>
                <Stack gap="sm">
                  <Title order={3} size="h5">
                    {t('rules.section.keywords')}
                  </Title>
                  <KeywordRows
                    label={t('rules.keywords')}
                    description={t('rules.keywordsDescription')}
                    rows={form.keywords}
                    onChange={(keywords) => set({ keywords })}
                  />
                  <KeywordRows
                    label={t('rules.excludeKeywords')}
                    description={t('rules.excludeKeywordsDescription')}
                    rows={form.excludeKeywords}
                    onChange={(excludeKeywords) => set({ excludeKeywords })}
                  />
                </Stack>
              </Card>
              <Card withBorder>
                <Stack gap="sm">
                  <Title order={3} size="h5">
                    {t('rules.section.targets')}
                  </Title>
                  <MultiSelect
                    label={t('rules.channelTypes')}
                    description={t('rules.channelTypesDescription')}
                    data={channelTypes.map((type) => ({
                      value: type.type,
                      label: type.displayName,
                    }))}
                    value={form.channelTypes}
                    onChange={(channelTypes) => set({ channelTypes })}
                    clearable
                  />
                  <MultiSelect
                    label={t('rules.services')}
                    data={services.map((service) => ({
                      value: service.id,
                      label: `${service.name}（${service.channelType.displayName}・${service.serviceId}）`,
                    }))}
                    renderOption={({ option }) => (
                      <ServiceLabel
                        name={option.label}
                        logoUrl={services.find((service) => service.id === option.value)?.logoUrl}
                      />
                    )}
                    value={form.services}
                    onChange={(selected) => set({ services: selected })}
                    searchable
                    clearable
                  />
                  <MultiSelect
                    label={t('rules.genres')}
                    data={genreOptions}
                    value={form.genres.map(encodeGenre)}
                    onChange={(genres) => set({ genres: genres.map(decodeGenre) })}
                    clearable
                  />
                  <MultiSelect
                    label={t('rules.excludeGenres')}
                    data={genreOptions}
                    value={form.excludeGenres.map(encodeGenre)}
                    onChange={(genres) => set({ excludeGenres: genres.map(decodeGenre) })}
                    clearable
                  />
                  <TimeRangeRows
                    rows={form.timeRanges}
                    onChange={(timeRanges) => set({ timeRanges })}
                  />
                  <Group grow>
                    <NumberInput
                      label={t('rules.minDuration')}
                      value={form.minDurationMinutes ?? ''}
                      onChange={(value) =>
                        set({ minDurationMinutes: typeof value === 'number' ? value : null })
                      }
                      min={1}
                      allowDecimal={false}
                      suffix={t('guide.view.minutes')}
                    />
                    <NumberInput
                      label={t('rules.maxDuration')}
                      value={form.maxDurationMinutes ?? ''}
                      onChange={(value) =>
                        set({ maxDurationMinutes: typeof value === 'number' ? value : null })
                      }
                      min={1}
                      allowDecimal={false}
                      suffix={t('guide.view.minutes')}
                    />
                  </Group>
                  <Switch
                    label={t('search.freeOnly')}
                    checked={form.freeOnly}
                    onChange={(e) => set({ freeOnly: e.currentTarget.checked })}
                  />
                </Stack>
              </Card>
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

type KeywordRowsProps = {
  label: string;
  description: string;
  rows: KeywordRow[];
  onChange: (rows: KeywordRow[]) => void;
};

// KeywordRows は、キーワードの行を足し、変え、消す。
function KeywordRows({ label, description, rows, onChange }: KeywordRowsProps) {
  const { t } = useTranslation();
  const change = (index: number, patch: Partial<KeywordRow>) =>
    onChange(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));

  return (
    <Stack gap={6}>
      <div>
        <Text size="sm" fw={500}>
          {label}
        </Text>
        <Text size="xs" c="dimmed">
          {description}
        </Text>
      </div>
      {rows.map((row, index) => (
        <Group key={index} gap="xs" align="center" wrap="wrap">
          <TextInput
            aria-label={label}
            value={row.text}
            onChange={(e) => change(index, { text: e.currentTarget.value })}
            style={{ flex: '1 1 180px' }}
            ff={row.regexp ? 'monospace' : undefined}
          />
          <Chip.Group
            multiple
            value={row.fields}
            onChange={(fields) => change(index, { fields: fields as KeywordField[] })}
          >
            <Group gap={4}>
              {keywordFields.map((field) => (
                <Chip key={field} value={field} size="xs">
                  {t(fieldKeys[field])}
                </Chip>
              ))}
            </Group>
          </Chip.Group>
          <Checkbox
            size="xs"
            label={t('rules.regexp')}
            checked={row.regexp}
            onChange={(e) => change(index, { regexp: e.currentTarget.checked })}
          />
          <Checkbox
            size="xs"
            label={t('rules.caseSensitive')}
            checked={row.caseSensitive}
            onChange={(e) => change(index, { caseSensitive: e.currentTarget.checked })}
          />
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t('rules.removeRow')}
            onClick={() => onChange(rows.filter((_, i) => i !== index))}
          >
            ×
          </ActionIcon>
        </Group>
      ))}
      <Group>
        <Button
          type="button"
          variant="default"
          size="xs"
          onClick={() =>
            onChange([...rows, { text: '', regexp: false, caseSensitive: false, fields: ['NAME'] }])
          }
        >
          {t('rules.addKeyword')}
        </Button>
      </Group>
    </Stack>
  );
}

type TimeRangeRowsProps = {
  rows: TimeRangeRow[];
  onChange: (rows: TimeRangeRow[]) => void;
};

// TimeRangeRows は、曜日と時間帯の行を足し、変え、消す。
function TimeRangeRows({ rows, onChange }: TimeRangeRowsProps) {
  const { t } = useTranslation();
  const change = (index: number, patch: Partial<TimeRangeRow>) =>
    onChange(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));

  return (
    <Stack gap={6}>
      <Stack gap={0}>
        <Text size="sm" fw={500}>
          {t('rules.timeRanges')}
        </Text>
        <Text size="xs" c="dimmed">
          {t('rules.timeRangesDescription')}
        </Text>
      </Stack>
      {rows.map((row, index) => (
        <Group key={index} gap="xs" wrap="wrap">
          <Chip.Group
            multiple
            value={row.weekdays.map(String)}
            onChange={(values) =>
              change(index, { weekdays: values.map(Number).sort((a, b) => a - b) })
            }
          >
            <Group gap={4}>
              {weekdays.map((weekday) => (
                <Chip key={weekday} value={String(weekday)} size="xs">
                  {t(weekdayKeys[weekday]!)}
                </Chip>
              ))}
            </Group>
          </Chip.Group>
          <TextInput
            type="time"
            aria-label={t('rules.startTime')}
            value={toTime(row.startMinute)}
            onChange={(e) => change(index, { startMinute: fromTime(e.currentTarget.value) })}
            w={110}
            size="xs"
          />
          <NumberInput
            aria-label={t('rules.rangeDuration')}
            value={row.durationMinutes}
            onChange={(value) =>
              change(index, { durationMinutes: typeof value === 'number' ? value : 60 })
            }
            min={1}
            max={7 * 24 * 60}
            allowDecimal={false}
            suffix={t('guide.view.minutes')}
            w={110}
            size="xs"
          />
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t('rules.removeRow')}
            onClick={() => onChange(rows.filter((_, i) => i !== index))}
          >
            ×
          </ActionIcon>
        </Group>
      ))}
      <Group>
        <Button
          type="button"
          variant="default"
          size="xs"
          onClick={() =>
            onChange([...rows, { weekdays: [], startMinute: 20 * 60, durationMinutes: 60 }])
          }
        >
          {t('rules.addTimeRange')}
        </Button>
      </Group>
    </Stack>
  );
}
