// Command freeloop-sbom writes the bill of materials of a built app (SEC-012).
//
//	freeloop-sbom -binary bin/Freeloop -version 0.0.1 < frontend/package-lock.json > sbom.json
//
// With -notices it writes the licenses of the same things instead, and lists on
// stderr anything that has none to copy.
//
//	freeloop-sbom -notices -modcache "$(go env GOMODCACHE)" -node-modules frontend/node_modules \
//	  -binary bin/Freeloop -version 0.0.1 < frontend/package-lock.json > THIRD_PARTY_NOTICES.txt
//
// -native names a native program the app carries, by the description beside it
// (GUI-050: bin/whisper-cli.json on a Mac).
package main

import (
	"debug/buildinfo"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/FinanceNik/Freeloop/internal/sbom"
)

func main() {
	binary := flag.String("binary", "", "the built app's binary")
	version := flag.String("version", "", "the app's version")
	notices := flag.Bool("notices", false, "write third-party licenses instead of the bill")
	modCache := flag.String("modcache", "", "with -notices: the Go module cache")
	nodeModules := flag.String("node-modules", "", "with -notices: the frontend's node_modules")
	native := flag.String("native", "", "a native program's description, such as bin/whisper-cli.json")
	flag.Parse()
	var natives []sbom.Native
	if *native != "" {
		n, err := sbom.ReadNative(os.DirFS("."), filepath.ToSlash(filepath.Clean(*native)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "freeloop-sbom:", err)
			os.Exit(1)
		}
		natives = append(natives, n)
	}
	var src *sbom.NoticeSources
	if *notices {
		src = &sbom.NoticeSources{ModCache: os.DirFS(*modCache), NodeModules: os.DirFS(*nodeModules), Natives: natives, NativeRoot: os.DirFS(".")}
	}
	if err := run(*binary, *version, src, natives); err != nil {
		fmt.Fprintln(os.Stderr, "freeloop-sbom:", err)
		os.Exit(1)
	}
}

func run(binary, version string, notices *sbom.NoticeSources, natives []sbom.Native) error {
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return err
	}
	packageLock, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	app := sbom.App{Name: "Freeloop", Version: version}
	if notices != nil {
		text, missing, err := sbom.Notices(app, info, packageLock, *notices)
		if err != nil {
			return err
		}
		for _, m := range missing {
			fmt.Fprintln(os.Stderr, "freeloop-sbom: no license file for", m)
		}
		_, err = os.Stdout.Write(text)
		return err
	}
	b, err := sbom.Build(app, info, packageLock, natives...)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}
