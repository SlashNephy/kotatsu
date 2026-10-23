import { Component, createContext, type ContextType, type ErrorInfo, type ReactNode } from 'react';

// ErrorRecovery は、時間をおけば通る見込みのある失敗から、サーバーが戻ったことを ErrorBoundary に知らせる。
// futon は Relay に依存しないので、アプリが ErrorRecoveryContext で渡す。
export type ErrorRecovery = {
  // recoveries は、サーバーが戻った回数。増えたら戻ったとみなす。
  recoveries: () => number;
  subscribe: (listener: () => void) => () => void;
  // isRecoverable は、サーバーが戻れば通る見込みのある失敗かどうかを返す。
  isRecoverable: (error: Error) => boolean;
};

export const ErrorRecoveryContext = createContext<ErrorRecovery | null>(null);

// ErrorBoundaryAttemptContext は、いちばん近い ErrorBoundary が、エラーから戻した回数。
// データの読み込みは、これを読み込みのキーに混ぜ、戻したときに読み直す。
// Relay は、失敗したクエリのエラーをしばらく覚えていて、描き直すだけでは送り直さないため。
export const ErrorBoundaryAttemptContext = createContext(0);

type Props = {
  fallback: (error: Error) => ReactNode;
  children: ReactNode;
};

type State = {
  error: Error | null;
  attempt: number;
};

// ErrorBoundary は、子の描画で起きたエラーを捕まえ、fallback を出す。
// Relay のクエリは、データを取得できなかったエラーを、描画中の例外として投げる。
// サーバーの入れ替わりのように時間をおけば通る失敗なら、サーバーが戻ったのを待って、自動で子を描き直す。
export class ErrorBoundary extends Component<Props, State> {
  static override contextType = ErrorRecoveryContext;
  declare context: ContextType<typeof ErrorRecoveryContext>;

  override state: State = { error: null, attempt: 0 };

  // unsubscribe は、サーバーが戻るのを待っている間だけ持つ。
  private unsubscribe: (() => void) | undefined;

  static getDerivedStateFromError(error: unknown): Partial<State> {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  }

  override componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(error, info.componentStack);
    const recovery = this.context;
    if (recovery === null || this.unsubscribe !== undefined || !recovery.isRecoverable(error)) {
      return;
    }
    // 失敗した時点の回数から増えたら、失敗した後にサーバーが戻ったとみなす。
    const failedAt = recovery.recoveries();
    this.unsubscribe = recovery.subscribe(() => {
      if (recovery.recoveries() === failedAt) {
        return;
      }
      this.stopWaiting();
      this.setState((state) => ({ error: null, attempt: state.attempt + 1 }));
    });
  }

  override componentWillUnmount() {
    this.stopWaiting();
  }

  private stopWaiting() {
    this.unsubscribe?.();
    this.unsubscribe = undefined;
  }

  override render() {
    if (this.state.error !== null) {
      return this.props.fallback(this.state.error);
    }

    return (
      <ErrorBoundaryAttemptContext value={this.state.attempt}>
        {this.props.children}
      </ErrorBoundaryAttemptContext>
    );
  }
}
