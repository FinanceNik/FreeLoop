// Voice input (GUI-050): what the recorder hears becomes a WAV the engine's speech
// model reads, and what it says goes into the message box at the caret. Pure, so
// node --test covers it; the recording itself is in Dictation.tsx.

// The rate whisper.cpp works at.
export const SPEECH_RATE = 16000;
// The longest recording taken, as the engine allows.
export const MAX_SECONDS = 180;

// downsample brings samples at rate down to SPEECH_RATE, each output sample the
// average of the input it covers, which also keeps out most of what is too high to
// keep.
export function downsample(samples: Float32Array, rate: number): Float32Array {
  if (rate === SPEECH_RATE) return samples;
  const step = rate / SPEECH_RATE;
  const out = new Float32Array(Math.floor(samples.length / step));
  for (let i = 0; i < out.length; i++) {
    const from = Math.floor(i * step);
    const to = Math.max(from + 1, Math.floor((i + 1) * step));
    let sum = 0;
    for (let j = from; j < to; j++) sum += samples[j];
    out[i] = sum / (to - from);
  }
  return out;
}

// A recording's loudness, in frames of 20 ms.
const FRAME = SPEECH_RATE / 50;
// A frame louder than this holds speech; a quiet room is about 0.002.
const SPEAKING = 0.01;
// Kept on both sides of the speech, so a soft first or last syllable stays.
const MARGIN = SPEECH_RATE * 0.3;

function frameLevel(samples: Float32Array, from: number): number {
  const to = Math.min(samples.length, from + FRAME);
  let sum = 0;
  for (let i = from; i < to; i++) sum += samples[i] * samples[i];
  return to > from ? Math.sqrt(sum / (to - from)) : 0;
}

// trimSilence cuts the quiet before and after the speech in samples at SPEECH_RATE,
// and returns nothing when nothing was said: Whisper fills silence with words of its
// own, as "Untertitel im Auftrag des ZDF".
export function trimSilence(samples: Float32Array): Float32Array {
  let first = -1;
  let last = -1;
  for (let i = 0; i < samples.length; i += FRAME) {
    if (frameLevel(samples, i) > SPEAKING) {
      if (first < 0) first = i;
      last = i + FRAME;
    }
  }
  if (first < 0) return new Float32Array(0);
  return samples.subarray(Math.max(0, first - MARGIN), Math.min(samples.length, last + MARGIN));
}

// level is how loud samples are, from 0 to 1, for the meter.
export function level(samples: Float32Array): number {
  let peak = 0;
  for (const s of samples) peak = Math.max(peak, Math.abs(s));
  return Math.min(1, Math.sqrt(peak));
}

// encodeWav writes samples at SPEECH_RATE as a 16-bit mono WAV.
export function encodeWav(samples: Float32Array): Uint8Array {
  const bytes = new Uint8Array(44 + samples.length * 2);
  const v = new DataView(bytes.buffer);
  const text = (at: number, s: string) => [...s].forEach((c, i) => v.setUint8(at + i, c.charCodeAt(0)));
  text(0, "RIFF");
  v.setUint32(4, 36 + samples.length * 2, true);
  text(8, "WAVE");
  text(12, "fmt ");
  v.setUint32(16, 16, true);
  v.setUint16(20, 1, true);
  v.setUint16(22, 1, true);
  v.setUint32(24, SPEECH_RATE, true);
  v.setUint32(28, SPEECH_RATE * 2, true);
  v.setUint16(32, 2, true);
  v.setUint16(34, 16, true);
  text(36, "data");
  v.setUint32(40, samples.length * 2, true);
  samples.forEach((s, i) => v.setInt16(44 + i * 2, Math.round(Math.max(-1, Math.min(1, s)) * 0x7fff), true));
  return bytes;
}

// base64 writes bytes as the engine's bindings take them.
export function base64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

// insertDictation puts said into text in place of the selection from start to end,
// with a space between it and a word before or after, and says where the caret goes.
export function insertDictation(text: string, start: number, end: number, said: string): { text: string; caret: number } {
  const before = text.slice(0, start);
  const after = text.slice(end);
  const words = said.trim();
  if (!words) return { text, caret: end };
  const lead = before === "" || /\s$/.test(before) ? "" : " ";
  const trail = after === "" || /^[\s.,;:!?)]/.test(after) ? "" : " ";
  const head = before + lead + words;
  return { text: head + trail + after, caret: head.length + trail.length };
}

// clock writes seconds as 0:07.
export function clock(seconds: number): string {
  const s = Math.floor(seconds);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}
