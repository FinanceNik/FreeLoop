package engine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// fakeWhisper is whisper-cli for the tests: it checks it was asked for the words of
// a WAV on stdin in the language given, and answers as whisper-cli does.
func fakeWhisper() int {
	args := strings.Join(os.Args[1:], " ")
	for _, want := range []string{"-f -", "-l " + os.Getenv("FREELOOP_FAKE_WHISPER_LANG"), "-nt", "-of dictation"} {
		if !strings.Contains(args, want) {
			fmt.Fprintf(os.Stderr, "missing %q in %s\n", want, args)
			return 2
		}
	}
	if os.Getenv("FREELOOP_FAKE_WHISPER") == "broken" {
		fmt.Fprintln(os.Stderr, "whisper_init_from_file_with_params_no_state: failed to load model")
		return 1
	}
	wav, _ := io.ReadAll(os.Stdin)
	if _, err := checkWAV(wav); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	fmt.Print("\n Hallo [BLANK_AUDIO] wereld.")
	return 0
}

// wav16k is a recording of n samples as the frontend makes it.
func wav16k(n int) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	b.WriteString("RIFF")
	binary.Write(&b, le, uint32(36+2*n))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		binary.Write(&b, le, v)
	}
	b.WriteString("data")
	binary.Write(&b, le, uint32(2*n))
	b.Write(make([]byte, 2*n))
	return b.Bytes()
}

func readyVoice(t *testing.T, e *Engine, mode string) {
	t.Helper()
	exe, _ := os.Executable()
	e.voice.program = exe
	e.voice.env = map[string]string{"FREELOOP_FAKE_WHISPER": mode, "FREELOOP_FAKE_WHISPER_LANG": "nl"}
	if err := environment.InstallTool(e.speechModel(), []byte("model")); err != nil {
		t.Fatal(err)
	}
	if st := e.VoiceStatus(); st.State != "ready" {
		t.Fatalf("status = %+v", st)
	}
}

// GUI-050: a dictation runs whisper-cli with the recording on its stdin, and the
// words come back without the marks for what was not speech.
func TestDictationTurnsARecordingIntoText(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.StartDictation("nl"); err == nil {
		t.Fatal("a dictation started without the model")
	}
	readyVoice(t, e, "1")
	ctx := context.Background()
	wav := base64.StdEncoding.EncodeToString(wav16k(16000))

	id, err := e.StartDictation("nl")
	if err != nil {
		t.Fatal(err)
	}
	text, err := e.FinishDictation(ctx, id, wav)
	if err != nil || text != "Hallo wereld." {
		t.Fatalf("text %q, %v", text, err)
	}
	if _, err := e.FinishDictation(ctx, id, wav); err == nil {
		t.Error("a dictation was finished twice")
	}

	// The language chosen in Settings wins over the window's.
	cfg := e.Config()
	cfg.Voice.Language = "nl"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	id, _ = e.StartDictation("de")
	if text, err := e.FinishDictation(ctx, id, wav); err != nil || text != "Hallo wereld." {
		t.Fatalf("with the language set: %q, %v", text, err)
	}

	// Cancelled, it ends; a recording that is not one is refused.
	id, _ = e.StartDictation("nl")
	e.CancelDictation(id)
	if _, err := e.FinishDictation(ctx, id, wav); err == nil {
		t.Error("a cancelled dictation finished")
	}
	id, _ = e.StartDictation("nl")
	if _, err := e.FinishDictation(ctx, id, base64.StdEncoding.EncodeToString([]byte("not a wav at all, not at all, not at all, no"))); err == nil {
		t.Error("took a recording that is not a WAV")
	}
}

func TestDictationSaysWhyTheModelFailed(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	readyVoice(t, e, "broken")
	id, err := e.StartDictation("nl")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.FinishDictation(context.Background(), id, base64.StdEncoding.EncodeToString(wav16k(1600)))
	if err == nil || !strings.Contains(err.Error(), "download it again") {
		t.Fatalf("err = %v", err)
	}
}

