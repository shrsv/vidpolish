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

## YouTube upload

vidpolish can upload the polished result straight to YouTube, defaulting
to an unlisted video with progress and an ETA printed while it uploads.

### One-time setup in Google Cloud Console

Uploading is a write operation on your channel, so it needs OAuth 2.0
consent, not just an API key:

1. Create (or pick) a project at [console.cloud.google.com](https://console.cloud.google.com/),
   then enable the **YouTube Data API v3** under APIs & Services.
2. Under APIs & Services > Credentials, create an **OAuth client ID** of
   type **Desktop app**. Note the client ID and client secret.
3. If your project's OAuth consent screen is still in testing mode, add
   your own Google account as a test user so login doesn't get rejected.

### Configure and log in

```sh
./vidpolish config init
```

Creates `~/.vidpolish/config.toml`. Open it and fill in `client_id` and
`client_secret` from step 2 above, and adjust `privacy`/`default_language`
if you want different defaults. Then:

```sh
./vidpolish youtube login
```

Opens a browser for a one-time consent screen and stores a refresh token
back into the config file. You do not need to repeat this on future
uploads, only if you revoke access or move to a new machine.

If your Google Cloud project's OAuth consent screen is in testing mode,
add your own account under Test users first or the consent step will be
rejected.

On a headless machine or inside WSL, `vidpolish youtube login` cannot open
a browser for you; it prints the authorization URL instead so you can open
it yourself (on Windows, if you're in WSL). It listens on a local port for
up to 5 minutes waiting for you to approve it.

### Uploading

```sh
./vidpolish upload path/to/polished.mp4
./vidpolish upload path/to/polished.mp4 --title "Walkthrough" --privacy public --language en-IN
```

Or do the whole thing in one command, polish then upload:

```sh
./vidpolish process path/to/recording.mp4 --upload --speed 1.25 --privacy unlisted
```

Every upload prints progress and an ETA as it goes, then the video link
**as soon as it's known**, right after the upload itself finishes:

```
==> uploading path/to/polished.mp4 as unlisted
uploading: 100% (ETA ...)
uploaded: https://youtu.be/dQw4w9WgXcQ
==> processing status: processing
==> done, video is fully processed: https://youtu.be/dQw4w9WgXcQ
```

The link is valid and shareable the moment it prints; you do not need to
wait for the "fully processed" line. That part only continues to poll
YouTube (for up to about two minutes) so you can see when it finishes
transcoding. Skip that wait entirely with `--no-wait`, and the command
returns right after the `uploaded:` line.

By default the description and tags contain just `vidpolish`; see the
next section for setting your own defaults. Fill in anything else you
want from YouTube Studio afterward.

### `upload` / `process --upload` flags

| Flag | Default | Description |
| --- | --- | --- |
| `--title` | input filename | Video title. |
| `--privacy` | from config (`unlisted`) | `public`, `unlisted`, or `private`. |
| `--language` | from config (`en`) | BCP-47 language code, e.g. `en`, `en-IN`. |
| `--no-wait` | off | Return right after the upload finishes instead of also waiting on YouTube's processing status. |
| `--thumbnail <path>` | none | Use a specific pre-made image as the thumbnail instead of auto-generating one. |
| `--no-thumbnail` | off | Don't set a thumbnail for this upload, even if auto-generation is enabled in config. |

### Default tags and description

Two `[youtube]` fields in `~/.vidpolish/config.toml` control what goes on
every upload beyond the title:

```toml
default_tags        = ["golang", "screencast"]
description_template = "{{.Title}}\n\nvidpolish"
```

`default_tags` is merged with a fixed `vidpolish` tag (which is always
included) on every upload. `description_template` is rendered with Go's
`text/template`, given `{{.Title}}` as the resolved video title; if the
rendered result doesn't contain the word `vidpolish`, it's appended
automatically so that tag is always present even if you edit the template.

### Auto-generated thumbnails

vidpolish can generate a 1280x720 thumbnail for every upload: a brand
background, your logo in a corner, and the video title laid out with a
title-fitting pass that shrinks the font and wraps across up to three
lines for longer titles, truncating with an ellipsis only as a last
resort. It's built as an SVG (so it stays human-inspectable/tweakable) and
rasterized with [resvg](https://github.com/linebender/resvg).

Configure it under `[thumbnail]` in `~/.vidpolish/config.toml`:

```toml
[thumbnail]
enabled           = true
logo_path         = "/path/to/your/logo.svg"   # PNG/JPG also accepted; empty = no logo
background_color  = "#0f172a"
accent_color      = "#22d3ee"
text_color        = "#ffffff"
```

`logo_path` is entirely up to you; an SVG logo is rasterized automatically
(and cached) the first time it's used. With `[thumbnail].enabled = true`,
every `upload`/`process --upload` generates and sets a thumbnail unless
you pass `--thumbnail <path>` (use your own image) or `--no-thumbnail`
(skip it for this run). The generated PNG is saved next to the uploaded
video as `<name>-thumbnail.png`, along with its source `.svg`.

**Custom thumbnails require phone verification on your channel.** This is
a real YouTube API requirement, not a vidpolish limitation: if your
channel hasn't verified a phone number, `thumbnails.set` will fail and
vidpolish surfaces YouTube's own error text (which names the requirement
directly) rather than failing silently. Verify your channel from YouTube
Studio if you hit this. The video itself still uploads fine either way;
only the custom thumbnail step is affected.

**Platform note**: resvg auto-downloads on Linux and macOS (x86_64/arm64).
It doesn't publish a Windows or linux/arm64 binary, so on those platforms
`vidpolish deps`/thumbnail generation expects `resvg` to already be on
PATH (e.g. `cargo install resvg`, or a package manager). Everything else
in vidpolish works normally either way; thumbnail generation is the only
feature affected.

YouTube's own AI/suggested-thumbnail feature (in Studio) isn't reachable
through the public Data API, so vidpolish can't tap into it directly;
this generated-thumbnail approach is the alternative.

### About automatic captions

There is no API call that generates subtitles on demand. YouTube's
automatic captions are produced by YouTube's own backend once it finishes
processing the audio track. What vidpolish does control is
`defaultLanguage`/`defaultAudioLanguage` on the uploaded video (via
`--language` or the config default), which tells YouTube which language
model to use for auto-generated captions. Setting it correctly improves
caption accuracy and availability, but does not force captions to appear
on any particular schedule.

## Project layout

```
cmd/vidpolish        CLI entrypoint (flag parsing, wiring)
internal/binmgr       resolves/downloads deep-filter, auto-editor, resvg, and the Inter font
internal/cache        the ~/.vidpolish/cache artifact cache
internal/config       ~/.vidpolish/config.toml load/init/save
internal/pipeline     the split / denoise / remux / auto-edit stages
internal/thumbnail    SVG-based thumbnail generation, rasterized via resvg
internal/ytauth       YouTube OAuth 2.0 installed-app login flow
internal/ytupload     resumable YouTube upload with progress/ETA and thumbnail set
media/                logo assets
```

## Status and roadmap

vidpolish covers the local processing pipeline (split, denoise, cut,
optional speed changes, with caching) and uploading the result to YouTube
as unlisted-by-default, with progress, ETA, language metadata, default
tags/description, and an auto-generated thumbnail.

Planned next:

- A UI on top of the same library/CLI.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
