import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import i18n from '../i18n';
import { ConditionsFields, type ServiceOption } from './ConditionsFields';
import { emptyConditions, type ConditionsForm } from './form';

afterEach(cleanup);

const channelTypes = [
  { type: 'GR', displayName: '地上波', hasServices: true },
  { type: 'GONE', displayName: '消えた種別', hasServices: false },
];

const services: ServiceOption[] = [
  { id: 'live', name: '残る局', logoUrl: null, serviceId: 1, channelType: { displayName: '地上波' } },
  {
    id: 'gone',
    name: '消えた局',
    logoUrl: null,
    serviceId: 2,
    channelType: { displayName: '消えた種別' },
    gone: true,
  },
];

// optionsOf は、label の欄を開き、出した選択肢の名前を返す。
function optionsOf(label: string): string[] {
  const input = screen
    .getAllByLabelText(label)
    .find((element) => element instanceof HTMLInputElement && element.type !== 'hidden');
  if (input === undefined) {
    throw new Error(`no input for ${label}`);
  }
  fireEvent.click(input);
  const listbox = document.getElementById(input.getAttribute('aria-controls') ?? '');
  if (listbox === null) {
    throw new Error(`no listbox for ${label}`);
  }

  return [...listbox.querySelectorAll('[role=option]')].map((option) => option.textContent);
}

function renderFields(patch: Partial<ConditionsForm>) {
  render(
    <MantineProvider env="test">
      <ConditionsFields
        value={{ ...emptyConditions(), ...patch }}
        onChange={() => {}}
        channelTypes={channelTypes}
        services={services}
      />
    </MantineProvider>,
  );
}

// 上流から消えたサービスと、消えていないサービスを持たない種別は、選んでいるときだけ選択肢に残す (#1973)。
describe('ConditionsFields', () => {
  test.each<{
    name: string;
    patch: Partial<ConditionsForm>;
    wantTypes: string[];
    wantServices: string[];
    wantExcluded: string[];
  }>([
    {
      name: '選んでいなければ、消えた種別とサービスを出さない',
      patch: {},
      wantTypes: ['地上波'],
      wantServices: ['残る局（地上波・1）'],
      wantExcluded: ['残る局（地上波・1）'],
    },
    {
      name: '選んでいれば、印を付けて、選んだ欄にだけ出す',
      patch: { channelTypes: ['GONE'], services: ['gone'] },
      wantTypes: ['地上波', '消えた種別'],
      wantServices: [
        '残る局（地上波・1）',
        i18n.t('rules.goneService', { name: '消えた局（消えた種別・2）' }),
      ],
      wantExcluded: ['残る局（地上波・1）'],
    },
  ])('$name', ({ patch, wantTypes, wantServices, wantExcluded }) => {
    renderFields(patch);

    expect(optionsOf(i18n.t('rules.channelTypes'))).toEqual(wantTypes);
    expect(optionsOf(i18n.t('rules.services'))).toEqual(wantServices);
    expect(optionsOf(i18n.t('rules.excludeServices'))).toEqual(wantExcluded);
  });
});