func TestWAVsAreChecked(t *testing.T) {
	if d, err := checkWAV(wav16k(32000)); err != nil || d != 2*time.Second {
		t.Fatalf("%v %v", d, err)
	}
	// Another writer's chunk before the samples, as macOS's say writes one.
	plain := wav16k(16000)
	padded := append(append(append([]byte{}, plain[:36]...), append([]byte("FLLR\x04\x00\x00\x00"), 0, 0, 0, 0)...), plain[36:]...)
	if d, err := checkWAV(padded); err != nil || d != time.Second {
		t.Errorf("with a chunk before the data: %v %v", d, err)
	}
	stereo := wav16k(100)
	binary.LittleEndian.PutUint16(stereo[22:24], 2)
	long := wav16k(16000 * 200)
	for name, b := range map[string][]byte{"stereo": stereo, "short": []byte("RIFF"), "longer than allowed": long} {
		if _, err := checkWAV(b); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
}

func TestDictatedTextLeavesOutWhatWasNotSpeech(t *testing.T) {
	for in, want := range map[string]string{
		"\n Make the button blue.":         "Make the button blue.",
		" [BLANK_AUDIO]":                   "",
		" (Musik)":                         "",
		" *Applaus*":                       "",
		" ***":                             "",
		" ...":                             "",
		" Fix the test [Musik] in api.go.": "Fix the test in api.go.",
		" Call foo(bar) twice.":            "Call foo(bar) twice.",
		"  Zet het\n  in de map src. ":     "Zet het in de map src.",
	} {
		if got := dictatedText(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestSpeechFilesKeepsOnlyTheProgram(t *testing.T) {
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	add := func(h *tar.Header, body string) {
		h.Size = int64(len(body))
		tw.WriteHeader(h)
		tw.Write([]byte(body))
	}
	add(&tar.Header{Name: "whisper-bin-ubuntu-x64/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	add(&tar.Header{Name: "whisper-bin-ubuntu-x64/whisper-cli", Typeflag: tar.TypeReg, Mode: 0o755}, "cli")
	add(&tar.Header{Name: "whisper-bin-ubuntu-x64/libwhisper.so.1.9.5", Typeflag: tar.TypeReg, Mode: 0o755}, "lib")
	add(&tar.Header{Name: "whisper-bin-ubuntu-x64/libwhisper.so.1", Typeflag: tar.TypeSymlink, Linkname: "libwhisper.so.1.9.5"}, "")
	add(&tar.Header{Name: "whisper-bin-ubuntu-x64/whisper-server", Typeflag: tar.TypeReg, Mode: 0o755}, "server")
	tw.Close()
	gz.Close()
	keep := config.BuiltinSpeech().Programs["linux/amd64"].Keep
	files, err := speechFiles(tgz.Bytes(), keep, "whisper-cli")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name+"->"+f.Link)
	}
	slices.Sort(names)
	if want := []string{"libwhisper.so.1->libwhisper.so.1.9.5", "libwhisper.so.1.9.5->", "whisper-cli->"}; !slices.Equal(names, want) {
		t.Errorf("kept %v", names)
	}

	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	for _, n := range []string{"Release/whisper-cli.exe", "Release/whisper.dll", "Release/ggml-cpu-haswell.dll", "Release/SDL2.dll", "Release/wchess.exe"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	files, err = speechFiles(z.Bytes(), config.BuiltinSpeech().Programs["windows/amd64"].Keep, "whisper-cli.exe")
	if err != nil || len(files) != 3 {
		t.Fatalf("kept %+v, %v", files, err)
	}
	if _, err := speechFiles(z.Bytes(), []string{"*.dll"}, "whisper-cli.exe"); err == nil {
		t.Error("an archive without the program was taken")
	}
}

// GUI-050: the download is the pinned program and model, or nothing; local-only
// mode refuses it.
func TestVoiceDownloadIsPinned(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	model := bytes.Repeat([]byte("weights"), 50_000)
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	program := exeName(goruntime.GOOS, "whisper-cli")
	tw.WriteHeader(&tar.Header{Name: "bin/" + program, Typeflag: tar.TypeReg, Mode: 0o755, Size: 3})
	tw.Write([]byte("cli"))
	tw.Close()
	gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/whisper.tar.gz":
			w.Write(tgz.Bytes())
		case "/model.bin":
			w.Write(model)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	sum := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	sp := config.BuiltinSpeech()
	sp.Programs = map[string]config.SpeechProgram{config.AssetKey(): {
		SpeechDownload: config.SpeechDownload{URL: srv.URL + "/whisper.tar.gz", SHA256: sum(tgz.Bytes()), Size: int64(tgz.Len())},
		Keep:           []string{program},
	}}
	sp.Model = config.SpeechDownload{URL: srv.URL + "/model.bin", SHA256: sum(model), Size: int64(len(model))}
	e.voice.speech = sp

	wait := func() VoiceStatus {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if st := e.VoiceStatus(); st.State != "downloading" {
				return st
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("the download did not end")
		return VoiceStatus{}
	}

	netguard.SetLocalOnly(true)
	err := e.InstallVoice()
	netguard.SetLocalOnly(false)
	if err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Fatalf("local-only: %v", err)
	}

	good := sp.Model
	sp.Model.SHA256 = sum([]byte("another model"))
	e.voice.speech = sp
	if err := e.InstallVoice(); err != nil {
		t.Fatal(err)
	}
	if st := wait(); st.State != "failed" || !strings.Contains(st.Error, "not the file Freeloop knows") {
		t.Fatalf("a wrong model: %+v", st)
	}
	if environment.IsFile(e.speechModel()) {
		t.Fatal("a wrong model was kept")
	}

	sp.Model = good
	e.voice.speech = sp
	if st := e.VoiceStatus(); st.Download != good.Size {
		t.Errorf("after the program, only the model is left to download: %+v", st)
	}
	if err := e.InstallVoice(); err != nil {
		t.Fatal(err)
	}
	if st := wait(); st.State != "ready" || st.Error != "" {
		t.Fatalf("status = %+v", st)
	}
	if b, _ := os.ReadFile(filepath.Join(e.downloadedSpeech(), program)); string(b) != "cli" {
		t.Errorf("program = %q", b)
	}

	if err := e.RemoveVoice(); err != nil {
		t.Fatal(err)
	}
	if st := e.VoiceStatus(); st.State != "missing" {
		t.Errorf("after removing: %+v", st)
	}
}

// The pins name a build for every system Freeloop is made for, but the Mac, whose
// build the app bundles.
func TestSpeechIsPinnedForEverySystem(t *testing.T) {
	sp := config.BuiltinSpeech()
	for _, key := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		p, ok := sp.Programs[key]
		if !ok || len(p.SHA256) != 64 || p.Size <= 0 || !strings.HasPrefix(p.URL, "https://github.com/ggml-org/whisper.cpp/releases/download/") {
			t.Errorf("%s: %+v", key, p)
		}
	}
	if _, ok := sp.Programs["darwin/arm64"]; ok {
		t.Error("the Mac's build is bundled, not downloaded")
	}
	if len(sp.Model.SHA256) != 64 || !strings.Contains(sp.Model.URL, "/resolve/5359861c") {
		t.Errorf("model: %+v", sp.Model)
	}
}
