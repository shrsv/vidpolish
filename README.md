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
go run ./cmd/vidpolish deps                              # resolve/download dependencies
go run ./cmd/vidpolish process path/to/input.mp4          # -> output/input-polished.mp4
go run ./cmd/vidpolish process --output-dir out --keep-temp path/to/input.mp4
```

## Layout

- `cmd/vidpolish` — CLI entrypoint.
- `internal/binmgr` — resolves/downloads the external tool binaries.
- `internal/pipeline` — the split/denoise/auto-edit stages, each shelling out
  to the resolved binaries.

## Status

Phase 1: local CLI pipeline only. YouTube upload/metadata integration and a
UI are planned as follow-ups.
