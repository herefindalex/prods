package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"prods/internal/sampledata"
)

func main() {
	releaseVersion := flag.String("release-version", "", "exact Prods release version embedded in this sample")
	output := flag.String("output", "", "new JSON output path")
	flag.Parse()
	if strings.TrimSpace(*releaseVersion) == "" || strings.TrimSpace(*output) == "" {
		fmt.Fprintln(os.Stderr, "usage: prods-sample -release-version TAG -output PATH")
		os.Exit(2)
	}
	sample, err := sampledata.Generate(*releaseVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err = file.Write(encoded); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
