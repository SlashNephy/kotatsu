import {
  Anchor,
  Code,
  Group,
  Loader,
  Select,
  Stack,
  Table,
  Text,
  TextInput,
  UnstyledButton,
} from '@mantine/core';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import type { FileNameTemplateFieldDefaultQuery } from './__generated__/FileNameTemplateFieldDefaultQuery.graphql';
import type { FileNameTemplateFieldHelpQuery } from './__generated__/FileNameTemplateFieldHelpQuery.graphql';
import type { FileNameTemplateFieldPreviewQuery } from './__generated__/FileNameTemplateFieldPreviewQuery.graphql';

// sampleProgram は、当てはめる番組の選択で、見本の番組を表す値。
const sampleProgram = 'sample';
// debounceMs は、入力が止まってからプレビューを取るまでの時間。
const debounceMs = 400;

const previewQuery = graphql`
  query FileNameTemplateFieldPreviewQuery($template: String!, $program: ID) {
    previewFileName(template: $template, program: $program) {
      fileName
      error
    }
  }
`;

type Props = {
  label: string;
  description?: string;
  value: string;
  onChange: (value: string) => void;
  // fallback は、空のときに使うテンプレート。空を許さない入力欄では渡さない。
  fallback?: string;
  // programs は、当てはめる番組の候補。見本の番組のほかに選べる。
  programs?: readonly { id: string; name: string }[];
  disabled?: boolean;
};

// FileNameTemplateField は、ファイル名のテンプレートの入力欄 (設計書 4.7)。入力が止まったら、番組を当てはめたファイル名を
// サーバーで求めて示し、Go のテンプレートを書かない利用者のために、使える書き方を表にして示す。
export function FileNameTemplateField({
  label,
  description,
  value,
  onChange,
  fallback,
  programs = [],
  disabled = false,
}: Props) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [helpOpened, setHelpOpened] = useState(false);
  const [program, setProgram] = useState(sampleProgram);
  // 候補から消えた番組を選んだままにしない。
  const programId = programs.some((p) => p.id === program) ? program : null;

  // insert は、カーソルの位置に snippet を差し込む。
  const insert = (snippet: string) => {
    const input = inputRef.current;
    const start = input?.selectionStart ?? value.length;
    const end = input?.selectionEnd ?? value.length;
    onChange(value.slice(0, start) + snippet + value.slice(end));
    requestAnimationFrame(() => {
      input?.focus();
      input?.setSelectionRange(start + snippet.length, start + snippet.length);
    });
  };

  return (
    <Stack gap={4}>
      <TextInput
        ref={inputRef}
        label={label}
        description={description}
        placeholder={fallback}
        ff="monospace"
        value={value}
        onChange={(event) => onChange(event.currentTarget.value)}
        disabled={disabled}
      />
      <Group gap="xs" align="center" wrap="nowrap">
        <Preview template={value === '' ? (fallback ?? '') : value} program={programId} />
        {programs.length > 0 && (
          <Select
            size="xs"
            w={220}
            aria-label={t('fileNameTemplate.program')}
            allowDeselect={false}
            data={[
              { value: sampleProgram, label: t('fileNameTemplate.sample') },
              ...programs.map((p) => ({ value: p.id, label: p.name })),
            ]}
            value={programId ?? sampleProgram}
            disabled={disabled}
            onChange={(next) => setProgram(next ?? sampleProgram)}
          />
        )}
      </Group>
      <Anchor
        component="button"
        type="button"
        size="xs"
        style={{ alignSelf: 'flex-start' }}
        onClick={() => setHelpOpened((o) => !o)}
      >
        {helpOpened ? t('fileNameTemplate.hideHelp') : t('fileNameTemplate.showHelp')}
      </Anchor>
      {helpOpened && (
        <Suspense fallback={<Loader size="xs" />}>
          <Help onInsert={insert} onReplace={onChange} disabled={disabled} />
        </Suspense>
      )}
    </Stack>
  );
}

// InheritedFileNameTemplateField は、空なら全体の既定値に従う入力欄。既定値を読み、空のときに当てはめる。
// 既定値を読む間は、ここだけを待たせ、まわりのフォームの入力を失わないようにする。
export function InheritedFileNameTemplateField(props: Omit<Props, 'fallback'>) {
  return (
    <Suspense fallback={<Loader size="xs" />}>
      <WithDefault {...props} />
    </Suspense>
  );
}

