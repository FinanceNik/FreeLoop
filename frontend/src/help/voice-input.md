# Voice input

Speak a prompt instead of typing it. The speech becomes text on this computer.

## Dictate

Click the microphone in the message box, or press ⌘⇧D, and speak. Click it again, or press ⌘⇧D again, and the words go into the message box where the cursor was. Nothing is sent: read it, change it, then send it as usual. Escape throws the recording away.

- A recording can be up to 3 minutes long.
- The first time, {{macOS|Windows|your system}} asks whether Freeloop may use the microphone.
- If nothing was heard, nothing is added. Speak a little louder or check the microphone.

## The speech model

Voice input runs Whisper large-v3-turbo through whisper.cpp, both MIT licensed. It is good in English, German and Dutch, and in many other languages.

The first time you dictate, Freeloop asks to download the model once: about 870 MB from Hugging Face, and on Windows and Linux whisper.cpp from GitHub. Each file is pinned and checked against a known checksum before it is kept. **Settings → Voice input** shows where it is kept and removes it. Local-only mode refuses the download.

How fast it is depends on the computer. On a Mac with Apple silicon, a sentence takes about a second. On a computer without a graphics chip whisper.cpp can use, a long recording can take several seconds.

## Which language

By default Freeloop tells the model that you speak the language the window is in. In **Settings → Voice input** you can name another language, or let the model detect it each time. Naming it is more accurate: a short Dutch sentence is easily taken for German.

## What stays on this computer

- What you say goes from the microphone to whisper.cpp on this computer, and is never written to disk or sent anywhere.
- whisper.cpp runs only while you dictate, and stops when the text is back.

## When it does not work

- **Freeloop may not use the microphone**: allow it in {{System Settings → Privacy & Security → Microphone|Settings → Privacy & security → Microphone, with "Let desktop apps access your microphone" on|your system's privacy or sound settings}}, then try again.
- {{After an update, macOS may ask again: Freeloop's builds are not signed with an Apple certificate.|On Windows, whisper.cpp needs the Microsoft Visual C++ Redistributable (2015–2022); Freeloop says so when it is missing.|On Linux, whisper.cpp needs glibc 2.35 or later and libgomp1, and the window needs GStreamer's PipeWire or PulseAudio plugin.}}
