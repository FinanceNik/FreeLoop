// Command freeloop-sbom writes the bill of materials of a built app (SEC-012).
//
//	freeloop-sbom -binary bin/Freeloop -version 0.0.1 < frontend/package-lock.json > sbom.json
package main

import (
	"debug/buildinfo"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/FinanceNik/Freeloop/internal/sbom"
)

func main() {
	binary := flag.String("binary", "", "the built app's binary")
	version := flag.String("version", "", "the app's version")
	flag.Parse()
	if err := run(*binary, *version); err != nil {
		fmt.Fprintln(os.Stderr, "freeloop-sbom:", err)
		os.Exit(1)
	}
}

func run(binary, version string) error {
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return err
	}
	packageLock, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	b, err := sbom.Build(sbom.App{Name: "Freeloop", Version: version}, info, packageLock)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}
