// Voice input (GUI-050): hands what the microphone hears to the page, in blocks of
// about 85 ms. It is a file of the app's own because the page's policy runs no
// script from anywhere else, a blob: URL included (vite.config.ts).
class Capture extends AudioWorkletProcessor {
  constructor() {
    super();
    this.block = new Float32Array(4096);
    this.filled = 0;
  }
  process(inputs) {
    const channel = inputs[0] && inputs[0][0];
    if (channel) {
      let from = 0;
      while (from < channel.length) {
        const n = Math.min(channel.length - from, this.block.length - this.filled);
        this.block.set(channel.subarray(from, from + n), this.filled);
        this.filled += n;
        from += n;
        if (this.filled === this.block.length) {
          this.port.postMessage(this.block, [this.block.buffer]);
          this.block = new Float32Array(4096);
          this.filled = 0;
        }
      }
    }
    return true;
  }
}
registerProcessor("capture", Capture);
