package engine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Voice input (GUI-050): what is said into the message box becomes text on this
// computer. Each dictation starts whisper.cpp's whisper-cli when recording starts,
// so the model loads while the user speaks; the recording goes to it on its stdin as
// a WAV and the text comes back on its stdout. Nothing listens on a port, the audio
// never touches the disk, and nothing stays running between dictations.

const (
	// maxDictation is the longest recording taken: a few minutes of speech is a
	// long prompt, and on a computer without a GPU it already takes a while.
	maxDictation = 3 * time.Minute
	// dictationLife ends a dictation that was started and never finished.
	dictationLife = maxDictation + time.Minute
	wavRate       = 16000
)

// VoiceStatus is what voice input needs and has.
type VoiceStatus struct {
	// State is "missing", "downloading", "ready", "failed" or "unsupported".
	State string `json:"state"`
	// Received and Total count the bytes of the download under way.
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Error    string `json:"error"`
	// Download is how many bytes a download would fetch; the program, where the
	// app does not bundle it, and the model.
	Download  int64  `json:"download"`
	ModelName string `json:"modelName"`
	License   string `json:"license"`
	Version   string `json:"version"`
	// Folder is where the downloads are kept.
	Folder string `json:"folder"`
	// Scripted is the stand-in for `make check-app`, which needs no microphone.
	Scripted bool `json:"scripted"`
}

type voice struct {
	speech   config.Speech
	scripted bool
	// program and env replace the program found and add to its environment, for
	// tests.
	program string
	env     map[string]string

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	fetching context.CancelFunc
	received int64
	total    int64
	failed   string
	// fake is a scripted download's having happened.
	fake       bool
	dictations map[string]*dictation
}

type dictation struct {
	p    environment.Process
	out  bytes.Buffer
	read chan struct{}
	stop *time.Timer
}

func newVoice(scripted bool) *voice {
	ctx, cancel := context.WithCancel(context.Background())
	return &voice{speech: config.BuiltinSpeech(), scripted: scripted, ctx: ctx, cancel: cancel, dictations: map[string]*dictation{}}
}

// close stops the download and every dictation, when the app quits.
func (v *voice) close() {
	v.cancel()
	v.mu.Lock()
	defer v.mu.Unlock()
	for id, d := range v.dictations {
		d.end()
		delete(v.dictations, id)
	}
}

func (e *Engine) voiceFolder() string { return e.appCache("voice") }

func (e *Engine) speechModel() string {
	return filepath.Join(e.voiceFolder(), e.voice.speech.ModelFile)
}

// speechProgram is whisper-cli: beside the app's own executable when the app
// bundles it, as a Mac build does, else what the app downloaded.
func (e *Engine) speechProgram() (path string, bundled bool) {
	if e.voice.program != "" {
		return e.voice.program, false
	}
	name := exeName(goruntime.GOOS, e.voice.speech.Program)
	if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), name); environment.IsFile(p) {
			return p, true
		}
	}
	return filepath.Join(e.downloadedSpeech(), name), false
}

func (e *Engine) downloadedSpeech() string {
	return filepath.Join(e.voiceFolder(), "whisper-"+e.voice.speech.Version)
}

// VoiceStatus says whether voice input can run here, and how a download goes.
func (e *Engine) VoiceStatus() VoiceStatus {
	sp := e.voice.speech
	st := VoiceStatus{ModelName: sp.ModelName, License: sp.License, Version: sp.Version, Folder: e.voiceFolder(), Scripted: e.voice.scripted}
	program, bundled := e.speechProgram()
	asset, downloadable := sp.Programs[config.AssetKey()]
	st.Download = sp.Model.Size
	if !bundled && !environment.IsFile(program) && downloadable {
		st.Download += asset.Size
	}
	v := e.voice
	v.mu.Lock()
	defer v.mu.Unlock()
	switch {
	case v.fetching != nil:
		st.State, st.Received, st.Total = "downloading", v.received, v.total
	case v.scripted && v.fake:
		st.State = "ready"
	case v.scripted:
		st.State = "missing"
	case !bundled && !downloadable && !environment.IsFile(program):
		st.State = "unsupported"
	case environment.IsFile(program) && environment.IsFile(e.speechModel()):
		st.State = "ready"
	case v.failed != "":
		st.State, st.Error = "failed", v.failed
	default:
		st.State = "missing"
	}
	return st
}

