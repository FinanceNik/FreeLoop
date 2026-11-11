package config

// Voice input (GUI-050): a speech model run on this computer by whisper.cpp's
// whisper-cli, both pinned by checksum. A Mac build bundles the program, built from
// the pinned source by `make whisper-cli`, since whisper.cpp publishes none for it;
// elsewhere the app downloads the project's own build when the user asks. The model
// is always downloaded on the user's click. A file cannot name either.

// Voice is the user's choice for voice input.
type Voice struct {
	// Language is what the speech is in: "auto", or a language code such as "de".
	// Empty means the language the app's window is in.
	Language string `yaml:"language,omitempty" json:"language"`
}

// VoiceLanguages are the choices offered for Language, besides empty.
var VoiceLanguages = []string{"auto", "en", "de", "nl"}

// SpeechDownload is a file the app fetches for voice input.
type SpeechDownload struct {
	URL    string
	SHA256 string
	Size   int64
}

// SpeechProgram is a build of whisper-cli in an archive.
type SpeechProgram struct {
	SpeechDownload
	// Keep are the files to unpack, as path.Match patterns on a name: the program
	// and the libraries it loads from its own folder.
	Keep []string
}

// Speech is what voice input runs.
type Speech struct {
	Version string
	License string
	// Program is whisper-cli's file name, without .exe.
	Program string
	// Programs are the builds by AssetKey; a Mac has none, its build is bundled.
	Programs map[string]SpeechProgram
	// Model is Whisper large-v3-turbo, quantized to 8 bits.
	Model     SpeechDownload
	ModelName string
	ModelFile string
}

// BuiltinSpeech is the pinned speech program and model. The program builds are the
// release attached to whisper.cpp's b5454 tag, the commit of v1.9.5, with GitHub's
// digests; the model is at a fixed revision of ggerganov/whisper.cpp on Hugging Face.
func BuiltinSpeech() Speech {
	const release = "https://github.com/ggml-org/whisper.cpp/releases/download/b5454/"
	linux := []string{"whisper-cli", "LICENSE", "libwhisper.so*", "libggml*.so*"}
	windows := []string{"whisper-cli.exe", "whisper.dll", "ggml*.dll", "libomp*.dll"}
	return Speech{
		Version: "1.9.5",
		License: "MIT",
		Program: "whisper-cli",
		Programs: map[string]SpeechProgram{
			"linux/amd64":   {SpeechDownload{release + "whisper-bin-ubuntu-x64.tar.gz", "a72becf15d7917f990f6313867a52638b82b7f9ef237fb0c980dac56a135781c", 10364195}, linux},
			"linux/arm64":   {SpeechDownload{release + "whisper-bin-ubuntu-arm64.tar.gz", "6b95ebfc60447df48e70ef00a73bdc3f41679ed2d01ef465827206a2ff325149", 4608377}, linux},
			"windows/amd64": {SpeechDownload{release + "whisper-bin-x64.zip", "6ba69e3482d7826214f90a6a9c84ca07782aec1e1d0c6a7c30c994fd5d816ccb", 8928640}, windows},
			"windows/arm64": {SpeechDownload{release + "whisper-bin-win-cpu-arm64.zip", "28c37e7b598c3d9bbfef94f3bd67f2ee6f12b7c86f3da5edcf5e4308d615ff6d", 4371193}, windows},
		},
		Model: SpeechDownload{
			URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3-turbo-q8_0.bin",
			SHA256: "317eb69c11673c9de1e1f0d459b253999804ec71ac4c23c17ecf5fbe24e259a1",
			Size:   874188075,
		},
		ModelName: "Whisper large-v3-turbo",
		ModelFile: "ggml-large-v3-turbo-q8_0.bin",
	}
}