function WithDefault(props: Omit<Props, 'fallback'>) {
  const data = useLazyLoadQuery<FileNameTemplateFieldDefaultQuery>(
    graphql`
      query FileNameTemplateFieldDefaultQuery {
        systemSettings {
          recordingDefaults {
            fileNameTemplate
          }
        }
      }
    `,
    {},
    { fetchPolicy: 'store-or-network' },
  );

  return (
    <FileNameTemplateField
      {...props}
      fallback={data.systemSettings.recordingDefaults.fileNameTemplate}
    />
  );
}

// Preview は、テンプレートを当てはめたファイル名か、テンプレートの誤りを示す。
function Preview({ template, program }: { template: string; program: string | null }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [result, setResult] = useState<{ fileName: string | null; error: string | null } | null>(
    null,
  );
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (template.trim() === '') {
      setLoading(false);
      setResult(null);

      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      setLoading(true);
      fetchQuery<FileNameTemplateFieldPreviewQuery>(
        environment,
        previewQuery,
        { template, program },
        { fetchPolicy: 'network-only' },
      )
        .toPromise()
        .then(
          (data) => {
            if (!cancelled && data !== undefined) {
              setResult({
                fileName: data.previewFileName.fileName ?? null,
                error: data.previewFileName.error ?? null,
              });
            }
          },
          () => {
            if (!cancelled) {
              setResult({ fileName: null, error: t('fileNameTemplate.previewError') });
            }
          },
        )
        .finally(() => {
          if (!cancelled) {
            setLoading(false);
          }
        });
    }, debounceMs);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [template, program, environment, t]);

  if (result === null) {
    return loading ? <Loader size="xs" /> : null;
  }
  if (result.error != null) {
    return (
      <Text size="xs" c="red" role="alert" style={{ flex: 1 }}>
        {t('fileNameTemplate.invalid', { error: result.error })}
      </Text>
    );
  }

  return (
    <Text size="xs" c="dimmed" style={{ flex: 1, wordBreak: 'break-all' }}>
      {t('fileNameTemplate.preview')} <Code>{result.fileName}</Code>
    </Text>
  );
}

// Help は、使える書き方の表と、よく使うテンプレートを示す。行を押すとカーソルの位置に差し込み、よく使うテンプレートを押すと置き換える。
function Help({
  onInsert,
  onReplace,
  disabled,
}: {
  onInsert: (snippet: string) => void;
  onReplace: (template: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<FileNameTemplateFieldHelpQuery>(
    graphql`
      query FileNameTemplateFieldHelpQuery {
        fileNameTemplateHelp {
          items {
            snippet
            description
            example
          }
          presets {
            name
            template
            example
          }
        }
      }
    `,
    {},
    { fetchPolicy: 'store-or-network' },
  );
  const help = data.fileNameTemplateHelp;

  return (
    <Stack gap="xs">
      <Text size="xs" c="dimmed">
        {t('fileNameTemplate.helpNote')}
      </Text>
      <Table fz="xs" verticalSpacing={4} highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('fileNameTemplate.snippet')}</Table.Th>
            <Table.Th>{t('fileNameTemplate.meaning')}</Table.Th>
            <Table.Th>{t('fileNameTemplate.example')}</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {help.items.map((item) => (
            <Table.Tr key={item.snippet}>
              <Table.Td>
                <UnstyledButton
                  disabled={disabled}
                  onClick={() => onInsert(item.snippet)}
                  aria-label={t('fileNameTemplate.insert', { snippet: item.snippet })}
                >
                  <Code>{item.snippet}</Code>
                </UnstyledButton>
              </Table.Td>
              <Table.Td>{item.description}</Table.Td>
              <Table.Td>{item.example}</Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      <Text size="xs" fw={700}>
        {t('fileNameTemplate.presets')}
      </Text>
      {help.presets.map((preset) => (
        <UnstyledButton
          key={preset.name}
          disabled={disabled}
          onClick={() => onReplace(preset.template)}
          aria-label={t('fileNameTemplate.usePreset', { name: preset.name })}
        >
          <Stack gap={0}>
            <Text size="xs">{preset.name}</Text>
            <Code block>{preset.template}</Code>
            <Text size="xs" c="dimmed">
              {preset.example}
            </Text>
          </Stack>
        </UnstyledButton>
      ))}
    </Stack>
  );
}