// InstallVoice starts downloading what voice input lacks, in the background;
// VoiceStatus tells how it goes. It runs only on the user's asking, and local-only
// mode refuses it.
func (e *Engine) InstallVoice() error {
	st := e.VoiceStatus()
	switch st.State {
	case "ready", "downloading":
		return nil
	case "unsupported":
		return fmt.Errorf("Freeloop has no build of whisper.cpp for this system (%s)", config.AssetKey())
	}
	if netguard.LocalOnly() {
		return errors.New("local-only mode is on, so Freeloop does not download the speech model; turn it off in Settings, Network, to download it")
	}
	v := e.voice
	ctx, cancel := context.WithCancel(v.ctx)
	v.mu.Lock()
	if v.fetching != nil {
		v.mu.Unlock()
		cancel()
		return nil
	}
	v.fetching, v.received, v.total, v.failed = cancel, 0, st.Download, ""
	v.mu.Unlock()
	go func() {
		defer cancel()
		err := e.fetchSpeech(ctx)
		v.mu.Lock()
		defer v.mu.Unlock()
		v.fetching = nil
		switch {
		case err == nil && v.scripted:
			v.fake = true
		case err == nil, ctx.Err() != nil:
		default:
			v.failed = err.Error()
		}
	}()
	return nil
}

// CancelVoiceInstall stops a download; what arrived is thrown away.
func (e *Engine) CancelVoiceInstall() {
	e.voice.mu.Lock()
	defer e.voice.mu.Unlock()
	if e.voice.fetching != nil {
		e.voice.fetching()
	}
}

// RemoveVoice deletes what voice input downloaded.
func (e *Engine) RemoveVoice() error {
	e.CancelVoiceInstall()
	v := e.voice
	v.mu.Lock()
	v.failed, v.fake = "", false
	for id, d := range v.dictations {
		d.end()
		delete(v.dictations, id)
	}
	v.mu.Unlock()
	// The download's goroutine lets go of its files once cancelled.
	for i := 0; i < 50 && e.VoiceStatus().State == "downloading"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	return environment.RemoveCache(e.voiceFolder())
}

func (e *Engine) fetchSpeech(ctx context.Context) error {
	v := e.voice
	sp := v.speech
	if v.scripted {
		// A download in steps, so the check sees progress.
		for i := int64(1); i <= 4; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(150 * time.Millisecond):
			}
			v.mu.Lock()
			v.received = v.total * i / 4
			v.mu.Unlock()
		}
		return nil
	}
	program, bundled := e.speechProgram()
	if !bundled && !environment.IsFile(program) {
		asset := sp.Programs[config.AssetKey()]
		dl, cancel := context.WithTimeout(ctx, toolDownloadFor)
		archive, err := fetchTool(dl, asset.URL)
		cancel()
		if err != nil {
			return fmt.Errorf("download whisper.cpp: %w", err)
		}
		if sum := sha256.Sum256(archive); hex.EncodeToString(sum[:]) != asset.SHA256 {
			return errors.New("the download of whisper.cpp is not the build Freeloop knows, so it was not kept")
		}
		files, err := speechFiles(archive, asset.Keep, filepath.Base(program))
		if err != nil {
			return fmt.Errorf("unpack whisper.cpp: %w", err)
		}
		if err := environment.InstallFolder(e.downloadedSpeech(), files); err != nil {
			return err
		}
		v.mu.Lock()
		v.received += asset.Size
		v.mu.Unlock()
	}
	if environment.IsFile(e.speechModel()) {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sp.Model.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download the speech model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download the speech model: the server answered %s", resp.Status)
	}
	v.mu.Lock()
	base := v.received
	v.mu.Unlock()
	err = environment.SaveDownload(e.speechModel(), resp.Body, sp.Model.Size, sp.Model.SHA256, func(n int64) {
		v.mu.Lock()
		v.received = base + n
		v.mu.Unlock()
	})
	if errors.Is(err, environment.ErrNotPinned) {
		return errors.New("the download of the speech model is not the file Freeloop knows, so it was not kept")
	}
	return err
}

