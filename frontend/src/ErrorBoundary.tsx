import { Component, type ReactNode } from "react";

// A bug in one view must not leave a blank window with an approval waiting behind it.
export default class ErrorBoundary extends Component<{ children: ReactNode }, { message: string | null }> {
  state = { message: null as string | null };

  static getDerivedStateFromError(err: unknown) {
    return { message: err instanceof Error ? err.message : String(err) };
  }

  render() {
    if (this.state.message === null) return this.props.children;
    return (
      <div className="empty" role="alert">
        <p>Something went wrong drawing this window. Your sessions and transcripts are safe.</p>
        <pre>{this.state.message}</pre>
        <button onClick={() => window.location.reload()}>Reload</button>
      </div>
    );
  }
}
