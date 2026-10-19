import {
  ActionIcon,
  Button,
  Card,
  Checkbox,
  Chip,
  Group,
  MultiSelect,
  NumberInput,
  SegmentedControl,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { MonitorX, Plus, RadioTower, Search, SearchX, Shapes, Stamp, Tv, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { markOptions } from '../futon/programMarks';
import { GenreMultiSelect } from '../guide/GenreMultiSelect';
import { decodeGenre, encodeGenre } from '../guide/guideView';
import { ServiceLabel } from '../guide/ServiceLabel';
import {
  emptyKeyword,
  keywordFields,
  type ConditionsForm,
  type KeywordField,
  type KeywordMatch,
  type KeywordRow,
  type TimeRangeRow,
} from './form';

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

export type ServiceOption = {
  id: string;
  name: string;
  logoUrl: string | null | undefined;
  serviceId: number;
  channelType: { displayName: string };
};

type Props = {
  value: ConditionsForm;
  onChange: (patch: Partial<ConditionsForm>) => void;
  channelTypes: readonly { type: string; displayName: string }[];
  services: readonly ServiceOption[];
};

// ConditionsFields は、番組の条件の入力欄 (設計書 3.5)。キーワード、放送局、ジャンル、スケジュールのカードを並べる。
// 番組の検索とルールの編集で、同じものを使う。
export function ConditionsFields({ value, onChange, channelTypes, services }: Props) {
  const { t } = useTranslation();

  return (
    <>
      <Card withBorder>
        <Stack gap="sm">
          <Title order={3} size="h5">
            {t('rules.section.keywords')}
          </Title>
          <KeywordRows
            label={t('rules.keywords')}
            description={
              value.keywordMatch === 'ANY'
                ? t('rules.keywordsAnyDescription')
                : t('rules.keywordsDescription')
            }
            match={value.keywordMatch}
            onMatchChange={(keywordMatch) => onChange({ keywordMatch })}
            icon={<Search size={16} aria-hidden />}
            rows={value.keywords}
            onChange={(keywords) => onChange({ keywords })}
          />
          <KeywordRows
            label={t('rules.excludeKeywords')}
            description={t('rules.excludeKeywordsDescription')}
            icon={<SearchX size={16} aria-hidden />}
            rows={value.excludeKeywords}
            onChange={(excludeKeywords) => onChange({ excludeKeywords })}
          />
          <MarkMultiSelect
            label={t('rules.marks')}
            description={t('rules.marksDescription')}
            keep={[...value.marks, ...value.excludeMarks]}
            value={value.marks}
            onChange={(marks) => onChange({ marks })}
          />
          <MarkMultiSelect
            label={t('rules.excludeMarks')}
            description={t('rules.excludeMarksDescription')}
            keep={[...value.marks, ...value.excludeMarks]}
            value={value.excludeMarks}
            onChange={(excludeMarks) => onChange({ excludeMarks })}
          />
        </Stack>
      </Card>
      <Card withBorder>
        <Stack gap="sm">
          <Title order={3} size="h5">
            {t('rules.section.broadcast')}
          </Title>
          <MultiSelect
            label={t('rules.channelTypes')}
            leftSection={<RadioTower size={16} aria-hidden />}
            data={channelTypes.map((type) => ({ value: type.type, label: type.displayName }))}
            value={value.channelTypes}
            onChange={(selected) => onChange({ channelTypes: selected })}
            clearable
          />
          <ServiceMultiSelect
            label={t('rules.services')}
            icon={<Tv size={16} aria-hidden />}
            services={services}
            value={value.services}
            onChange={(selected) => onChange({ services: selected })}
          />
          <ServiceMultiSelect
            label={t('rules.excludeServices')}
            description={t('rules.excludeServicesDescription')}
            icon={<MonitorX size={16} aria-hidden />}
            services={services}
            value={value.excludeServices}
            onChange={(selected) => onChange({ excludeServices: selected })}
          />
          <Switch
            label={t('search.freeOnly')}
            checked={value.freeOnly}
            onChange={(e) => onChange({ freeOnly: e.currentTarget.checked })}
          />
        </Stack>
      </Card>
      <Card withBorder>
        <Stack gap="sm">
          <Title order={3} size="h5">
            {t('rules.section.genres')}
          </Title>
          <GenreMultiSelect
            label={t('rules.genres')}
            leftSection={<Shapes size={16} aria-hidden />}
            keep={[...value.genres, ...value.excludeGenres]}
            value={value.genres.map(encodeGenre)}
            onChange={(selected) => onChange({ genres: selected.map(decodeGenre) })}
          />
          <GenreMultiSelect
            label={t('rules.excludeGenres')}
            leftSection={<Shapes size={16} aria-hidden />}
            keep={[...value.genres, ...value.excludeGenres]}
            value={value.excludeGenres.map(encodeGenre)}
            onChange={(selected) => onChange({ excludeGenres: selected.map(decodeGenre) })}
          />
        </Stack>
      </Card>
      <Card withBorder>
        <Stack gap="sm">
          <Title order={3} size="h5">
            {t('rules.section.schedule')}
          </Title>
          <TimeRangeRows
            rows={value.timeRanges}
            onChange={(timeRanges) => onChange({ timeRanges })}
          />
          <Group grow>
            <NumberInput
              label={t('rules.minDuration')}
              value={
                value.minDurationSeconds == null ? '' : Math.round(value.minDurationSeconds / 60)
              }
              onChange={(minutes) =>
                onChange({ minDurationSeconds: typeof minutes === 'number' ? minutes * 60 : null })
              }
              min={1}
              allowDecimal={false}
              suffix={t('guide.view.minutes')}
            />
            <NumberInput
              label={t('rules.maxDuration')}
              value={
                value.maxDurationSeconds == null ? '' : Math.round(value.maxDurationSeconds / 60)
              }
              onChange={(minutes) =>
                onChange({ maxDurationSeconds: typeof minutes === 'number' ? minutes * 60 : null })
              }
              min={1}
              allowDecimal={false}
              suffix={t('guide.view.minutes')}
            />
          </Group>
        </Stack>
      </Card>
    </>
  );
}

type KeywordRowsProps = {
  label: string;
  description: string;
  // match と onMatchChange は、キーワードの照合のしかた。渡したときだけ、見出しの横に切り替えを出す。
  match?: KeywordMatch;
  onMatchChange?: (match: KeywordMatch) => void;
  icon: ReactNode;
  rows: KeywordRow[];
  onChange: (rows: KeywordRow[]) => void;
};

// MarkMultiSelect は、番組名の印 (新、字など) を選ぶ欄。
// keep は、選択肢になくても残す印。API で保存した、一覧にない印を選んだままにする。
function MarkMultiSelect({
  label,
  description,
  keep,
  value,
  onChange,
}: {
  label: string;
  description: string;
  keep: readonly string[];
  value: string[];
  onChange: (selected: string[]) => void;
}) {
  const { t } = useTranslation();
  const known: readonly string[] = [...markOptions.ideographic, ...markOptions.alphanumeric];
  const others = [...new Set(keep)].filter((mark) => !known.includes(mark));

  return (
    <MultiSelect
      label={label}
      description={description}
      leftSection={<Stamp size={16} aria-hidden />}
      data={[
        { group: t('rules.markGroup.ideographic'), items: [...markOptions.ideographic] },
        { group: t('rules.markGroup.alphanumeric'), items: [...markOptions.alphanumeric] },
        ...(others.length > 0 ? [{ group: t('rules.markGroup.others'), items: others }] : []),
      ]}
      value={value}
      onChange={onChange}
      searchable
      clearable
      maxDropdownHeight={320}
    />
  );
}

// ServiceMultiSelect は、サービスを選ぶ欄。同じ名前のサービスが別の種別にもあるので、種別とサービス ID を添える。
function ServiceMultiSelect({
  label,
  description,
  icon,
  services,
  value,
  onChange,
}: {
  label: string;
  description?: string;
  icon: ReactNode;
  services: readonly ServiceOption[];
  value: string[];
  onChange: (selected: string[]) => void;
}) {
  return (
    <MultiSelect
      label={label}
      description={description}
      leftSection={icon}
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
      value={value}
      onChange={onChange}
      searchable
      clearable
    />
  );
}

// KeywordRows は、キーワードの行を足し、変え、消す。
function KeywordRows({
  label,
  description,
  match,
  onMatchChange,
  icon,
  rows,
  onChange,
}: KeywordRowsProps) {
  const { t } = useTranslation();
  const change = (index: number, patch: Partial<KeywordRow>) =>
    onChange(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));

  return (
    <Stack gap={12}>
      <div>
        <Group justify="space-between" gap="xs" wrap="wrap">
          <Text size="sm" fw={500}>
            {label}
          </Text>
          {match !== undefined && onMatchChange !== undefined && (
            <SegmentedControl
              size="xs"
              aria-label={t('rules.keywordMatch.label')}
              value={match}
              onChange={(next) => onMatchChange(next === 'ANY' ? 'ANY' : 'ALL')}
              data={[
                { value: 'ALL', label: t('rules.keywordMatch.ALL') },
                { value: 'ANY', label: t('rules.keywordMatch.ANY') },
              ]}
            />
          )}
        </Group>
        <Text size="xs" c="dimmed">
          {description}
        </Text>
      </div>
      {rows.map((row, index) => (
        // 狭い列にも収まるよう、1 段目に語と消すボタン、2 段目に照合のしかたを並べる。
        <Stack key={index} gap={8}>
          <Group gap="xs" wrap="nowrap">
            <TextInput
              aria-label={label}
              leftSection={icon}
              value={row.text}
              onChange={(e) => change(index, { text: e.currentTarget.value })}
              style={{ flex: 1 }}
              ff={row.regexp ? 'monospace' : undefined}
            />
            <ActionIcon
              variant="subtle"
              color="gray"
              aria-label={t('rules.removeRow')}
              onClick={() => onChange(rows.filter((_, i) => i !== index))}
            >
              <X size={16} aria-hidden />
            </ActionIcon>
          </Group>
          <Group gap="sm" wrap="wrap" pl="sm">
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
          </Group>
          <Group gap="sm" wrap="wrap" pl="sm">
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
          </Group>
        </Stack>
      ))}
      <Group>
        <Button
          type="button"
          variant="default"
          size="xs"
          leftSection={<Plus size={14} aria-hidden />}
          onClick={() => onChange([...rows, emptyKeyword()])}
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
          {t('rules.rangeFrom')}
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
            <X size={16} aria-hidden />
          </ActionIcon>
        </Group>
      ))}
      <Group>
        <Button
          type="button"
          variant="default"
          size="xs"
          leftSection={<Plus size={14} aria-hidden />}
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