// speechFiles takes the files keep names out of a whisper.cpp build, a .tar.gz
// for Linux or a .zip for Windows; program must be among them.
func speechFiles(archive []byte, keep []string, program string) ([]environment.ToolFile, error) {
	kept := func(name string) bool {
		for _, k := range keep {
			if ok, _ := path.Match(k, name); ok {
				return true
			}
		}
		return false
	}
	var files []environment.ToolFile
	if bytes.HasPrefix(archive, []byte("PK\x03\x04")) {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
			if !f.Mode().IsRegular() || !kept(name) {
				continue
			}
			if f.UncompressedSize64 > maxToolDownload {
				return nil, errors.New("it is larger than expected")
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(io.LimitReader(rc, maxToolDownload+1))
			rc.Close()
			if err != nil {
				return nil, err
			}
			files = append(files, environment.ToolFile{Name: name, Data: b})
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			name := path.Base(h.Name)
			if !kept(name) {
				continue
			}
			switch h.Typeflag {
			case tar.TypeReg:
				b, err := io.ReadAll(io.LimitReader(tr, maxToolDownload+1))
				if err != nil {
					return nil, err
				}
				files = append(files, environment.ToolFile{Name: name, Data: b})
			case tar.TypeSymlink:
				files = append(files, environment.ToolFile{Name: name, Link: h.Linkname})
			}
		}
	}
	for _, f := range files {
		if f.Name == program && f.Link == "" {
			return files, nil
		}
	}
	return nil, fmt.Errorf("it does not hold %s", program)
}

// voiceLanguage is the language whisper-cli is told the speech is in: the user's
// choice, or else the window's. Left to itself, Whisper takes a short Dutch clip
// for German or Afrikaans and then translates it.
func (e *Engine) voiceLanguage(window string) string {
	lang := e.Config().Voice.Language
	if lang == "" {
		lang = window
	}
	if lang == "auto" || regexp.MustCompile(`^[a-z]{2,3}$`).MatchString(lang) {
		return lang
	}
	return "auto"
}

// StartDictation starts the speech program for a recording that is starting, so
// its model loads while the user speaks, and returns the dictation's ID. window is
// the language the window is in.
func (e *Engine) StartDictation(window string) (string, error) {
	st := e.VoiceStatus()
	if st.State != "ready" {
		return "", errors.New("voice input needs its speech model; download it first")
	}
	v := e.voice
	id := rand.Text()
	if v.scripted {
		v.mu.Lock()
		v.dictations[id] = &dictation{p: nil, stop: time.AfterFunc(dictationLife, func() { e.CancelDictation(id) })}
		v.mu.Unlock()
		return id, nil
	}
	program, _ := e.speechProgram()
	folder := filepath.Dir(program)
	env, err := e.localEnv(folder, nil)
	if err != nil {
		return "", err
	}
	argv := []string{program,
		"-m", e.speechModel(),
		"-l", e.voiceLanguage(window),
		"-t", strconv.Itoa(min(8, max(1, goruntime.NumCPU()/2))),
		// Only the words: no timestamps, no progress, nothing for silence or noise.
		"-nt", "-np", "-sns", "-nth", "0.6",
		// Read the recording from stdin. An output name other than "-" makes it
		// print the text as it goes rather than to a file, which takes none.
		"-of", "dictation", "-f", "-",
	}
	p, err := env.Spawn(v.ctx, environment.SpawnSpec{Argv: argv, Cwd: folder, Env: v.env})
	if err != nil {
		return "", fmt.Errorf("start whisper.cpp: %w", err)
	}
	d := &dictation{p: p, read: make(chan struct{})}
	go func() {
		io.Copy(&d.out, io.LimitReader(p.Stdout(), 1<<20))
		io.Copy(io.Discard, p.Stdout())
		close(d.read)
	}()
	d.stop = time.AfterFunc(dictationLife, func() { e.CancelDictation(id) })
	v.mu.Lock()
	v.dictations[id] = d
	v.mu.Unlock()
	return id, nil
}

// CancelDictation stops a dictation and throws its recording away.
func (e *Engine) CancelDictation(id string) {
	v := e.voice
	v.mu.Lock()
	d, ok := v.dictations[id]
	delete(v.dictations, id)
	v.mu.Unlock()
	if ok {
		d.end()
	}
}

// end stops a dictation's timer and its program; a scripted one has none.
func (d *dictation) end() {
	d.stop.Stop()
	if d.p != nil {
		d.p.Kill()
	}
}

