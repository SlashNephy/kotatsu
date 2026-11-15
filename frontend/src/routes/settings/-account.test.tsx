import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './account';

afterEach(cleanup);

// settings は、サーバーが持つ記録の設定。保存すると、送った値にする。
type Settings = {
  thresholdPercent: number;
  watchingOnFirst: boolean;
  watchingOnEpisode: boolean;
  watchedOnLast: boolean;
  notifyWishlist: boolean;
};

// serve は、記録の設定を返し、保存を受けたら送った値を覚えて返す Relay の環境と、送った値の一覧を作る。
// fails が true の間は、保存を断る。
function serve(fails = { value: false }) {
  let settings: Settings = {
    thresholdPercent: 80,
    watchingOnFirst: false,
    watchingOnEpisode: false,
    watchedOnLast: false,
    notifyWishlist: false,
  };
  const sent: Settings[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.name === 'accountSettingsMutation') {
      if (fails.value) {
        sent.push(variables.input as Settings);

        return new Error('unavailable');
      }
      settings = variables.input as Settings;
      sent.push(settings);

      return {
        data: { updateAnimeRecordSettings: { thresholdPercent: settings.thresholdPercent } },
      };
    }

    return {
      data: { animeRecordSettings: { enabled: true, ...settings, destinations: [] } },
    };
  });

  return { environment, sent };
}

describe('記録の設定を、変えたらすぐ保存する (#1740)', () => {
  test('保存のボタンを置かず、チェックを変えたら保存する', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    const first = await screen.findByRole('checkbox', {
      name: '第1話（[新]）を見たら「見てる」にする',
    });
    expect(screen.queryByRole('button', { name: '保存' })).toBeNull();

    await act(async () => fireEvent.click(first));

    expect(sent).toEqual([
      expect.objectContaining({ watchingOnFirst: true, thresholdPercent: 80 }),
    ]);
    expect(await screen.findByRole('status')).toHaveProperty('textContent', '保存しました');
  });

  test('割合は、打っている途中では保存せず、欄を離れたら保存する', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    const threshold = await screen.findByRole('textbox', { name: '見たとみなす割合' });

    fireEvent.change(threshold, { target: { value: '5' } });
    fireEvent.change(threshold, { target: { value: '50' } });
    expect(sent).toEqual([]);

    await act(async () => fireEvent.blur(threshold));

    expect(sent).toEqual([expect.objectContaining({ thresholdPercent: 50 })]);
  });

  test('割合が 1〜100 に収まらなければ保存しない', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    const threshold = await screen.findByRole('textbox', { name: '見たとみなす割合' });

    fireEvent.change(threshold, { target: { value: '' } });
    await act(async () => fireEvent.blur(threshold));

    expect(sent).toEqual([]);
    expect(screen.getByText('1〜100 の整数で入れてください')).toBeTruthy();
  });

  test('保存に失敗したら、チェックを戻し、割合はもう一度欄を離れれば保存し直す', async () => {
    const fails = { value: true };
    const { environment, sent } = serve(fails);
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    const first = await screen.findByRole('checkbox', {
      name: '第1話（[新]）を見たら「見てる」にする',
    });
    await act(async () => fireEvent.click(first));
    expect(first).toHaveProperty('checked', false);

    const threshold = screen.getByRole('textbox', { name: '見たとみなす割合' });
    fireEvent.change(threshold, { target: { value: '50' } });
    await act(async () => fireEvent.blur(threshold));
    fails.value = false;
    await act(async () => fireEvent.blur(threshold));

    expect(sent.map((input) => input.thresholdPercent)).toEqual([80, 50, 50]);
    expect(await screen.findByRole('status')).toHaveProperty('textContent', '保存しました');
  });

  test('保存の途中は、次の変更を受け付けない', async () => {
    const sent: unknown[] = [];
    // 保存には応えず、保存の途中のままにする。
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'accountSettingsMutation') {
        sent.push(variables.input);

        return undefined;
      }

      return {
        data: {
          animeRecordSettings: {
            enabled: true,
            thresholdPercent: 80,
            watchingOnFirst: false,
            watchingOnEpisode: false,
            watchedOnLast: false,
            notifyWishlist: false,
            destinations: [],
          },
        },
      };
    });
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    await act(async () =>
      fireEvent.click(
        await screen.findByRole('checkbox', { name: '第1話（[新]）を見たら「見てる」にする' }),
      ),
    );
    const last = screen.getByRole('checkbox', { name: /^最終話（\[終\]）を見たら/ });

    expect(last).toHaveProperty('disabled', true);
    await act(async () => fireEvent.click(last));
    expect(sent).toHaveLength(1);
  });
});
