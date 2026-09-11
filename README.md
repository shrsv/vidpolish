<p align="center">
  <img src="media/logo.svg" alt="vidpolish logo" width="140" height="140">
</p>

<h1 align="center">vidpolish</h1>

<p align="center">
  Turn a raw screen recording into a polished, clean, tightly cut video with one command.
</p>

---

## What it does

vidpolish takes a raw talking-head or screen recording and runs it through a
small pipeline:

1. **Split** the input into a video-only stream and an audio-only track.
2. **Denoise** the audio with [DeepFilterNet](https://github.com/Rikorose/DeepFilterNet)
   (the `deep-filter` CLI), the same kind of deep-learning noise removal
   behind tools like Adobe Podcast.
3. **Remux** the cleaned audio back onto the video.
4. **Auto-cut** silence and dead air with [auto-editor](https://github.com/WyattBlue/auto-editor),
   optionally speeding up the parts you kept.

Every stage shells out to a well-tested external binary instead of
reinventing audio/video processing in Go: vidpolish is just the orchestrator
gluing them together with sensible defaults, caching, and a simple CLI.

## Why

Recording a quick Loom-style walkthrough is easy. Cleaning it up by hand,
trimming pauses, cutting background hiss, isn't. vidpolish automates that
part so a five-minute rough recording turns into a tight, clean clip in one
command, without you touching a timeline editor.

## Prerequisites

You need Go and `ffmpeg` installed before building or running vidpolish.
Everything else (`deep-filter`, `auto-editor`) is fetched automatically the
first time you run the tool, so you do not need to install those by hand.

| Tool | Why it's needed | How to get it |
| --- | --- | --- |
| [Go](https://go.dev/dl/) 1.24+ | builds and runs the CLI | `apt install golang-go`, `brew install go`, or the official installer |
| `ffmpeg` / `ffprobe` | splitting, remuxing, and probing video/audio | `apt install ffmpeg`, `brew install ffmpeg`, or download from [ffmpeg.org](https://ffmpeg.org/download.html) |
| `deep-filter` | audio denoising | downloaded automatically by vidpolish from the [DeepFilterNet releases](https://github.com/Rikorose/DeepFilterNet/releases) into `~/.cache/vidpolish/bin` |
| `auto-editor` | silence cutting and speed changes | downloaded automatically by vidpolish from the [auto-editor releases](https://github.com/WyattBlue/auto-editor/releases) into `~/.cache/vidpolish/bin` |

Supported platforms for the auto-downloaded binaries: Linux (x86_64,
aarch64), macOS (Intel and Apple Silicon), and Windows (x86_64).

### Check and install everything in one go

Once Go and ffmpeg are on your machine, let vidpolish resolve the rest:

```sh
git clone git@github.com:shrsv/vidpolish.git
cd vidpolish
go build -o vidpolish ./cmd/vidpolish

./vidpolish deps
```

`vidpolish deps` prints the resolved path for each dependency and downloads
whatever is missing. Run it any time to double check your setup:

```
ffmpeg       /usr/bin/ffmpeg
ffprobe      /usr/bin/ffprobe
deep-filter  /home/you/.cache/vidpolish/bin/deep-filter/0.5.6/deep-filter
auto-editor  /home/you/.cache/vidpolish/bin/auto-editor/31.6.0/auto-editor
```

If `ffmpeg`/`ffprobe` are missing, `deps` tells you to install them; it will
not attempt to fetch those itself since they are near-universally available
through a system package manager.

## Usage

The basic case, denoise and trim silence with default settings:

```sh
./vidpolish process path/to/recording.mp4
# -> output/recording-polished.mp4
```

Give the kept speech more breathing room if cuts feel too tight (default is
`0.2s`):

```sh
./vidpolish process --margin 0.3s path/to/recording.mp4
```

Speed up the parts you kept, say to tighten a long walkthrough (silence is
still fully cut, not just sped through):

```sh
./vidpolish process --speed 1.25 path/to/recording.mp4
./vidpolish process --speed 1.5  path/to/recording.mp4
./vidpolish process --speed 1.75 path/to/recording.mp4
```

Send the result somewhere other than `./output`:

```sh
./vidpolish process --output-dir ~/Desktop/clips path/to/recording.mp4
```

Force a full recompute even if a cached run already exists for this file:

```sh
./vidpolish process --no-cache path/to/recording.mp4
```

Flags can go before or after the input path; both of these work the same
way:

```sh
./vidpolish process --speed 1.5 path/to/recording.mp4
./vidpolish process path/to/recording.mp4 --speed 1.5
```

### All `process` flags

| Flag | Default | Description |
| --- | --- | --- |
| `--margin` | `0.2s` | Padding kept around detected speech before a cut. Raise it (e.g. `0.3s`) if words feel clipped. |
| `--speed` | `1.0` | Speed multiplier for kept/spoken segments only, between `0.5` and `4.0`. Silence stays fully cut. |
| `--output-dir` | `output` | Directory the final file is written to. |
| `--no-cache` | off | Ignore cached intermediate artifacts and recompute everything. |

### Cache commands

```sh
./vidpolish cache clean   # remove all cached pipeline artifacts right now
```

## How caching works

The expensive parts of the pipeline (splitting, denoising, remuxing) do not
depend on `--margin` or `--speed`, only on the source file. vidpolish caches
those intermediate artifacts at `~/.vidpolish/cache/<fingerprint>/`, keyed
by a fast content fingerprint of the input (its size, modification time,
and sampled head/tail bytes, not a full-file hash). Re-running `process` on
the same file with a different `--speed` or `--margin` reuses that cached
work and only re-runs the fast final cut, instead of redoing several
minutes of denoising.

Cache entries are kept for 7 days and swept lazily on each run, or you can
purge them immediately with `vidpolish cache clean`. This is a separate
cache from `~/.cache/vidpolish/bin`, which only holds the downloaded tool
binaries themselves.

**Your source file is never modified.** Every stage reads the input and
writes to a separate cache/output path; nothing in the pipeline writes back
to the file you pass to `process`.

## Project layout

```
cmd/vidpolish       CLI entrypoint (flag parsing, wiring)
internal/binmgr      resolves/downloads deep-filter and auto-editor
internal/cache       the ~/.vidpolish/cache artifact cache
internal/pipeline    the split / denoise / remux / auto-edit stages
media/               logo assets
```

## Status and roadmap

vidpolish currently covers the local processing pipeline: split, denoise,
cut, and optional speed changes, with caching to make iterating fast.

Planned next:

- YouTube upload and metadata automation (title, description, default
  language such as Indian English).
- A UI on top of the same library/CLI.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