// FinishDictation hands a dictation its recording, a 16 kHz mono 16-bit WAV in
// base64, and returns what was said.
func (e *Engine) FinishDictation(ctx context.Context, id, wavBase64 string) (string, error) {
	v := e.voice
	v.mu.Lock()
	d, ok := v.dictations[id]
	delete(v.dictations, id)
	v.mu.Unlock()
	if !ok {
		return "", errors.New("the dictation ended; start it again")
	}
	d.stop.Stop()
	kill := func() {
		if d.p != nil {
			d.p.Kill()
		}
	}
	wav, err := base64.StdEncoding.DecodeString(wavBase64)
	if err != nil {
		kill()
		return "", errors.New("the recording could not be read")
	}
	length, err := checkWAV(wav)
	if err != nil {
		kill()
		return "", err
	}
	if v.scripted {
		return fmt.Sprintf("Dictated %.1f seconds.", length.Seconds()), nil
	}
	go func() {
		d.p.Stdin().Write(wav)
		d.p.Stdin().Close()
	}()
	// Time to load the model, then the speech at a CPU's slow pace.
	wait := time.NewTimer(30*time.Second + 2*length)
	defer wait.Stop()
	select {
	case <-d.read:
	case <-ctx.Done():
		d.p.Kill()
		return "", ctx.Err()
	case <-wait.C:
		d.p.Kill()
		return "", errors.New("the speech model took too long; try a shorter recording")
	}
	if err := d.p.Wait(); err != nil {
		return "", speechFailure(err, environment.StderrAfterExit(d.p))
	}
	return dictatedText(d.out.String()), nil
}

// checkWAV says how long a recording is, if it is 16 kHz mono 16-bit PCM, reading
// its chunks in turn, as other writers put more before the samples.
func checkWAV(b []byte) (time.Duration, error) {
	bad := errors.New("the recording is not a 16 kHz mono WAV")
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0, bad
	}
	le := binary.LittleEndian
	var format bool
	for at := 12; at+8 <= len(b); {
		id, size := string(b[at:at+4]), int(le.Uint32(b[at+4:at+8]))
		body := at + 8
		if size < 0 || size > len(b)-body {
			return 0, bad
		}
		switch id {
		case "fmt ":
			if size < 16 || le.Uint16(b[body:]) != 1 || le.Uint16(b[body+2:]) != 1 || le.Uint32(b[body+4:]) != wavRate || le.Uint16(b[body+14:]) != 16 {
				return 0, bad
			}
			format = true
		case "data":
			if !format {
				return 0, bad
			}
			length := time.Duration(size/2) * time.Second / wavRate
			if length > maxDictation+5*time.Second {
				return 0, fmt.Errorf("the recording is longer than %d minutes", int(maxDictation.Minutes()))
			}
			return length, nil
		}
		at = body + size + size%2
	}
	return 0, bad
}

// speechFailure says why whisper-cli stopped, in terms the user can act on.
func speechFailure(err error, stderr string) error {
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) {
		// STATUS_DLL_NOT_FOUND: the builds need Microsoft's C++ runtime.
		if code := uint32(exit.ExitCode()); code == 0xC0000135 {
			return errors.New("whisper.cpp needs the Microsoft Visual C++ Redistributable (2015–2022, x64 or ARM64); install it from Microsoft and try again")
		}
	}
	switch {
	case strings.Contains(stderr, "libgomp"):
		return errors.New("whisper.cpp needs libgomp (the GNU OpenMP library); install your system's libgomp1 package and try again")
	case strings.Contains(stderr, "GLIBC"):
		return errors.New("whisper.cpp's build needs a newer C library (glibc 2.35 or later) than this system has")
	case strings.Contains(stderr, "failed to load model"), strings.Contains(stderr, "failed to initialize"):
		return errors.New("the speech model could not be loaded; remove it in Settings, Voice input, and download it again")
	}
	if line := lastLine(stderr); line != "" {
		return fmt.Errorf("whisper.cpp stopped: %s", line)
	}
	return fmt.Errorf("whisper.cpp stopped: %w", err)
}

var (
	// Whisper marks what is not speech in brackets, as [BLANK_AUDIO] or [Musik].
	speechTag = regexp.MustCompile(`\[[^\]\n]{0,40}\]`)
	// A recording that was only a sound comes back as (music) or *Applaus*.
	onlySound = regexp.MustCompile(`^[(*][^()*\n]{0,40}[)*]$`)
	spaces    = regexp.MustCompile(`\s+`)
	// Anything said has a letter or a digit; silence can come back as "***".
	wordy = regexp.MustCompile(`[\p{L}\p{N}]`)
)

// dictatedText is whisper-cli's output as words for the message box.
func dictatedText(out string) string {
	s := strings.TrimSpace(spaces.ReplaceAllString(speechTag.ReplaceAllString(out, " "), " "))
	if onlySound.MatchString(s) || !wordy.MatchString(s) {
		return ""
	}
	return s
}
