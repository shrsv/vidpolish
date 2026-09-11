// Command vidpolish is a CLI that turns a raw screen recording into a
// polished video: split audio/video, denoise the audio with DeepFilterNet,
// and cut silence with auto-editor.
package main

import (
	"flag"
	"fmt"
	"os"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/cache"
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
	case "cache":
		runCache(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vidpolish - split, denoise, and auto-cut raw recordings

Usage:
  vidpolish process <input.mp4> [flags]
  vidpolish deps
  vidpolish cache clean`)
}

func runProcess(args []string) {
	fs := flag.NewFlagSet("process", flag.ExitOnError)
	outputDir := fs.String("output-dir", "output", "directory to write the final polished video to")
	margin := fs.String("margin", "0.2s", "auto-editor margin around kept speech (e.g. 0.2s, 0.3s)")
	speed := fs.Float64("speed", 1.0, "playback speed multiplier for kept/spoken segments (e.g. 1.25, 1.5, 1.75)")
	noCache := fs.Bool("no-cache", false, "ignore cached intermediate artifacts and recompute everything")
	// flag.Parse stops at the first non-flag argument, so reorder to let the
	// input path appear anywhere on the command line (before or after flags).
	fs.Parse(reorderFlags(args))

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpolish process <input.mp4> [flags]")
		os.Exit(1)
	}
	if *speed < 0.5 || *speed > 4.0 {
		fmt.Fprintln(os.Stderr, "error: --speed must be between 0.5 and 4.0")
		os.Exit(1)
	}

	out, err := pipeline.Process(pipeline.Options{
		Input:     fs.Arg(0),
		OutputDir: *outputDir,
		Margin:    *margin,
		Speed:     *speed,
		NoCache:   *noCache,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("done:", out)
}

func runCache(args []string) {
	if len(args) != 1 || args[0] != "clean" {
		fmt.Fprintln(os.Stderr, "usage: vidpolish cache clean")
		os.Exit(1)
	}
	root, err := cache.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := cache.Sweep(root, 0); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("cleaned", root)
}

// reorderFlags moves the positional argument to the front (before any
// flags), since Go's flag package otherwise stops parsing at the first
// non-flag token.
func reorderFlags(args []string) []string {
	boolFlags := map[string]bool{"-no-cache": true, "--no-cache": true}
	valueFlags := map[string]bool{
		"-output-dir": true, "--output-dir": true,
		"-margin": true, "--margin": true,
		"-speed": true, "--speed": true,
	}

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
