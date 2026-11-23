import type { DeinterlaceStats } from '@mpeg2toh264/yadif';
import {
  Alert,
  Button,
  Card,
  CopyButton,
  Modal,
  Progress,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { createBenchmarkStream } from './benchmarkStream';
import {
  baselineOf,
  type BenchmarkReport,
  type BenchmarkScenario,
  type BenchmarkScenarioID,
  benchmarkScenarios,
  type Counters,
  formatReport,
  judge,
  measureMs,
  median,
  type RunResult,
  runs,
  streamSeconds,
  summarizeRun,
  summarizeScenario,
  warmupMs,
} from './benchmarkResults';
import { measureEstimate } from './estimateBenchmark';
import { TsPlayer } from './TsPlayer';
import { useCloseWatch } from './WatchHost';
import type { PlayerMedia } from './playerMedia';

// loadTimeoutMs は、再生を始めるまで待つ時間。これを過ぎたら、その回は測れなかったものとする。
const loadTimeoutMs = 15_000;
// refreshFrames は、画面の書き換えの間隔を測るときに数える書き換えの数。
const refreshFrames = 120;

// Mounted は、測っている回に出しているプレイヤー。
type Mounted = { key: number; scenario: BenchmarkScenario; url: string };

// Collector は、測っている回で、プレイヤーから受け取った値。
type Collector = {
  stats: DeinterlaceStats | null;
  fieldRateDropped: boolean;
  film: boolean;
  // collecting は、測る窓の中かどうか。窓の中で受け取った fps だけを数える。
  collecting: boolean;
  // offscreen は、測る窓の中で、プレイヤーが画面から外れたことがあるか。
  // 画面の外の canvas は合成されないので、GPU が追いつかなくても、コマの数に表れない (#1771)。
  offscreen: boolean;
  fpsSamples: number[];
  loaded: (() => void) | null;
};

// sleep は、ms だけ待つ。signal で止めたら、すぐに投げる。
const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason);

      return;
    }
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal.reason);
    };
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    signal.addEventListener('abort', onAbort, { once: true });
  });

// measureRefreshInterval は、画面の書き換えの間隔の中央値 (ミリ秒) を測る。
function measureRefreshInterval(): Promise<number | null> {
  return new Promise((resolve) => {
    const times: number[] = [];
    const step = (time: number) => {
      times.push(time);
      if (times.length <= refreshFrames) {
        requestAnimationFrame(step);
      } else {
        resolve(median(times.slice(1).map((value, index) => value - times[index]!)));
      }
    };
    requestAnimationFrame(step);
  });
}

function rendererName(): string | null {
  const gl = document.createElement('canvas').getContext('webgl2');
  if (gl === null) {
    return null;
  }
  const info = gl.getExtension('WEBGL_debug_renderer_info');
  const name = gl.getParameter(
    info === null ? gl.RENDERER : info.UNMASKED_RENDERER_WEBGL,
  ) as string;
  gl.getExtension('WEBGL_lose_context')?.loseContext();

  return name;
}

