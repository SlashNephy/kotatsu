import { Component, type ErrorInfo, type ReactNode } from 'react';

type Props = {
  fallback: (error: Error) => ReactNode;
  children: ReactNode;
};

type State = {
  error: Error | null;
};

// ErrorBoundary は、子の描画で起きたエラーを捕まえ、fallback を出す。
// Relay のクエリは、データを取得できなかったエラーを、描画中の例外として投げる。
export class ErrorBoundary extends Component<Props, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: unknown): State {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  }

  override componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(error, info.componentStack);
  }

  override render() {
    if (this.state.error !== null) {
      return this.props.fallback(this.state.error);
    }

    return this.props.children;
  }
}
