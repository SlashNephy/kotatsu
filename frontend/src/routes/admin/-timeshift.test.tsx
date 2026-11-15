import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './timeshift';

afterEach(cleanup);

// timeshiftResponse は、タイムシフトの画面の応答を作る。enabled は、視聴している間だけ録るかどうか。
function timeshiftResponse(enabled: boolean) {
  return {
    data: {
      services: [],
      timeshiftAvailable: true,
      systemSettings: {
        timeshiftWhileWatching: enabled,
        timeshiftWhileWatchingRetentionSeconds: 3 * 3600,
      },
      timeshiftChannels: [],
    },
  };
}

describe('視聴している間だけ録るかは、変えたらすぐ保存する (#1740)', () => {
  test('切り替えだけを送り、書きかけの保持時間は送らずに残す', async () => {
    let enabled = false;
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'timeshiftWhileWatchingMutation') {
        sent.push(variables.input);
        enabled = (variables.input as { timeshiftWhileWatching: boolean }).timeshiftWhileWatching;

        return {
          data: {
            updateSystemSettings: {
              timeshiftWhileWatching: enabled,
              timeshiftWhileWatchingRetentionSeconds: 3 * 3600,
            },
          },
        };
      }

      return timeshiftResponse(enabled);
    });
    await renderRoute(Route, '/admin/timeshift', '/admin/timeshift', environment);
    const toggle = await screen.findByRole('switch', { name: /^ライブ視聴している間だけ録る/ });
    // 保持時間の欄は、サービスを足す欄にもあるので、後ろの、視聴している間の欄を使う。
    const watchingHours = () => screen.getAllByRole('textbox', { name: '保持時間（時間）' }).at(-1);
    const hours = watchingHours();
    if (hours === undefined) {
      throw new Error('no retention field');
    }
    fireEvent.change(hours, { target: { value: '5' } });

    await act(async () => fireEvent.click(toggle));
    await screen.findByRole('status');

    expect(sent).toEqual([{ timeshiftWhileWatching: true }]);
    expect(screen.getByRole('switch', { name: /^ライブ視聴している間だけ録る/ })).toHaveProperty(
      'checked',
      true,
    );
    expect(watchingHours()).toHaveProperty('value', '5');
  });

  test('保存に失敗したら、変える前に戻す', async () => {
    const environment = createTestEnvironment((request) =>
      request.name === 'timeshiftWhileWatchingMutation'
        ? new Error('unavailable')
        : timeshiftResponse(false),
    );
    await renderRoute(Route, '/admin/timeshift', '/admin/timeshift', environment);
    const toggle = await screen.findByRole('switch', { name: /^ライブ視聴している間だけ録る/ });

    await act(async () => fireEvent.click(toggle));
    await screen.findByRole('alert');

    expect(screen.getByRole('switch', { name: /^ライブ視聴している間だけ録る/ })).toHaveProperty(
      'checked',
      false,
    );
  });
});
