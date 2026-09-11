# vidpolish

Turns a raw screen recording into a polished video:

1. Split the input into video and audio.
2. Denoise the audio with [DeepFilterNet](https://github.com/Rikorose/DeepFilterNet) (`deep-filter`).
3. Remux the cleaned audio back with the video.
4. Cut silence/dead air with [auto-editor](https://github.com/WyattBlue/auto-editor).

`ffmpeg`/`ffprobe` must already be on your `PATH`. `deep-filter` and
`auto-editor` are downloaded automatically (pinned per-platform release
binaries) into `~/.cache/vidpolish/bin` on first use.

## Usage

```sh
go run ./cmd/vidpolish deps                                    # resolve/download dependencies
go run ./cmd/vidpolish process path/to/input.mp4                # -> output/input-polished.mp4
go run ./cmd/vidpolish process --margin 0.3s path/to/input.mp4  # more breathing room around speech
go run ./cmd/vidpolish process --speed 1.5 path/to/input.mp4    # 1.5x playback on kept speech
go run ./cmd/vidpolish process --output-dir out --no-cache path/to/input.mp4
go run ./cmd/vidpolish cache clean                              # purge all cached artifacts now
```

Flags on `process`:

- `--margin` (default `0.2s`) — padding kept around detected speech before a
  cut; raise it (e.g. `0.3s`) if cuts feel too tight/words get clipped.
- `--speed` (default `1.0`) — speed multiplier applied to kept/spoken
  segments only (silence is still cut, not just sped up); e.g. `1.25`,
  `1.5`, `1.75`. Must be between 0.5 and 4.0.
- `--output-dir` (default `output`).
- `--no-cache` — ignore any cached intermediate artifacts for this run.

## Caching

Intermediate artifacts (split video/audio, denoised audio, the remuxed
video) are cached at `~/.vidpolish/cache/<fingerprint>/`, keyed by a fast
content fingerprint of the input file (size + mtime + sampled head/tail
bytes — not a full-file hash). Re-running `process` on the same input with
different `--margin`/`--speed` reuses the cached split/denoise/remux output
and only re-runs the final auto-editor pass. Entries are kept for 7 days
(swept lazily on each `process` run) or removed immediately with
`vidpolish cache clean`.

This is separate from `~/.cache/vidpolish/bin`, which caches the downloaded
`deep-filter`/`auto-editor` tool binaries themselves, not per-video
artifacts.

## Layout

- `cmd/vidpolish` — CLI entrypoint.
- `internal/binmgr` — resolves/downloads the external tool binaries.
- `internal/cache` — the `~/.vidpolish/cache` pipeline-artifact cache.
- `internal/pipeline` — the split/denoise/auto-edit stages, each shelling out
  to the resolved binaries, reusing cached artifacts where possible.

## Status

Phase 2: local CLI pipeline with tunable margin/speed and artifact caching.
YouTube upload/metadata integration and a UI are planned as follow-ups.
