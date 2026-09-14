<p align="center">
  <img src="media/logo.svg" alt="vidpolish logo" width="140" height="140">
</p>

<h1 align="center">vidpolish</h1>

<p align="center">
  Turn a raw screen recording into a polished, clean, tightly cut video with one command.
</p>

---

## Contents

| I want to... | Go to |
| --- | --- |
| Understand why vidpolish exists | [The Problem](#the-problem) |
| Install it | [Install](#install) |
| See the UI before installing anything | [Screenshots](#screenshots) |
| See what the pipeline actually does | [What it does](#what-it-does) |
| Check required tools before running it | [Prerequisites](#prerequisites) |
| Run it from the command line | [Usage](#usage) |
| Understand how caching avoids repeat work | [How caching works](#how-caching-works) |
| Upload a finished video straight to YouTube | [YouTube upload](#youtube-upload) |
| Use the local Projects web UI | [Local UI](#local-ui) |
| Find my way around the source code | [Project layout](#project-layout) |
| See what's done and what's planned | [Status and roadmap](#status-and-roadmap) |
| Build from source or contribute | [Development](#development) |
| Check the license | [License](#license) |
| Find a related tool for code review | [See More](#see-more) |

## The Problem

Recording a quick Loom-style walkthrough is easy. Cleaning it up by hand is not.
You have to trim pauses and cut background hiss yourself.

A single "clean" pass is rarely enough, either:

- The cut you want for a fast internal update is not the cut you want for a
  public upload.
- Finding the right speed, resolution, and bitrate for a size target usually
  means re-exporting the file again and again, then checking the result each
  time.

vidpolish automates the cleanup (denoise, cut silence) in one command. It
also gives you a local **Projects** workspace for everything that comes
after:

- **One project per video.** Every iteration lives in one place, not in a
  folder full of `final_v3_REAL_final.mp4` files.
- **As many edit iterations as you want, on the same source.** Each one is
  its own pass with its own margin, speed, resize, and bitrate settings.
  Keep a "1.25x tight cut" and a "1080p archival copy" side by side. They
  share the same denoise/split work, so trying five variants does not mean
  five full re-runs.
- **Real numbers, before and after.** Resolution, duration, bitrate, frame
  rate, file size, and a percent-change comparison against the source. Dial
  an iteration in to hit a size or bitrate target instead of guessing.
- **Upload any iteration straight to YouTube.** Set a title, description,
  tags, and privacy per upload, with an auto-generated cover thumbnail.
  Picking "which cut goes out" is a one-click choice, not a re-export.
- **Export without YouTube.** Download any edit as an MP4, or export it as
  an animated GIF (fps and width configurable), for sharing anywhere else.
- **Notes live next to the video, not in a separate doc.** Markdown text
  cells hold links, timestamps, and checklists for a project. Type
  `@<seq>` to link straight to the exact source, edit, or upload cell you
  mean.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/shrsv/vidpolish/main/scripts/install.sh | bash
```

This script:

- Downloads the right prebuilt binary for your OS and architecture, from
  the [latest release](https://github.com/shrsv/vidpolish/releases/latest).
- Installs it to `~/.local/bin` and adds that to your `PATH`.
- Runs `vidpolish deps`, so ffmpeg is checked and the other tool
  dependencies (deep-filter, auto-editor, resvg, font) are fetched right
  away.

Supported platforms: Linux (amd64/arm64) and macOS (amd64/arm64). Windows
users can grab `vidpolish-windows-amd64.exe` directly from the releases
page.

Then either run it once:

```sh
vidpolish process <input.mp4>
```

or start the [local UI](#local-ui):

```sh
vidpolish ui
```

Building from source (`make build`, or `go install`) still works, if you'd
rather not use the installer.

## Screenshots

A quick tour of the local UI (`vidpolish ui`), the same "Projects" notebook
model described in [Local UI](#local-ui) below:

- One project per video.
- As many edit iterations on it as you want.
- Real before/after metrics, to hit a size or platform target.
- Upload cells that push any iteration straight to YouTube, with an
  auto-generated cover.

*The video preview in these shots is a generated color-bars/tone clip, not
a real recording. It plays the same role as an SMPTE test card: the
screenshots don't include anyone's actual footage.*

<table>
<tr><td width="33%">

**Projects list.** Every video you're working on is its own project.
Iterations on it live in one place, not in a folder full of
`final_v3_REAL_final.mp4` files. One click to open.

<img src="media/screenshots/01-projects-list.png" width="100%">

</td><td width="33%">

**Category quick-nav and collapse-all.** Jump between Source, Edit, and
Upload sections, or collapse every cell down to its header row.

<img src="media/screenshots/05-project-header.png" width="100%">

</td><td width="34%">

**Cache panel.** Shows which project a cache entry belongs to. Deleting an
entry never deletes the project itself.

<img src="media/screenshots/09-cache.png" width="100%">

</td></tr>
</table>

**A project's notebook view.** Try as many edit iterations on the same
source as you want (different speed, resize, or bitrate combos), side by
side, in one scrollable page. Drag to reorder using the grip handle. All
iterations share the same denoise/silence-cut work under the hood, so
trying five variants does not mean five full re-runs.

<img src="media/screenshots/02-project-view.png" width="100%">

**Collapsed cells.** A compact overview of a project, for when you don't
need every cell expanded.

<img src="media/screenshots/06-collapsed-cells.png" width="100%">

<table>
<tr><td width="50%">

**An edit cell at rest.** Every knob for one iteration: denoise and
silence-cut margin (with an inline explanation), speed presets, resize
(exact pixels, or a quick 25/50/75%/original scale, with an aspect-ratio
lock), bitrate, and a live pre-run size estimate. Aim an iteration at a
target file size before you even run it.

<img src="media/screenshots/03-edit-cell.png" width="100%">

</td><td width="50%">

**Tool status.** The same checks as `vidpolish deps`, with a re-check
button per tool.

<img src="media/screenshots/08-tools.png" width="100%">

</td></tr>
<tr><td width="50%">

**Live thumbnail preview.** Every upload cell gets a nice auto-generated
cover thumbnail for free. It updates live as you type the title, before
you've even run the upload.

<img src="media/screenshots/04-upload-cell-live-preview.png" width="100%">

</td><td width="50%">

**A finished upload.** The YouTube link appears as soon as it's known, in
a copyable box with a dedicated Copy button, not a bare link.

<img src="media/screenshots/04-upload-cell-done.png" width="100%">

</td></tr>
<tr><td width="50%">

**A finished edit cell.** The resolved video, a compact media-info summary
below it, Download, and the GIF export action, all next to "Add upload
from this edit" as the primary next step.

<img src="media/screenshots/10-edit-cell-done.png" width="100%">

</td><td width="50%">

**Media info, expanded.** The real numbers for hitting a distribution
target. Click the summary for full resolution, duration, bitrate, frame
rate, and file size, plus a percent-change comparison against the source.

<img src="media/screenshots/11-media-info-popover.png" width="100%">

</td></tr>
</table>

**Export as GIF.** Pick fps and width, saved through the same
save-file-picker flow as the video download.

<img src="media/screenshots/12-gif-export.png" width="100%">

**Notes.** Freeform markdown text cells for links, timestamps, and
checklists on a project. Type `@<seq>` to reference any other cell
(source, edit, upload, or text) as a clickable link. Done switches back to
the rendered view.

<img src="media/screenshots/13-notes.png" width="100%">

**Config panel.** YouTube credentials (secrets masked), upload defaults,
and thumbnail styling, with automatic backups before every save.

<img src="media/screenshots/07-config.png" width="100%">

## What it does

vidpolish takes a raw talking-head or screen recording and runs it through
a small pipeline:

1. **Split** the input into a video-only stream and an audio-only track.
2. **Denoise** the audio with [DeepFilterNet](https://github.com/Rikorose/DeepFilterNet)
   (the `deep-filter` CLI). This is the same kind of deep-learning noise
   removal behind tools like Adobe Podcast.
3. **Remux** the cleaned audio back onto the video.
4. **Auto-cut** silence and dead air with [auto-editor](https://github.com/WyattBlue/auto-editor).
   Optionally speed up the parts you kept.
5. **Resize/re-encode**, optional. This only runs if you ask for a
   different resolution or bitrate. Otherwise the auto-cut output is used
   as-is.

Every stage shells out to a well-tested external binary, instead of
reinventing audio/video processing in Go. vidpolish is the orchestrator:
it glues these tools together with sensible defaults, caching, and a
simple CLI.

## Prerequisites

You need **Go** and **ffmpeg** installed before building or running
vidpolish.

Everything else (`deep-filter`, `auto-editor`, `resvg`, the Inter font) is
fetched automatically the first time you run the tool. You do not need to
install those by hand.

Node.js is only needed if you're editing the local UI's frontend (see
[Local UI](#local-ui)). The built frontend is committed to the repo, so a
plain `go build` never requires Node.

| Tool | Why it's needed | How to get it |
| --- | --- | --- |
| [Go](https://go.dev/dl/) 1.24+ | builds and runs the CLI | `apt install golang-go`, `brew install go`, or the official installer |
| `ffmpeg` / `ffprobe` | splitting, remuxing, and probing video/audio | `apt install ffmpeg`, `brew install ffmpeg`, or download from [ffmpeg.org](https://ffmpeg.org/download.html) |
| `deep-filter` | audio denoising | downloaded automatically by vidpolish, from the [DeepFilterNet releases](https://github.com/Rikorose/DeepFilterNet/releases), into `~/.cache/vidpolish/bin` |
| `auto-editor` | silence cutting and speed changes | downloaded automatically by vidpolish, from the [auto-editor releases](https://github.com/WyattBlue/auto-editor/releases), into `~/.cache/vidpolish/bin` |
| `resvg` | rasterizing auto-generated YouTube thumbnails | downloaded automatically by vidpolish, from the [resvg releases](https://github.com/linebender/resvg/releases), into `~/.cache/vidpolish/bin` on Linux/macOS (see the platform note below for Windows/linux-arm64) |
| Inter font | text rendering inside generated thumbnails | downloaded automatically by vidpolish, from the [Inter releases](https://github.com/rsms/inter/releases), into `~/.cache/vidpolish/fonts` |

**Supported platforms** for the auto-downloaded `deep-filter`/`auto-editor`
binaries: Linux (x86_64, aarch64), macOS (Intel and Apple Silicon), and
Windows (x86_64).

`resvg` (used only for thumbnail generation) has narrower prebuilt
coverage:

- Linux x86_64 and macOS (Intel and Apple Silicon): auto-downloads.
- Windows and Linux aarch64: no official prebuilt binary. Install `resvg`
  yourself (for example `cargo install resvg`, or a package manager) and
  make sure it's on `PATH`.

Every other vidpolish feature works normally either way. Only thumbnail
generation is affected.

### Check and install everything in one go

Once Go and ffmpeg are on your machine, let vidpolish resolve the rest:

```sh
git clone git@github.com:shrsv/vidpolish.git
cd vidpolish
go build -o vidpolish ./cmd/vidpolish

./vidpolish deps
```

`vidpolish deps` prints the resolved path for each dependency and
downloads whatever is missing. Run it any time to double check your
setup:

```
ffmpeg       /usr/bin/ffmpeg
ffprobe      /usr/bin/ffprobe
deep-filter  /home/you/.cache/vidpolish/bin/deep-filter/0.5.6/deep-filter
auto-editor  /home/you/.cache/vidpolish/bin/auto-editor/31.6.0/auto-editor
resvg        /home/you/.cache/vidpolish/bin/resvg/0.48.1/resvg
font         /home/you/.cache/vidpolish/fonts/inter-4.1/Inter-Regular.ttf, /home/you/.cache/vidpolish/fonts/inter-4.1/Inter-Bold.ttf
```

If `ffmpeg`/`ffprobe` are missing, `deps` tells you to install them. It
will not try to fetch those itself, since they are available through
almost any system package manager already.

## Usage

The basic case: denoise and trim silence, with default settings.

```sh
./vidpolish process path/to/recording.mp4
# -> output/recording-polished.mp4
```

Give the kept speech more breathing room, if cuts feel too tight (default
is `0.2s`):

```sh
./vidpolish process --margin 0.3s path/to/recording.mp4
```

Speed up the parts you kept, for example to tighten a long walkthrough.
Silence is still fully cut, not just sped through.

```sh
./vidpolish process --speed 1.25 path/to/recording.mp4
./vidpolish process --speed 1.5  path/to/recording.mp4
./vidpolish process --speed 1.75 path/to/recording.mp4
```

Send the result somewhere other than `./output`:

```sh
./vidpolish process --output-dir ~/Desktop/clips path/to/recording.mp4
```

Force a full recompute, even if a cached run already exists for this file:

```sh
./vidpolish process --no-cache path/to/recording.mp4
```

Flags can go before or after the input path. Both of these work the same
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
| `--width` | original | Resize output to this width in pixels. Pair with `--height`, or leave it out to scale proportionally. |
| `--height` | original | Resize output to this height in pixels. Pair with `--width`, or leave it out to scale proportionally. |
| `--bitrate` | original | Target video bitrate in kbps. Leaving it unset keeps the original encode quality (no bitrate-driven re-encode). |
| `--output-dir` | `output` | Directory the final file is written to. |
| `--no-cache` | off | Ignore cached intermediate artifacts and recompute everything. |

Leaving `--width`/`--height`/`--bitrate` unset skips the resize/re-encode
pass entirely. The auto-edited output is used as-is, at its original
resolution and bitrate.

### Cache commands

```sh
./vidpolish cache clean   # remove all cached pipeline artifacts right now
```

## How caching works

The expensive parts of the pipeline (splitting, denoising, remuxing) do
not depend on `--margin` or `--speed`. They depend only on the source
file.

vidpolish caches those intermediate artifacts at
`~/.vidpolish/cache/<fingerprint>/`, keyed by a fast content fingerprint
of the input (its size, modification time, and sampled head/tail bytes,
not a full-file hash). Re-running `process` on the same file with a
different `--speed` or `--margin` reuses that cached work. Only the fast
final cut re-runs, instead of redoing several minutes of denoising.

Cache entries are kept for 7 days and swept lazily on each run. You can
also purge them immediately with `vidpolish cache clean`. This is a
separate cache from `~/.cache/vidpolish/bin`, which only holds the
downloaded tool binaries themselves.

**Your source file is never modified.** Every stage reads the input and
writes to a separate cache/output path. Nothing in the pipeline writes
back to the file you pass to `process`.

## YouTube upload

vidpolish can upload the polished result straight to YouTube. By default
it uploads as an unlisted video, with progress and an ETA printed while it
uploads.

### One-time setup in Google Cloud Console

Uploading is a write operation on your channel, so it needs OAuth 2.0
consent, not just an API key:

1. Create (or pick) a project at [console.cloud.google.com](https://console.cloud.google.com/).
   Enable the **YouTube Data API v3** under APIs & Services.
2. Under APIs & Services > Credentials, create an **OAuth client ID** of
   type **Desktop app**. Note the client ID and client secret.
3. If your project's OAuth consent screen is still in testing mode, add
   your own Google account as a test user. Otherwise login gets rejected.

### Configure and log in

```sh
./vidpolish config init
```

This creates `~/.vidpolish/config.toml`. Open it and fill in `client_id`
and `client_secret` from step 2 above. Adjust `privacy`/`default_language`
too, if you want different defaults. Then:

```sh
./vidpolish youtube login
```

This opens a browser for a one-time consent screen, and stores a refresh
token back into the config file. You do not need to repeat this for
future uploads, only if you revoke access or move to a new machine.

If your Google Cloud project's OAuth consent screen is in testing mode,
add your own account under Test users first, or the consent step will be
rejected.

On a headless machine, or inside WSL, `vidpolish youtube login` cannot
open a browser for you. It prints the authorization URL instead, so you
can open it yourself (on Windows, if you're in WSL). It listens on a
local port for up to 5 minutes, waiting for you to approve it.

### Uploading

```sh
./vidpolish upload path/to/polished.mp4
./vidpolish upload path/to/polished.mp4 --title "Walkthrough" --privacy public --language en-IN
```

Or do the whole thing in one command: polish, then upload.

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

The link is valid and shareable the moment it prints. You do not need to
wait for the "fully processed" line; that part only keeps polling YouTube
(for up to about two minutes) so you can see when it finishes
transcoding. Skip that wait entirely with `--no-wait`: the command returns
right after the `uploaded:` line.

By default, the description and tags contain just `vidpolish`. See the
next section to set your own defaults. Fill in anything else you want
from YouTube Studio afterward.

### `upload` / `process --upload` flags

| Flag | Default | Description |
| --- | --- | --- |
| `--title` | input filename | Video title. |
| `--privacy` | from config (`unlisted`) | `public`, `unlisted`, or `private`. |
| `--language` | from config (`en`) | BCP-47 language code, e.g. `en`, `en-IN`. |
| `--no-wait` | off | Return right after the upload finishes, instead of also waiting on YouTube's processing status. |
| `--thumbnail <path>` | none | Use a specific pre-made image as the thumbnail, instead of auto-generating one. |
| `--no-thumbnail` | off | Don't set a thumbnail for this upload. Generation is on by default otherwise. |

### Default tags and description

Two `[youtube]` fields in `~/.vidpolish/config.toml` control what goes on
every upload beyond the title:

```toml
default_tags        = ["golang", "screencast"]
description_template = "{{.Title}}\n\nvidpolish"
```

- `default_tags` is merged with a fixed `vidpolish` tag, which is always
  included, on every upload.
- `description_template` is rendered with Go's `text/template`, given
  `{{.Title}}` as the resolved video title. If the rendered result does
  not contain the word `vidpolish`, it's appended automatically, so that
  tag is always present even if you edit the template.

### Auto-generated thumbnails

**Enabled by default.** Every `upload`/`process --upload` generates a
1280x720 thumbnail and sets it on the video automatically:

- A brand background.
- Your logo in a corner, if you configure one.
- The video title, laid out with a title-fitting pass. It shrinks the font
  and wraps across up to three lines for longer titles, truncating with an
  ellipsis only as a last resort.

You don't need to turn anything on for this; it just happens as part of
`upload`. It's built as an SVG, so it stays human-inspectable and
tweakable, and rasterized with [resvg](https://github.com/linebender/resvg).

Skip it for a single run with `--no-thumbnail`, or use your own pre-made
image with `--thumbnail <path>` instead of generating one. To turn it off
entirely, set `enabled = false` under `[thumbnail]` in
`~/.vidpolish/config.toml`. Leaving `[thumbnail]` out of the config
entirely (or `vidpolish config init`'s freshly generated file) keeps it
on, since on is the default either way.

Configure it under `[thumbnail]` in `~/.vidpolish/config.toml`:

```toml
[thumbnail]
enabled           = true   # generation is on by default even without this line
logo_path         = "/path/to/your/logo.svg"   # PNG/JPG also accepted; empty = no logo
background_color  = "#0f172a"
accent_color      = "#22d3ee"
text_color        = "#ffffff"
```

`logo_path` is entirely up to you. An SVG logo is rasterized automatically
(and cached) the first time it's used. The generated PNG is saved next to
the uploaded video as `<name>-thumbnail.png`, along with its source
`.svg`, so you can see exactly what got set and hand-edit the SVG if you
want.

**Custom thumbnails require phone verification on your channel.** This is
a real YouTube API requirement, not a vidpolish limitation. If your
channel has not verified a phone number, `thumbnails.set` will fail, and
vidpolish surfaces YouTube's own error text (which names the requirement
directly) rather than failing silently. Verify your channel from YouTube
Studio if you hit this. The video itself still uploads fine either way;
only the custom thumbnail step is affected.

See the Prerequisites table above for resvg's platform coverage and the
manual-install fallback on Windows/linux-arm64.

YouTube's own AI/suggested-thumbnail feature (in Studio) is not reachable
through the public Data API, so vidpolish can't tap into it directly.
This generated-thumbnail approach is the alternative.

### About automatic captions

There is no API call that generates subtitles on demand. YouTube's
automatic captions are produced by YouTube's own backend, once it
finishes processing the audio track.

What vidpolish does control is `defaultLanguage`/`defaultAudioLanguage` on
the uploaded video (via `--language` or the config default). This tells
YouTube which language model to use for auto-generated captions. Setting
it correctly improves caption accuracy and availability, but does not
force captions to appear on any particular schedule.

## Local UI

```sh
./vidpolish ui
```

This starts a small local web app, embedded in the `vidpolish` binary
itself (no separate server or process to run). It opens in your browser
at `http://127.0.0.1:7890` (`--port` to change it, `--no-open` to skip
launching a browser). It binds to `127.0.0.1` only and has no login page
of its own: the same local-machine trust model as running the CLI.

### The "Projects" model

The UI is organized around **Projects**, notebook-style. Each project is
an ordered list of **cells**:

- **Source cell** (`#1`, always first). Drag a video in, or use the file
  picker. Created automatically with the project.
- **Edit cells.** Denoise, plus cut at a chosen margin (with an inline
  explanation) and speed (1x/1.25x/1.5x/1.75x/2x presets, or type your
  own). Optional resize (exact pixels, or a quick 25/50/75%/original
  scale, aspect-locked by default) and bitrate overrides. Leave a field
  blank and it stays at the source's original value; a rough size
  estimate updates live as you change them.

  Each edit cell is its own named, independently runnable cell (for
  example "1.25x draft" or "slow calm cut"). Add as many as you want, at
  different speeds. They share the same underlying denoise/split work as
  `process` uses, so trying five speeds does not denoise five times, and
  running several at once is safe: the shared stage is serialized
  per-source internally, and the fast parts run in parallel.

  Once a cell has run, a compact resolution/duration/bitrate/size summary
  sits under the video. Click it for the full breakdown, plus a
  comparison against the source. Download the result, or export it as a
  GIF (fps/width configurable), right alongside it.
- **Upload cells.** Pick which edit cell's output to push to YouTube. Set
  title, description, tags, and privacy per cell, then run it. Multiple
  upload cells can target the same or different edit cells, and run
  concurrently.
- **Text cells ("Notes").** Freeform markdown for links, timestamps, and
  checklists about the video: headings, bold/italic, lists, and links
  render straight away. An Edit button switches to the raw markdown, and
  Done saves and switches back. Type `@<seq>` (e.g. `@2`) to reference any
  other cell in the project (source, edit, upload, or another note) as a
  clickable link. An "Insert reference" picker in edit mode lists every
  cell, so you don't need to remember its number.

Every cell has a stable number (`#2`, `#3`, ...), assigned once and never
reused, even if you reorder or delete other cells. It also has an
optional custom name you can set any time. Both work as a way to refer
back to that cell.

Running cells stream live progress (the same stage/status lines the CLI
prints) on the page in real time. Edit-cell and upload results (a video
player, a clickable YouTube link) appear as soon as they're ready, link
included, before YouTube finishes processing, the same as the CLI.

### Config, tools, and cache panels

- **Config tab.** Manages `~/.vidpolish/config.toml` from the browser:
  YouTube credentials (the client secret and refresh token are never sent
  back to the browser in full, only a short preview, so the page is safe
  to leave open), upload defaults, and thumbnail settings, plus a "Connect
  YouTube" button that runs the same OAuth flow as `vidpolish youtube
  login`.
- **Tools tab.** Shows the same resolution status as `vidpolish deps`,
  with a re-check button per tool.
- **Cache tab.** Lists `~/.vidpolish/cache` entries with size and age.
  Delete one, or clean all expired entries at once, the same as
  `vidpolish cache clean`.

### Developing the UI itself

The frontend lives in `web/` (Preact + Vite + Tailwind) and builds into
`internal/server/webdist/`, which is committed to the repo and embedded
via `go:embed`. Building the `vidpolish` binary never requires Node; only
editing the UI does.

```sh
cd web
npm install
npm run dev     # Vite dev server with hot reload, proxies /api to :7890
npm run build   # writes internal/server/webdist for `go build` to embed
```

## Project layout

```
cmd/vidpolish         CLI entrypoint (flag parsing, wiring)
internal/binmgr        resolves/downloads deep-filter, auto-editor, resvg, and the Inter font
internal/browseropen   opens a URL in the default browser
internal/cache         the ~/.vidpolish/cache artifact cache
internal/config        ~/.vidpolish/config.toml load/init/save
internal/pipeline      the split / denoise / remux / auto-edit stages
internal/server        the embedded local UI's HTTP API and static frontend
internal/store         SQLite-backed Projects/Cells storage (~/.vidpolish/vidpolish.db)
internal/thumbnail     SVG-based thumbnail generation, rasterized via resvg
internal/ytauth        YouTube OAuth 2.0 installed-app login flow
internal/ytmeta        shared tag/description building for uploads
internal/ytupload      resumable YouTube upload with progress/ETA and thumbnail set
media/                 logo assets
web/                   Preact/Vite source for the local UI
```

## Status and roadmap

vidpolish currently covers:

- The **local processing pipeline**: split, denoise, cut, optional speed
  changes, with caching.
- **Uploading the result to YouTube**: unlisted by default, with
  progress, ETA, language metadata, default tags/description, and an
  auto-generated thumbnail.
- A local **"Projects" UI** (`vidpolish ui`), built on the same library,
  with config/tools/cache management panels.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

## License

MIT. See [LICENSE](LICENSE).

---

## See More

Your team's review attention is limited. Spend it where **business risk is
highest**, not spread evenly across every diff.

vidpolish automates cleanup on your *video*. [**LiveReview**](https://hexmos.com/livereview)
does the analogous thing for your *code changes*: instead of reviewing
every diff with equal effort, it scores each change by blast radius (how
far its impact reaches through your call graph), so review attention goes
where it actually matters.

[![LiveReview: Blast-Radius Aware AI Code Review for Business-Critical Systems](media/livereview-banner.png)](https://hexmos.com/livereview)
