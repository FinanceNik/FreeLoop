import { test } from "node:test";
import assert from "node:assert/strict";
import { base64, clock, downsample, encodeWav, insertDictation, level, SPEECH_RATE, trimSilence } from "./voice.ts";

const tone = (seconds: number, amplitude: number, rate = SPEECH_RATE) =>
  Float32Array.from({ length: Math.round(seconds * rate) }, (_, i) => amplitude * Math.sin((2 * Math.PI * 220 * i) / rate));

test("a recording comes down to the speech model's rate", () => {
  const at48 = tone(1, 0.5, 48000);
  const at16 = downsample(at48, 48000);
  assert.equal(at16.length, SPEECH_RATE);
  // A 220 Hz tone keeps its loudness.
  assert.ok(Math.abs(Math.max(...at16) - 0.5) < 0.02);
  assert.equal(downsample(at16, SPEECH_RATE), at16);
  assert.equal(downsample(tone(1, 0.5, 44100), 44100).length, SPEECH_RATE);
});

test("silence is cut away, and silence alone is nothing", () => {
  const quiet = new Float32Array(SPEECH_RATE * 2).fill(0.001);
  const speech = tone(1, 0.3);
  const all = new Float32Array(quiet.length * 2 + speech.length);
  all.set(quiet);
  all.set(speech, quiet.length);
  all.set(quiet, quiet.length + speech.length);
  const kept = trimSilence(all);
  // The speech, with 0.3 s on either side.
  assert.ok(kept.length >= speech.length && kept.length <= speech.length + 0.7 * SPEECH_RATE, `${kept.length}`);
  assert.equal(trimSilence(quiet).length, 0);
  assert.equal(trimSilence(new Float32Array(0)).length, 0);
});

test("the WAV is 16 kHz mono 16-bit, as the engine checks", () => {
  const wav = encodeWav(Float32Array.from([0, 1, -1, 2]));
  const v = new DataView(wav.buffer);
  assert.equal(String.fromCharCode(...wav.subarray(0, 4)), "RIFF");
  assert.equal(String.fromCharCode(...wav.subarray(8, 16)), "WAVEfmt ");
  assert.equal(v.getUint16(20, true), 1);
  assert.equal(v.getUint16(22, true), 1);
  assert.equal(v.getUint32(24, true), SPEECH_RATE);
  assert.equal(v.getUint16(34, true), 16);
  assert.equal(String.fromCharCode(...wav.subarray(36, 40)), "data");
  assert.equal(v.getUint32(40, true), 8);
  assert.deepEqual([v.getInt16(44, true), v.getInt16(46, true), v.getInt16(48, true), v.getInt16(50, true)], [0, 32767, -32767, 32767]);
  assert.equal(base64(Uint8Array.from([104, 105])), "aGk=");
  assert.equal(base64(new Uint8Array(100_000)).length, 133_336);
});

test("dictated words go in at the caret with a space where one is needed", () => {
  assert.deepEqual(insertDictation("", 0, 0, " Make it blue. "), { text: "Make it blue.", caret: 13 });
  assert.deepEqual(insertDictation("Fix", 3, 3, "the test"), { text: "Fix the test", caret: 12 });
  assert.deepEqual(insertDictation("Fix ", 4, 4, "the test"), { text: "Fix the test", caret: 12 });
  assert.deepEqual(insertDictation("Fix  now", 4, 4, "it"), { text: "Fix it now", caret: 6 });
  assert.deepEqual(insertDictation("Fixnow", 3, 3, "it"), { text: "Fix it now", caret: 7 });
  assert.deepEqual(insertDictation("Fix it.", 3, 3, "all of"), { text: "Fix all of it.", caret: 10 });
  assert.deepEqual(insertDictation("Fix this.", 4, 8, "that"), { text: "Fix that.", caret: 8 });
  assert.deepEqual(insertDictation("Fix", 3, 3, "  "), { text: "Fix", caret: 3 });
});

test("the meter and the clock", () => {
  assert.equal(level(new Float32Array(10)), 0);
  assert.equal(level(Float32Array.from([0.25, -1])), 1);
  assert.equal(clock(7.9), "0:07");
  assert.equal(clock(125), "2:05");
});
