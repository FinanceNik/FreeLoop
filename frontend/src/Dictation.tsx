import { useEffect, useEffectEvent, useRef, useState } from "react";
import { EngineService, type VoiceStatus } from "./api.ts";
import { language, locale, t } from "./i18n.ts";
import { MicIcon, StopIcon } from "./icons.tsx";
import { ariaKeys, keyLabel, os } from "./platform.ts";
import { bindingOf, pressed } from "./shortcuts.ts";
import { base64, clock, downsample, encodeWav, level, MAX_SECONDS, SPEECH_RATE, trimSilence } from "./voice.ts";

// Voice input in the message box (GUI-050). The button records from the microphone;
// the engine's speech model, run on this computer, turns the recording into text that
// goes in at the caret. Nothing is sent: the user reads it first. The first time, the
// model has to be downloaded, which the user starts here or in Settings.

// A recording under way.
interface Capture {
  stop(): { samples: Float32Array; rate: number };
  cancel(): void;
}

// startCapture records from the default microphone. The audio context is made at
// once, inside the click that asked for it: made later, WebKit keeps it silent.
// scripted, for `make check-app`, records a tone instead, as no microphone is there.
function startCapture(scripted: boolean, onLevel: (n: number) => void): Promise<Capture> {
  const blocks: Float32Array[] = [];
  if (scripted) {
    let n = 0;
    const timer = setInterval(() => {
      const b = Float32Array.from({ length: SPEECH_RATE / 10 }, (_, i) => 0.3 * Math.sin((2 * Math.PI * 220 * (n + i)) / SPEECH_RATE));
      n += b.length;
      blocks.push(b);
      onLevel(level(b));
    }, 100);
    const end = () => clearInterval(timer);
    return Promise.resolve({ stop: () => (end(), { samples: join(blocks), rate: SPEECH_RATE }), cancel: end });
  }
  const ctx = new AudioContext();
  const resumed = ctx.resume();
  let stream: MediaStream | null = null;
  const close = () => {
    stream?.getTracks().forEach((track) => track.stop());
    ctx.close().catch(() => {});
  };
  return (async () => {
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true } });
      await resumed;
      await ctx.audioWorklet.addModule("/capture-worklet.js");
      const node = new AudioWorkletNode(ctx, "capture");
      node.port.onmessage = (e: MessageEvent<Float32Array>) => {
        blocks.push(e.data);
        onLevel(level(e.data));
      };
      ctx.createMediaStreamSource(stream).connect(node);
    } catch (err) {
      close();
      throw err;
    }
    return { stop: () => (close(), { samples: join(blocks), rate: ctx.sampleRate }), cancel: close };
  })();
}

function join(blocks: Float32Array[]): Float32Array {
  const out = new Float32Array(blocks.reduce((n, b) => n + b.length, 0));
  let at = 0;
  for (const b of blocks) {
    out.set(b, at);
    at += b.length;
  }
  return out;
}

// microphoneProblem says in the user's words why the microphone could not be opened.
function microphoneProblem(err: unknown): string {
  const name = (err as { name?: string })?.name ?? "";
  if (name === "NotAllowedError" || name === "SecurityError") return t(os === "mac" ? "voice.error.denied.mac" : os === "windows" ? "voice.error.denied.windows" : "voice.error.denied.linux");
  if (name === "NotFoundError" || name === "OverconstrainedError") return t("voice.error.noMicrophone");
  if (name === "NotReadableError") return t(os === "windows" ? "voice.error.busy.windows" : "voice.error.busy");
  return t("voice.error.microphone", { error: (err as Error)?.message ?? String(err) });
}

// canDictate: the page can reach a microphone. A browser on plain http, in server
// mode, cannot.
export const canDictate = () => typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia && typeof AudioWorkletNode !== "undefined";

type Phase = "idle" | "starting" | "recording" | "transcribing";

interface Props {
  // The pane is the one in use: its shortcut and Escape act.
  active: boolean;
  // said goes into the message box at the caret.
  onText: (said: string) => void;
  onAnnounce: (text: string) => void;
}

