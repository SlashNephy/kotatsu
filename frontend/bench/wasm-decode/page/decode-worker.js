// 測定の worker。デコードの計測 (decode.js) と、小さなループの計測 (loop.js) を、受け取ったメッセージに応じて行う。
import { measureDecode } from './decode.js';
import { measureLoop } from './loop.js';

onmessage = async ({ data }) => {
  postMessage(data.kind === 'loop' ? await measureLoop() : await measureDecode(data));
};
