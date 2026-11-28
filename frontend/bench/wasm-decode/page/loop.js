// WASM と JavaScript の小さなループの速さを計測する (#2162)。ブラウザが WASM を最適化したコードで動かしているかを見る。
// 同じ計算 (s = s + (i * i ^ (s >>> 3)) を n 回) を、手で組んだ WASM と JavaScript で回し、それぞれの時間を返す。
// WASM だけが遅ければ WASM の実行の段階を、どちらも遅ければ JIT そのものを疑う。

// 次の WAT を手で組んだもの。
// (func (export "run") (param $n i32) (result i32) (local $i i32) (local $s i32)
//   (block (loop (br_if 1 (i32.ge_u (local.get $i) (local.get $n)))
//     (local.set $s (i32.add (local.get $s) (i32.xor (i32.mul (local.get $i) (local.get $i)) (i32.shr_u (local.get $s) (i32.const 3)))))
//     (local.set $i (i32.add (local.get $i) (i32.const 1)))
//     (br 0)))
//   (local.get $s))
const loopWasm = new Uint8Array([
  0, 97, 115, 109, 1, 0, 0, 0, 1, 6, 1, 96, 1, 127, 1, 127, 3, 2, 1, 0, 7, 7, 1, 3, 114, 117, 110,
  0, 0, 10, 46, 1, 44, 1, 2, 127, 2, 64, 3, 64, 32, 1, 32, 0, 79, 13, 1, 32, 2, 32, 1, 32, 1, 108,
  32, 2, 65, 3, 118, 115, 106, 33, 2, 32, 1, 65, 1, 106, 33, 1, 12, 0, 11, 11, 32, 2, 11,
]);

const loopJs = (n) => {
  let s = 0;
  for (let i = 0; i < n; i++) s = (s + (Math.imul(i, i) ^ (s >>> 3))) | 0;
  return s;
};

// iterations は 1 回のループの回数。速い端末で 0.1〜0.3 秒になる長さにする。
const iterations = 100_000_000;
const repeats = 5;

// measureLoop は、WASM と JavaScript のループを交互に repeats 回ずつ回し、それぞれの時間 (ms) を返す。
// 最初の回は最適化の前の段階で動くことがあるので、結果には全部の回を入れ、読むときは後ろの回を見る。
export const measureLoop = async () => {
  try {
    const { instance } = await WebAssembly.instantiate(loopWasm);
    const run = instance.exports.run;
    if (run(1000) !== loopJs(1000)) throw new Error('the WASM loop and the JS loop disagree');
    const wasmMs = [];
    const jsMs = [];
    for (let i = 0; i < repeats; i++) {
      let t = performance.now();
      run(iterations);
      wasmMs.push(performance.now() - t);
      t = performance.now();
      loopJs(iterations);
      jsMs.push(performance.now() - t);
    }
    return {
      ok: true,
      kind: 'loop',
      iterations,
      wasmMs,
      jsMs,
      thread: self.document ? 'main' : 'worker',
    };
  } catch (error) {
    return { ok: false, kind: 'loop', error: String(error?.stack ?? error) };
  }
};
