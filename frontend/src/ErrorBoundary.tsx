import { Component, type ReactNode } from "react";
import { t } from "./i18n.ts";

// A bug in one view must not leave a blank window with an approval waiting behind it.
// The error's own words are shown as given, under the app's.
export default class ErrorBoundary extends Component<{ children: ReactNode }, { message: string | null }> {
  state = { message: null as string | null };

  static getDerivedStateFromError(err: unknown) {
    return { message: err instanceof Error ? err.message : String(err) };
  }

  render() {
    if (this.state.message === null) return this.props.children;
    return (
      <div className="empty" role="alert">
        <p>{t("app.error.drawing")}</p>
        <pre>{this.state.message}</pre>
        <button onClick={() => window.location.reload()}>{t("app.error.reload")}</button>
      </div>
    );
  }
}
