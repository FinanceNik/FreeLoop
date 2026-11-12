import { Component, type ErrorInfo, type ReactNode } from "react";
import { EngineService } from "./api.ts";
import { errorMessage, failureReport } from "./failure.ts";
import { copyText } from "./host.ts";
import { t } from "./i18n.ts";

interface Props {
  // A pane is one session (or the usage pane); a page is one screen of the window.
  kind: "pane" | "page";
  // The session whose problem report is wanted, if any.
  sessionId?: string;
  // Closes the pane or leaves the page.
  onClose?: () => void;
  // Told when the view failed, and with false when it is shown again.
  onFailed?: (failed: boolean) => void;
  children: ReactNode;
}

interface State {
  failure: { err: unknown; stack: string } | null;
  note: string;
}

// Each pane and each page has a boundary of its own (GUI-017): one view that fails to
// draw leaves the other panes, the sidebar and a waiting approval working. The window's
// boundary in main.tsx is the last resort.
export default class PaneBoundary extends Component<Props, State> {
  state: State = { failure: null, note: "" };
  private stack = "";

  static getDerivedStateFromError(err: unknown): Partial<State> {
    return { failure: { err, stack: "" }, note: "" };
  }

  componentDidCatch(err: unknown, info: ErrorInfo) {
    this.stack = info.componentStack ?? "";
    this.setState({ failure: { err, stack: this.stack } });
    this.props.onFailed?.(true);
  }

  componentWillUnmount() {
    if (this.state.failure) this.props.onFailed?.(false);
  }

  private copy = async () => {
    const failure = this.state.failure;
    if (!failure) return;
    let report = "";
    try {
      report = await EngineService.ProblemReport(this.props.sessionId ?? "");
    } catch (e) {
      this.setState({ note: t("pane.error.reportFailed", { error: errorMessage(e) }) });
    }
    try {
      await copyText(failureReport(report, failure.err, failure.stack));
    } catch (e) {
      this.setState({ note: t("pane.error.reportFailed", { error: errorMessage(e) }) });
    }
  };

  render() {
    const { failure, note } = this.state;
    if (!failure) return this.props.children;
    const { kind, onClose } = this.props;
    return (
      <div className="empty pane-failed" role="alert">
        <p>{t(kind === "pane" ? "pane.error.pane" : "pane.error.page")}</p>
        <pre>{errorMessage(failure.err)}</pre>
        {note && <p className="muted">{note}</p>}
        <div className="row">
          <button onClick={this.copy}>{t("settings.report.copy")}</button>
          {onClose && <button onClick={onClose}>{t(kind === "pane" ? "workspace.close" : "pane.error.closePage")}</button>}
        </div>
      </div>
    );
  }
}
