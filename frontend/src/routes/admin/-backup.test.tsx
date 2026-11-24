import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import type { Variables } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './backup';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const keyA = 'age1000aznq0kvdaya52s5v2fe55y3sy2m4m908tuvj24h99mdvc952s699zfj';
const keyB = 'age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p';

type Run = {
  manual: boolean;
  startedAt: string;
  finishedAt: string;
  fileName: string | null;
  size: number | null;
  error: string | null;
};

// serve は、サーバーのバックアップの設定を持ち、保存されたら送った項目を当てる環境を作る。sent は、送った mutation の変数。
// frozen なら、保存しても、読み直した設定を変えない (読み直しが保存より遅れて届く場面)。
function serve(recipients: string[], latestRun: Run | null, frozen = false) {
  let settings = { recipients, intervalHours: 24, generations: 7 };
  // replace は、次に読み直したときの公開鍵の一覧を、ほかの管理者が変えたものにする。
  const replace = (next: string[]) => {
    settings = { ...settings, recipients: next };
  };
  const sent: { name: string; variables: Variables }[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.name === 'backupMutation' || request.name === 'backupRunMutation') {
      sent.push({ name: request.name, variables });
      if (!frozen) {
        settings = { ...settings, ...(variables.input as Partial<typeof settings>) };
      }
      const field = request.name === 'backupMutation' ? 'updateBackupSettings' : 'runBackup';

      return { data: { [field]: { settings: { updatedAt: '2026-11-17T00:00:01Z' } } } };
    }

    return {
      data: {
        backup: { settings: { ...settings, updatedAt: '2026-11-17T00:00:00Z' }, latestRun },
      },
    };
  });

  return { environment, sent, replace };
}

describe('バックアップの設定 (#1926)', () => {
  test('最後のバックアップが失敗していれば、理由を出す', async () => {
    const { environment } = serve([keyA], {
      manual: false,
      startedAt: '2026-11-17T03:00:00Z',
      finishedAt: '2026-11-17T03:00:01Z',
      fileName: null,
      size: null,
      error: 'the backup directory (backup.dir) is not configured on nekonote',
    });
    await renderRoute(Route, '/admin/backup', '/admin/backup', environment);

    expect(await screen.findByText(/バックアップできませんでした（定期）/)).toBeDefined();
    expect(screen.getByText(/backup\.dir\) is not configured/)).toBeDefined();
  });

  test('公開鍵がなければ、今すぐバックアップするボタンを押せない', async () => {
    const { environment } = serve([], null);
    await renderRoute(Route, '/admin/backup', '/admin/backup', environment);

    expect(await screen.findByText('まだバックアップしていません')).toBeDefined();
    const run = screen.getByRole('button', { name: '今すぐバックアップする' });
    expect(run.hasAttribute('disabled') || run.getAttribute('data-disabled') === 'true').toBe(true);
  });

  test('公開鍵を足すと消すは、押したらすぐ、鍵の一覧を保存する', async () => {
    const { environment, sent } = serve([keyA], null);
    await renderRoute(Route, '/admin/backup', '/admin/backup', environment);
    await screen.findByText(keyA);

    const input = screen.getByRole('textbox', { name: '公開鍵を足す' });
    fireEvent.change(input, { target: { value: 'AGE-SECRET-KEY-1ABC' } });
    expect(screen.getByText('age1で始まる公開鍵を入れてください')).toBeDefined();
    fireEvent.change(input, { target: { value: keyB } });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '足す' })));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]!.variables).toEqual({ input: { recipients: [keyA, keyB] } });

    await act(async () => fireEvent.click(screen.getAllByRole('button', { name: '消す' })[0]!));
    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1]!.variables).toEqual({ input: { recipients: [keyB] } });
  });

  test('今すぐバックアップすると、頼んだことを出す', async () => {
    const { environment, sent } = serve([keyA], null);
    await renderRoute(Route, '/admin/backup', '/admin/backup', environment);

    await act(async () =>
      fireEvent.click(await screen.findByRole('button', { name: '今すぐバックアップする' })),
    );
    await waitFor(() => expect(sent.map((s) => s.name)).toEqual(['backupRunMutation']));
    expect(await screen.findByText(/バックアップを頼みました/)).toBeDefined();
  });

  test('保存した公開鍵の一覧を読み直すまで、続けて消せない', async () => {
    const { environment, sent } = serve([keyA, keyB], null, true);
    await renderRoute(Route, '/admin/backup', '/admin/backup', environment);
    await screen.findByText(keyA);

    await act(async () => fireEvent.click(screen.getAllByRole('button', { name: '消す' })[0]!));
    await waitFor(() => expect(sent).toHaveLength(1));
    // 古い一覧のまま keyB を消すと、keyA を戻してしまうので、押せない。
    const removes = screen.getAllByRole('button', { name: '消す' });
    expect(removes.every((button) => button.hasAttribute('disabled'))).toBe(true);
  });

  test('読み直した一覧が、送ったものと違っても、変わったら続けて操作できる', async () => {
    const { environment, sent, replace } = serve([keyA, keyB], null, true);
    await renderRoute(Route, '/admin/backup', '/admin/backup', environment);
    await screen.findByText(keyA);

    await act(async () => fireEvent.click(screen.getAllByRole('button', { name: '消す' })[0]!));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(
      screen.getAllByRole('button', { name: '消す' }).every((b) => b.hasAttribute('disabled')),
    ).toBe(true);

    // ほかの管理者が、keyB だけの一覧にした。読み直したら、また消せる。
    replace([keyB]);
    const interval = screen.getByRole('textbox', { name: 'バックアップの間隔（時間）' });
    fireEvent.change(interval, { target: { value: '12' } });
    await act(async () => fireEvent.blur(interval));
    await waitFor(() => expect(screen.queryByText(keyA)).toBeNull());
    expect(screen.getByRole('button', { name: '消す' }).hasAttribute('disabled')).toBe(false);
  });
});
