import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './account';

afterEach(cleanup);

// dramaRecordsDisabled は、TMDB との連携が無効なときの、ドラマの記録の問い合わせの応答 (#1649)。
const dramaRecordsDisabled = {
  data: { dramaRecordSettings: { enabled: false, destinations: [] } },
};

// settings は、サーバーが持つ記録の設定。保存すると、送った値にする。
type Settings = {
  thresholdPercent: number;
  watchingOnFirst: boolean;
  watchingOnEpisode: boolean;
  watchedOnLast: boolean;
  notifyWishlist: boolean;
};

// serve は、記録の設定を返し、保存を受けたら送った値を保持して返す Relay の環境と、送った値の一覧を作る。
// fails が true の間は、保存を拒否する。
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
    if (request.name === 'DramaRecordsQuery') {
      return dramaRecordsDisabled;
    }
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
    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
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

    fireEvent.change(threshold, { target: { value: '0' } });
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
    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
  });

  test('保存の途中に変えたら、前の保存を待ってから、変えた項目をまとめて送る', async () => {
    const sent: Settings[] = [];
    let release: () => void = () => {};
    const settings: Settings = {
      thresholdPercent: 80,
      watchingOnFirst: false,
      watchingOnEpisode: false,
      watchedOnLast: false,
      notifyWishlist: false,
    };
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'DramaRecordsQuery') {
        return dramaRecordsDisabled;
      }
      if (request.name === 'accountSettingsMutation') {
        sent.push(variables.input as Settings);
        const response = { data: { updateAnimeRecordSettings: { thresholdPercent: 80 } } };

        // 1 つ目の保存は、release を呼ぶまで応えない。
        return sent.length === 1
          ? new Promise((resolve) => {
              release = () => resolve(response);
            })
          : response;
      }

      return { data: { animeRecordSettings: { enabled: true, ...settings, destinations: [] } } };
    });
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    await act(async () =>
      fireEvent.click(
        await screen.findByRole('checkbox', { name: '第1話（[新]）を見たら「見てる」にする' }),
      ),
    );
    await act(async () =>
      fireEvent.click(screen.getByRole('checkbox', { name: /^最終話（\[終\]）を見たら/ })),
    );
    expect(sent).toHaveLength(1);

    await act(async () => release());
    await screen.findByRole('status');

    expect(sent).toEqual([
      expect.objectContaining({ watchingOnFirst: true, watchedOnLast: false }),
      expect.objectContaining({ watchingOnFirst: true, watchedOnLast: true }),
    ]);
  });

  test('この端末で記録するかを変えても、同じ通知で伝える', async () => {
    const { environment } = serve();
    await renderRoute(Route, '/settings/account', '/settings/account', environment);

    await act(async () =>
      fireEvent.click(
        await screen.findByRole('switch', { name: /^この端末で見たアニメを記録する/ }),
      ),
    );

    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
  });
});

// serveDrama は、Simkl と連携したユーザーの、ドラマの記録の問い合わせに応え、取り込み直しの依頼を保持する Relay の環境を作る (#1649)。
function serveDrama() {
  const imports: unknown[] = [];
  const saved: unknown[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.name === 'DramaRecordsQuery') {
      return {
        data: {
          dramaRecordSettings: {
            enabled: true,
            thresholdPercent: 20,
            destinations: [
              {
                destination: 'SIMKL',
                available: true,
                connection: {
                  accountName: 'simkl-user',
                  connectedAt: '2026-10-10T03:00:00Z',
                  reauthorizationRequired: false,
                  watchlistImportedAt: null,
                  watchlistError: '/sync/activities returned 503',
                },
              },
            ],
          },
        },
      };
    }
    if (request.name === 'DramaRecordsSettingsMutation') {
      saved.push(variables);

      return { data: { updateDramaRecordSettings: { thresholdPercent: 20 } } };
    }
    if (request.name === 'DramaRecordsImportMutation') {
      imports.push(variables);

      return { data: { importDramaWatchlist: true } };
    }

    return {
      data: {
        animeRecordSettings: {
          enabled: true,
          thresholdPercent: 20,
          watchingOnFirst: false,
          watchingOnEpisode: false,
          watchedOnLast: false,
          notifyWishlist: false,
          destinations: [],
        },
      },
    };
  });

  return { environment, imports, saved };
}

describe('見たドラマを記録する Simkl との連携 (#1649)', () => {
  test('連携したアカウントと取り込みの様子を出し、取り込み直しを頼める', async () => {
    const { environment, imports } = serveDrama();
    await renderRoute(Route, '/settings/account', '/settings/account', environment);

    expect(await screen.findByText(/simkl-user として連携しています/)).toBeTruthy();
    expect(screen.getByText('ウォッチリストはまだ取り込んでいません')).toBeTruthy();
    expect(screen.getByText('取り込めませんでした（/sync/activities returned 503）')).toBeTruthy();

    await act(async () => fireEvent.click(screen.getByRole('button', { name: '取り込み直す' })));

    expect(imports).toHaveLength(1);
    expect(
      await screen.findByText('取り込みを始めました。終わったら、ドラマの視聴管理の画面に出ます'),
    ).toBeTruthy();
  });

  test('Simkl から戻ってきたときの失敗は、ドラマの記録の欄にだけ出す', async () => {
    const { environment } = serveDrama();
    await renderRoute(
      Route,
      '/settings/account',
      '/settings/account?error=drama_record_mismatch',
      environment,
    );

    const alerts = await screen.findAllByRole('alert');
    expect(alerts.map((alert) => alert.textContent)).toEqual([
      expect.stringContaining('この端末で始めた連携ではないので'),
    ]);
  });

  test('Simkl と連携して戻ってきたら、連携したことを出す', async () => {
    const { environment } = serveDrama();
    await renderRoute(Route, '/settings/account', '/settings/account?connected=simkl', environment);

    expect(await screen.findByText('Simklと連携しました')).toBeTruthy();
  });

  test('見たドラマの割合は、アニメの割合と別の欄で、欄を離れたら保存する', async () => {
    const { environment, saved } = serveDrama();
    await renderRoute(Route, '/settings/account', '/settings/account', environment);
    const thresholds = await screen.findAllByRole('textbox', { name: '見たとみなす割合' });
    // アニメの欄とドラマの欄に、1 つずつある。後ろがドラマの欄。
    expect(thresholds).toHaveLength(2);

    fireEvent.change(thresholds[1]!, { target: { value: '40' } });
    await act(async () => fireEvent.blur(thresholds[1]!));

    expect(saved).toEqual([{ input: { thresholdPercent: 40 } }]);
    expect(screen.getByRole('switch', { name: /^この端末で見たドラマを記録する/ })).toBeTruthy();
    expect(screen.getByRole('switch', { name: /^この端末で見たアニメを記録する/ })).toBeTruthy();
  });
});
