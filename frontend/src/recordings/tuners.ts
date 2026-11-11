// SegmentTuner は、セグメントを書いたストリームを受信していたチューナー (#1467)。管理者にだけ返る。
export type SegmentTuner = {
  readonly index: number;
  readonly name: string;
};

// distinctTunerNames は、セグメントのチューナーの名前を、始まりの順に並べる。続けて同じチューナーで録ったセグメントは 1 つにまとめ、
// 別のチューナーに移って戻ったときは、移った順が分かるよう、もう一度並べる。見つけられなかったセグメントは飛ばす。
export function distinctTunerNames(tuners: readonly (SegmentTuner | null | undefined)[]): string[] {
  const names: string[] = [];
  for (const tuner of tuners) {
    if (tuner == null) {
      continue;
    }
    if (names.at(-1) !== tuner.name) {
      names.push(tuner.name);
    }
  }

  return names;
}
