import {
  Alert,
  Button,
  Group,
  Modal,
  MultiSelect,
  NumberInput,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { GuideViewEditorCreateMutation } from './__generated__/GuideViewEditorCreateMutation.graphql';
import type { GuideViewEditorDeleteMutation } from './__generated__/GuideViewEditorDeleteMutation.graphql';
import type { GuideViewEditorUpdateMutation } from './__generated__/GuideViewEditorUpdateMutation.graphql';
import { GenreMultiSelect } from './GenreMultiSelect';
import { decodeGenre, encodeGenre, type Genre } from './guideView';
import { ServiceLabel } from './ServiceLabel';

// EditableGuideView は、編集する番組表ビュー。id がなければ、新しく作る。
export type EditableGuideView = {
  id: string | undefined;
  name: string;
  genres: readonly Genre[];
  excludeGenres: readonly Genre[];
  minDurationSeconds: number | null | undefined;
  services: readonly string[];
  hideEmptyServices: boolean;
};

type Props = {
  view: EditableGuideView;
  // services は、並びに選べるサービス。
  services: readonly {
    id: string;
    name: string;
    logoUrl: string | null | undefined;
    serviceId: number;
    channelType: { displayName: string };
  }[];
  onClose: () => void;
  // onSaved は、保存したビューの ID を受け取る。消したときは undefined を受け取る。
  onSaved: (id: string | undefined) => void;
};

// GuideViewEditor は、番組表ビューを作り、変え、消すダイアログ。
export function GuideViewEditor({ view, services, onClose, onSaved }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState(view.name);
  const [genres, setGenres] = useState(view.genres.map(encodeGenre));
  const [excludeGenres, setExcludeGenres] = useState(view.excludeGenres.map(encodeGenre));
  // 最短の長さは秒で持ち、画面では分で見せる。入力を変えていなければ、秒の値をそのまま送る。
  const [minSeconds, setMinSeconds] = useState(view.minDurationSeconds ?? null);
  const [serviceOrder, setServiceOrder] = useState([...view.services]);
  const [hideEmpty, setHideEmpty] = useState(view.hideEmptyServices);
  const [error, setError] = useState<string | null>(null);
  const [create, creating] = useMutation<GuideViewEditorCreateMutation>(graphql`
    mutation GuideViewEditorCreateMutation($input: GuideViewInput!) {
      createGuideView(input: $input) {
        id
      }
    }
  `);
  const [update, updating] = useMutation<GuideViewEditorUpdateMutation>(graphql`
    mutation GuideViewEditorUpdateMutation($id: ID!, $input: GuideViewInput!) {
      updateGuideView(id: $id, input: $input) {
        id
      }
    }
  `);
  const [remove, removing] = useMutation<GuideViewEditorDeleteMutation>(graphql`
    mutation GuideViewEditorDeleteMutation($id: ID!) {
      deleteGuideView(id: $id)
    }
  `);

  // 名前のない中分類の条件 (API で作ったビューにあるものなど) は、消すまで選択肢に残す。
  const keep = [...view.genres, ...view.excludeGenres];

  const submit = (event: FormEvent) => {
    event.preventDefault();
    // 空白だけの名前は、Enter で送られても保存しない。
    if (name.trim() === '') {
      return;
    }
    setError(null);
    const input = {
      name: name.trim(),
      genres: genres.map(decodeGenre),
      excludeGenres: excludeGenres.map(decodeGenre),
      minDurationSeconds: minSeconds,
      services: serviceOrder,
      hideEmptyServices: hideEmpty,
    };
    const onError = () => setError(t('guide.view.saveError'));
    if (view.id === undefined) {
      create({
        variables: { input },
        onCompleted: (response) => onSaved(response.createGuideView.id),
        onError,
      });
    } else {
      const id = view.id;
      update({ variables: { id, input }, onCompleted: () => onSaved(id), onError });
    }
  };

  const deleteView = () => {
    if (view.id === undefined) {
      return;
    }
    remove({
      variables: { id: view.id },
      onCompleted: () => onSaved(undefined),
      onError: () => setError(t('guide.view.deleteError')),
    });
  };

  return (
    <Modal
      opened
      onClose={onClose}
      title={view.id === undefined ? t('guide.view.createTitle') : t('guide.view.editTitle')}
    >
      <form onSubmit={submit}>
        <Stack>
          {view.id === undefined && (
            <Text size="sm" c="dimmed">
              {t('guide.view.createHint')}
            </Text>
          )}
          <TextInput
            label={t('guide.view.name')}
            value={name}
            onChange={(e) => setName(e.currentTarget.value)}
            required
            maxLength={100}
          />
          <GenreMultiSelect
            label={t('guide.view.genres')}
            keep={keep}
            value={genres}
            onChange={setGenres}
          />
          <GenreMultiSelect
            label={t('guide.view.excludeGenres')}
            keep={keep}
            value={excludeGenres}
            onChange={setExcludeGenres}
          />
          <NumberInput
            label={t('guide.view.minDuration')}
            value={minSeconds == null ? '' : Math.round(minSeconds / 60)}
            onChange={(value) => setMinSeconds(typeof value === 'number' ? value * 60 : null)}
            min={1}
            max={24 * 60}
            allowDecimal={false}
            suffix={t('guide.view.minutes')}
          />
          <MultiSelect
            label={t('guide.view.services')}
            description={t('guide.view.servicesDescription')}
            data={services.map((service) => ({
              value: service.id,
              // 同じ名前のサービスが、別のチャンネル種別や、同じ放送局の別のサービスにもあるので、種別とサービス ID を添える。
              label: `${service.name}（${service.channelType.displayName}・${service.serviceId}）`,
            }))}
            renderOption={({ option }) => (
              <ServiceLabel
                name={option.label}
                logoUrl={services.find((service) => service.id === option.value)?.logoUrl}
              />
            )}
            value={serviceOrder}
            onChange={setServiceOrder}
            searchable
            clearable
          />
          <Switch
            label={t('guide.view.hideEmpty')}
            checked={hideEmpty}
            onChange={(e) => setHideEmpty(e.currentTarget.checked)}
          />
          {error !== null && (
            <Alert color="red" role="alert">
              {error}
            </Alert>
          )}
          <Group justify="space-between">
            {view.id !== undefined ? (
              <Button
                type="button"
                color="red"
                variant="subtle"
                onClick={deleteView}
                loading={removing}
              >
                {t('guide.view.delete')}
              </Button>
            ) : (
              <span />
            )}
            <Button type="submit" loading={creating || updating} disabled={name.trim() === ''}>
              {t('guide.view.save')}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