export default function DictateButton({ active, onText, onAnnounce }: Props) {
  const [status, setStatus] = useState<VoiceStatus | null>(null);
  const [phase, setPhase] = useState<Phase>("idle");
  const [seconds, setSeconds] = useState(0);
  const [meter, setMeter] = useState(0);
  const [note, setNote] = useState("");
  const [asking, setAsking] = useState(false);
  const run = useRef<{ id: string; capture: Capture | null; started: number; token: number } | null>(null);
  const token = useRef(0);

  const refresh = () =>
    EngineService.VoiceStatus()
      .then(setStatus)
      .catch(() => setStatus(null));
  useEffect(() => {
    refresh();
  }, []);

  const end = () => {
    run.current = null;
    setPhase("idle");
    setMeter(0);
  };
  const cancel = () => {
    const r = run.current;
    if (!r) return;
    token.current++;
    r.capture?.cancel();
    EngineService.CancelDictation(r.id).catch(() => {});
    end();
    onAnnounce(t("voice.announce.cancelled"));
  };
  const start = () => {
    if (!status || status.state !== "ready") {
      refresh();
      setAsking(true);
      return;
    }
    setNote("");
    setPhase("starting");
    const mine = ++token.current;
    // Both at once: the model loads while the microphone opens and the user speaks.
    const capture = startCapture(status.scripted, setMeter);
    const id = EngineService.StartDictation(language());
    Promise.all([id, capture]).then(
      ([id, capture]) => {
        if (token.current !== mine) {
          capture.cancel();
          EngineService.CancelDictation(id).catch(() => {});
          return;
        }
        run.current = { id, capture, started: Date.now(), token: mine };
        setSeconds(0);
        setPhase("recording");
        onAnnounce(t("voice.announce.recording"));
      },
      (err) => {
        capture.then((c) => c.cancel()).catch(() => {});
        id.then((i) => EngineService.CancelDictation(i)).catch(() => {});
        if (token.current !== mine) return;
        end();
        // A refused microphone is the capture's failure; anything else is the engine's.
        const said = (err as { name?: string })?.name ? microphoneProblem(err) : String((err as Error)?.message ?? err);
        setNote(said);
        onAnnounce(said);
      },
    );
  };
  const finish = () => {
    const r = run.current;
    if (!r?.capture) return;
    const { samples, rate } = r.capture.stop();
    r.capture = null;
    setPhase("transcribing");
    setMeter(0);
    onAnnounce(t("voice.announce.transcribing"));
    const speech = trimSilence(downsample(samples, rate));
    const nothing = () => {
      end();
      setNote(t("voice.nothingHeard"));
      onAnnounce(t("voice.nothingHeard"));
    };
    if (speech.length === 0) {
      EngineService.CancelDictation(r.id).catch(() => {});
      nothing();
      return;
    }
    EngineService.FinishDictation(r.id, base64(encodeWav(speech)))
      .then((said) => {
        if (token.current !== r.token) return;
        if (!said.trim()) return nothing();
        end();
        onText(said);
        onAnnounce(t("voice.announce.inserted"));
      })
      .catch((err) => {
        if (token.current !== r.token) return;
        end();
        setNote(String(err?.message ?? err));
      });
  };
  const toggle = () => {
    if (phase === "idle") start();
    else if (phase === "recording") finish();
    else cancel();
  };

  // The clock, and the end of the longest recording taken.
  const finishNow = useEffectEvent(() => finish());
  useEffect(() => {
    if (phase !== "recording") return;
    const timer = setInterval(() => {
      const s = (Date.now() - (run.current?.started ?? Date.now())) / 1000;
      setSeconds(s);
      if (s >= MAX_SECONDS) finishNow();
    }, 250);
    return () => clearInterval(timer);
  }, [phase]);

  // The shortcut starts and stops it in the pane in use; Escape throws it away.
  const latest = useRef({ toggle, cancel, phase });
  latest.current = { toggle, cancel, phase };
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (pressed(e, "dictate")) {
        e.preventDefault();
        latest.current.toggle();
      } else if (e.key === "Escape" && latest.current.phase !== "idle") {
        e.preventDefault();
        e.stopPropagation();
        latest.current.cancel();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [active]);
  // Leaving the session throws a recording away.
  useEffect(() => () => {
    if (run.current) {
      run.current.capture?.cancel();
      EngineService.CancelDictation(run.current.id).catch(() => {});
    }
  }, []);

  // The scripted stand-in records a tone, so it needs no microphone.
  if (!(canDictate() || status?.scripted) || status?.state === "unsupported") return null;
  const key = bindingOf("dictate");
  const label = phase === "recording" ? t("voice.stop") : phase === "idle" ? t("voice.start") : t("voice.cancel");
  return (
    <>
      {phase === "recording" && (
        <span className="dictation-live" aria-hidden="true">
          <span className="dictation-meter" style={{ transform: `scaleX(${Math.max(0.05, meter)})` }} />
          <span className="dictation-clock">{clock(seconds)}</span>
        </span>
      )}
      {phase === "transcribing" && (
        <span className="dictation-live muted" aria-hidden="true">
          {t("voice.transcribing")}
        </span>
      )}
      {note && phase === "idle" && (
        <span className="dictation-note" role="status">
          {note}
        </span>
      )}
      <button
        className={`ghost composer-dictate${phase === "recording" ? " recording" : ""}`}
        aria-label={label}
        aria-pressed={phase === "recording"}
        aria-keyshortcuts={ariaKeys(key)}
        title={`${phase === "idle" ? t("voice.start.tip") : label} (${keyLabel(key)})`}
        onClick={toggle}
        disabled={phase === "starting"}
      >
        {phase === "idle" || phase === "starting" ? <MicIcon /> : <StopIcon />}
      </button>
      {asking && (
        <VoiceDownload
          status={status}
          onStatus={setStatus}
          onClose={() => setAsking(false)}
          onReady={() => {
            setAsking(false);
            onAnnounce(t("voice.ready"));
          }}
        />
      )}
    </>
  );
}

