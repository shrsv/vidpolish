package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
)

type processArgs struct {
	Input     string  `json:"input" jsonschema:"path to the raw input video"`
	OutputDir string  `json:"outputDir,omitempty" jsonschema:"directory the polished video is written to; defaults to the input's own directory"`
	Margin    string  `json:"margin,omitempty" jsonschema:"auto-editor --margin value, e.g. '0.2s'; defaults to 0.2s"`
	Speed     float64 `json:"speed,omitempty" jsonschema:"playback speed multiplier for kept/spoken segments; defaults to 1.0"`

	Width       int `json:"width,omitempty" jsonschema:"resize output to this width in pixels; 0 keeps the original"`
	Height      int `json:"height,omitempty" jsonschema:"resize output to this height in pixels; 0 keeps the original"`
	BitrateKbps int `json:"bitrateKbps,omitempty" jsonschema:"re-encode at this bitrate in kbps; 0 keeps the original"`

	NoCache bool `json:"noCache,omitempty" jsonschema:"ignore any existing cache entries and recompute every stage"`
}

type processResult struct {
	OutputPath string `json:"outputPath"`
}

func (s *Server) addPipelineTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_process",
		Description: "Run the full vidpolish pipeline (split audio/video, denoise with DeepFilterNet, " +
			"cut silence with auto-editor, and optionally resize/re-encode) on a raw recording, writing " +
			"the polished result to outputDir (default: alongside the input) as <name>-polished.mp4. " +
			"This can take from tens of seconds to several minutes depending on video length; progress " +
			"is reported via MCP progress notifications if the client requested one.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args processArgs) (*mcp.CallToolResult, any, error) {
		outputDir := args.OutputDir
		if outputDir == "" {
			var err error
			outputDir, err = outputDirFor(args.Input)
			if err != nil {
				return toolError(err)
			}
		}
		out, err := pipeline.Process(pipeline.Options{
			Input:       args.Input,
			OutputDir:   outputDir,
			Margin:      args.Margin,
			Speed:       args.Speed,
			Width:       args.Width,
			Height:      args.Height,
			BitrateKbps: args.BitrateKbps,
			Denoise:     true,
			NoCache:     args.NoCache,
			Log:         progressLogger(ctx, req),
		})
		if err != nil {
			return toolError(err)
		}
		return nil, processResult{OutputPath: out}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_probe",
		Description: "Probe a video file's resolution, duration, bitrate, frame rate, and file size.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Path string `json:"path" jsonschema:"path to a video file"`
	}) (*mcp.CallToolResult, any, error) {
		ffprobePath, err := binmgr.Resolve(binmgr.FFprobe)
		if err != nil {
			return toolError(err)
		}
		info, err := pipeline.ProbeVideoInfo(ffprobePath, args.Path)
		if err != nil {
			return toolError(err)
		}
		return nil, info, nil
	})
}
