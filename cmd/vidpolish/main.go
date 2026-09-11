// Command vidpolish is a CLI that turns a raw screen recording into a
// polished video: split audio/video, denoise the audio with DeepFilterNet,
// and cut silence with auto-editor.
package main

import (
	"flag"
	"fmt"
	"os"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "process":
		runProcess(os.Args[2:])
	case "deps":
		runDeps()
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vidpolish - split, denoise, and auto-cut raw recordings

Usage:
  vidpolish process <input.mp4> [flags]
  vidpolish deps`)
}

func runProcess(args []string) {
	fs := flag.NewFlagSet("process", flag.ExitOnError)
	outputDir := fs.String("output-dir", "output", "directory to write the final polished video to")
	keepTemp := fs.Bool("keep-temp", false, "keep intermediate working files instead of deleting them")
	// flag.Parse stops at the first non-flag argument, so reorder to let the
	// input path appear anywhere on the command line (before or after flags).
	fs.Parse(reorderFlags(args))

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpolish process <input.mp4> [flags]")
		os.Exit(1)
	}

	out, err := pipeline.Process(pipeline.Options{
		Input:     fs.Arg(0),
		OutputDir: *outputDir,
		KeepTemp:  *keepTemp,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("done:", out)
}

// reorderFlags moves the positional argument to the front (before any
// flags), since Go's flag package otherwise stops parsing at the first
// non-flag token.
func reorderFlags(args []string) []string {
	boolFlags := map[string]bool{"-keep-temp": true, "--keep-temp": true}
	valueFlags := map[string]bool{"-output-dir": true, "--output-dir": true}

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case boolFlags[a]:
			flags = append(flags, a)
		case valueFlags[a]:
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case len(a) > 0 && a[0] == '-':
			flags = append(flags, a)
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

func runDeps() {
	for _, tool := range []binmgr.Tool{binmgr.FFmpeg, binmgr.FFprobe, binmgr.DeepFilter, binmgr.AutoEditor} {
		path, err := binmgr.Resolve(tool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%-12s ERROR: %v\n", tool, err)
			continue
		}
		fmt.Printf("%-12s %s\n", tool, path)
	}
}
