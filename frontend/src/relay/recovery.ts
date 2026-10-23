import type { ErrorRecovery } from '../futon';

import { serverConnections } from './connection';
import { isTransientError } from './transient';

// errorRecovery は、ErrorBoundary に、サーバーが戻ったことを知らせる。
// subscription がつなぎ直せたときと、一時的な障害で送り直していた query が通ったときに、戻ったとみなす。
export const errorRecovery: ErrorRecovery = {
  recoveries: serverConnections.recoveries,
  subscribe: serverConnections.subscribe,
  isRecoverable: isTransientError,
};