// size writes bytes as megabytes in the window's language.
export function size(bytes: number): string {
  return t("voice.megabytes", { n: Math.round(bytes / 1e6).toLocaleString(locale()) });
}

// useDownload follows a download of the speech model, asking every half second.
export function useVoiceDownload(status: VoiceStatus | null, onStatus: (s: VoiceStatus) => void) {
  const downloading = status?.state === "downloading";
  const report = useEffectEvent((s: VoiceStatus) => onStatus(s));
  useEffect(() => {
    if (!downloading) return;
    const timer = setInterval(() => EngineService.VoiceStatus().then(report).catch(() => {}), 500);
    return () => clearInterval(timer);
  }, [downloading]);
}

// VoiceDownload asks to download the speech model the first time voice input is
// used, and shows how the download goes.
function VoiceDownload({ status, onStatus, onClose, onReady }: { status: VoiceStatus | null; onStatus: (s: VoiceStatus) => void; onClose: () => void; onReady: () => void }) {
  const [error, setError] = useState("");
  useVoiceDownload(status, onStatus);
  const ready = useEffectEvent(() => onReady());
  useEffect(() => {
    if (status?.state === "ready") ready();
  }, [status?.state]);
  const download = () => {
    setError("");
    EngineService.InstallVoice()
      .then(() => EngineService.VoiceStatus().then(onStatus))
      .catch((err) => setError(String(err?.message ?? err)));
  };
  const stop = () => EngineService.CancelVoiceInstall().then(() => EngineService.VoiceStatus().then(onStatus));
  const downloading = status?.state === "downloading";
  return (
    <div className="palette-backdrop" onMouseDown={(e) => e.target === e.currentTarget && !downloading && onClose()}>
      <div
        className="afk-dialog voice-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={t("voice.dialog.title")}
        onKeyDown={(e) => e.key === "Escape" && !downloading && (e.stopPropagation(), onClose())}
      >
        <h2>{t("voice.dialog.title")}</h2>
        <p>{t("voice.dialog.body")}</p>
        {status && <p className="muted">{t("voice.dialog.what", { model: status.modelName, size: size(status.download), license: status.license })}</p>}
        <p className="muted">{t("voice.dialog.where")}</p>
        {downloading && status && (
          <>
            <progress className="voice-progress" max={status.total} value={status.received} aria-label={t("voice.dialog.progress")} />
            <p className="muted" role="status">
              {t("voice.dialog.received", { received: size(status.received), total: size(status.total) })}
            </p>
          </>
        )}
        {(error || status?.state === "failed") && (
          <p className="error-text" role="alert">
            {error || status?.error}
          </p>
        )}
        <p className="row">
          {downloading ? (
            <button onClick={stop}>{t("voice.dialog.stop")}</button>
          ) : (
            <>
              <button className="primary" autoFocus onClick={download}>
                {t("voice.dialog.download")}
              </button>
              <button onClick={onClose}>{t("common.cancel")}</button>
            </>
          )}
        </p>
      </div>
    </div>
  );
}