// DeviceBenchmark は、「この端末で試す」のカード (#1771、設計書 7.5)。
// 合成の映像を、経路ごとに短く再生してコマが間に合うかを測り、結果の表と、報告に貼れるテキストを出す。
export function DeviceBenchmark() {
  const { t } = useTranslation();
  const [mounted, setMounted] = useState<Mounted | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [report, setReport] = useState<BenchmarkReport | null>(null);
  const [failed, setFailed] = useState(false);
  const mediaRef = useRef<PlayerMedia | null>(null);
  const collector = useRef<Collector>({
    stats: null,
    fieldRateDropped: false,
    film: false,
    collecting: false,
    offscreen: false,
    fpsSamples: [],
    loaded: null,
  });
  // visible は、プレイヤーの半分以上が画面に入っているか。
  const visible = useRef(false);
  const observer = useRef<IntersectionObserver | null>(null);
  // playerBox は、プレイヤーを入れる要素を受け取り、画面に入っているかを見張る。
  const playerBox = (element: HTMLDivElement | null) => {
    observer.current?.disconnect();
    observer.current = null;
    visible.current = false;
    if (element === null) {
      return;
    }
    observer.current = new IntersectionObserver(
      ([entry]) => {
        visible.current = (entry?.intersectionRatio ?? 0) >= 0.5;
        if (!visible.current && collector.current.collecting) {
          collector.current.offscreen = true;
        }
      },
      { threshold: [0, 0.5, 1] },
    );
    observer.current.observe(element);
  };
  const abort = useRef<AbortController | null>(null);
  const closeWatch = useCloseWatch();

  // 画面を離れたら、測るのをやめる。
  useEffect(() => () => abort.current?.abort(), []);

  const label = (id: BenchmarkScenarioID) => t(`settings.account.benchmark.scenarios.${id}`);

  const counters = (): Counters => ({
    time: performance.now(),
    late: collector.current.stats?.late ?? 0,
    dropped: mediaRef.current?.getVideoPlaybackQuality?.().droppedVideoFrames ?? 0,
  });

  // runOnce は、経路を 1 回測る。測れなければ null。
  const runOnce = async (scenario: BenchmarkScenario, key: number, signal: AbortSignal) => {
    const url = await createBenchmarkStream(scenario.clip, streamSeconds, signal);
    try {
      collector.current = {
        stats: null,
        fieldRateDropped: false,
        film: false,
        collecting: false,
        offscreen: false,
        fpsSamples: [],
        loaded: null,
      };
      const loaded = new Promise<void>((resolve) => {
        collector.current.loaded = resolve;
      });
      setMounted({ key, scenario, url });
      const started = await Promise.race([
        loaded.then(() => true),
        sleep(loadTimeoutMs, signal).then(() => false),
      ]);
      if (!started) {
        return null;
      }
      let hidden = document.hidden;
      const onVisibility = () => {
        hidden ||= document.hidden;
      };
      document.addEventListener('visibilitychange', onVisibility);
      try {
        await sleep(warmupMs, signal);
        const start = counters();
        collector.current.collecting = true;
        collector.current.offscreen = !visible.current;
        await sleep(measureMs, signal);
        collector.current.collecting = false;
        const end = counters();
        // タブを隠した回と、プレイヤーが画面から外れた回は、描くのを間引かれるので数えない。
        if (hidden || collector.current.offscreen) {
          return null;
        }

        return summarizeRun(
          start,
          end,
          collector.current.fpsSamples,
          collector.current.film,
          collector.current.fieldRateDropped,
        );
      } finally {
        document.removeEventListener('visibilitychange', onVisibility);
      }
    } finally {
      setMounted(null);
      URL.revokeObjectURL(url);
    }
  };

  const start = async () => {
    abort.current?.abort();
    // 小さなプレイヤーで再生を続けていると、GPU を取り合って結果が濁るので、閉じてから試す。
    closeWatch();
    const controller = new AbortController();
    abort.current = controller;
    setReport(null);
    setFailed(false);
    const total = benchmarkScenarios.length * runs;
    setProgress({ done: 0, total });
    try {
      const refreshInterval = await measureRefreshInterval();
      const results = new Map<BenchmarkScenarioID, (RunResult | null)[]>();
      let done = 0;
      // 経路を 1 回ずつ順に回すのを繰り返し、端末の温度や、ほかの負荷の変化が、特定の経路にだけ偏らないようにする。
      for (let run = 0; run < runs; run++) {
        for (const scenario of benchmarkScenarios) {
          const result = await runOnce(scenario, done, controller.signal);
          results.set(scenario.id, [...(results.get(scenario.id) ?? []), result]);
          done++;
          setProgress({ done, total });
        }
      }
      const estimate = await measureEstimate(controller.signal);
      controller.signal.throwIfAborted();
      setReport({
        userAgent: navigator.userAgent,
        renderer: rendererName(),
        refreshInterval,
        scenarios: benchmarkScenarios.map((scenario) =>
          summarizeScenario(scenario.id, results.get(scenario.id) ?? []),
        ),
        estimate,
      });
    } catch (error) {
      if (!controller.signal.aborted) {
        console.error('the device benchmark failed', error);
        setFailed(true);
      }
    } finally {
      if (abort.current === controller) {
        abort.current = null;
        setProgress(null);
      }
    }
  };

  const number = (value: number | null, digits: number) =>
    value === null ? '-' : value.toFixed(digits);
  const scenarioOf = (id: BenchmarkScenarioID | undefined) =>
    id === undefined ? undefined : report?.scenarios.find((scenario) => scenario.id === id);

  return (
    <Card withBorder>
      <Stack gap="xs" align="flex-start">
        <Title order={3} size="h5">
          {t('settings.account.benchmark.title')}
        </Title>
        <Text size="sm" c="dimmed">
          {t('settings.account.benchmark.help')}
        </Text>
        <Button
          variant="default"
          size="xs"
          onClick={() => void start()}
          loading={progress !== null}
        >
          {t('settings.account.benchmark.start')}
        </Button>
        {/* 測っている間は、プレイヤーをモーダルに出し、必ず画面に入れておく。画面の外では、描くのを間引かれる (#1771)。 */}
        <Modal
          opened={progress !== null}
          onClose={() => abort.current?.abort()}
          closeOnClickOutside={false}
          title={t('settings.account.benchmark.title')}
          size="lg"
        >
          {progress !== null && (
            <Stack gap="xs">
              <Progress
                value={(progress.done / progress.total) * 100}
                aria-label={t('settings.account.benchmark.progress')}
              />
              <Text size="xs" c="dimmed" role="status">
                {mounted === null
                  ? t('settings.account.benchmark.preparing', {
                      done: progress.done,
                      total: progress.total,
                    })
                  : t('settings.account.benchmark.running', {
                      scenario: label(mounted.scenario.id),
                      done: progress.done,
                      total: progress.total,
                    })}
              </Text>
              <div
                ref={playerBox}
                style={{ width: '100%', aspectRatio: '16 / 9', background: 'black' }}
              >
                {mounted !== null && (
                  <TsPlayer
                    key={mounted.key}
                    url={mounted.url}
                    mediaRef={mediaRef}
                    captions={false}
                    settings={{
                      fieldRate: mounted.scenario.fieldRate,
                      interpolate: mounted.scenario.interpolate,
                      // 補間したコマは、利用者が決めた上限によらず、画面の書き換えごとに出し、書き換えの速さと比べる。
                      interpolationRate: null,
                      // 拡大 (#1654) は、測る経路の違いに混ぜないよう、利用者の設定によらず切る。
                      superResolution: false,
                      // 擬似 HDR (#1656) も、同じく切る。
                      hdrOutput: false,
                    }}
                    onMedia={(media) => {
                      if (media !== null) {
                        // 合成の映像に音はないが、念のため消音して流す。
                        media.muted = true;
                        media.volume = 0;
                      }
                    }}
                    onLoaded={() => {
                      void mediaRef.current?.play().catch(() => undefined);
                      collector.current.loaded?.();
                    }}
                    onDeinterlaceStats={(stats, fieldRateDropped) => {
                      const current = collector.current;
                      current.stats = stats;
                      current.fieldRateDropped ||= fieldRateDropped;
                      if (current.collecting) {
                        current.fpsSamples.push(stats.fps);
                        current.film ||= stats.film;
                      }
                    }}
                  />
                )}
              </div>
              <Button variant="default" size="xs" onClick={() => abort.current?.abort()}>
                {t('settings.account.benchmark.cancel')}
              </Button>
            </Stack>
          )}
        </Modal>
        {failed && <Alert color="amber">{t('settings.account.benchmark.failed')}</Alert>}
        {report !== null && (
          <Stack gap="xs" w="100%">
            <Table withTableBorder fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>{t('settings.account.benchmark.scenario')}</Table.Th>
                  <Table.Th>{t('settings.account.benchmark.fps')}</Table.Th>
                  <Table.Th>{t('settings.account.benchmark.late')}</Table.Th>
                  <Table.Th>{t('settings.account.benchmark.verdict')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {report.scenarios.map((scenario) => {
                  const baseline = scenarioOf(baselineOf[scenario.id]);
                  const smooth = judge(scenario, baseline, report.refreshInterval);

                  return (
                    <Table.Tr key={scenario.id}>
                      <Table.Td>{label(scenario.id)}</Table.Td>
                      <Table.Td>{number(scenario.fps, 1)}</Table.Td>
                      <Table.Td>
                        {number(scenario.latePerSecond, 1)}
                        {baseline?.latePerSecond != null && scenario.latePerSecond !== null && (
                          <Text span size="xs" c="dimmed">
                            {' '}
                            {t('settings.account.benchmark.lateDifference', {
                              difference: (scenario.latePerSecond - baseline.latePerSecond).toFixed(
                                1,
                              ),
                            })}
                          </Text>
                        )}
                      </Table.Td>
                      <Table.Td>
                        {smooth === null
                          ? '-'
                          : smooth
                            ? t('settings.account.benchmark.smooth')
                            : t('settings.account.benchmark.notSmooth')}
                      </Table.Td>
                    </Table.Tr>
                  );
                })}
              </Table.Tbody>
            </Table>
            <Text size="xs" c="dimmed">
              {t('settings.account.benchmark.summary', {
                refresh: number(report.refreshInterval, 1),
                estimate:
                  report.estimate === null
                    ? '-'
                    : t(`settings.account.benchmark.estimate.${report.estimate.method}`, {
                        ms: report.estimate.ms.toFixed(1),
                      }),
              })}
            </Text>
            <CopyButton value={formatReport(report, label)}>
              {({ copied, copy }) => (
                <Button variant="default" size="xs" onClick={copy}>
                  {copied
                    ? t('settings.account.benchmark.copied')
                    : t('settings.account.benchmark.copy')}
                </Button>
              )}
            </CopyButton>
          </Stack>
        )}
      </Stack>
    </Card>
  );
}
