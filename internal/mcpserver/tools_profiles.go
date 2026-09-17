package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
	server "vidpolish/internal/server"
	"vidpolish/internal/store"
)

// profileResponse mirrors the JSON shape internal/server's REST API returns
// for a profile.
type profileResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Params    any    `json:"params"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func profileToResponse(p *store.Profile) *profileResponse {
	r := &profileResponse{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt.Unix(), UpdatedAt: p.UpdatedAt.Unix()}
	var params any
	if err := json.Unmarshal([]byte(p.ParamsJSON), &params); err == nil {
		r.Params = params
	}
	return r
}

// sourceResolutionFor returns the probed width/height of cell's ultimate
// source video (its parent's output for an edit cell), or 0,0 if it isn't
// probeable yet (no output, or ffprobe unavailable) — callers treat that as
// "unknown" and fall back to "keep original" behavior.
func sourceResolutionFor(db *store.DB, cell *store.Cell) (int, int) {
	if cell.Kind != store.KindEdit || cell.ParentCellID == nil {
		return 0, 0
	}
	parent, err := db.GetCell(*cell.ParentCellID)
	if err != nil || parent.OutputPath == nil || *parent.OutputPath == "" {
		return 0, 0
	}
	ffprobePath, err := binmgr.Resolve(binmgr.FFprobe)
	if err != nil {
		return 0, 0
	}
	info, err := pipeline.ProbeVideoInfo(ffprobePath, *parent.OutputPath)
	if err != nil {
		return 0, 0
	}
	return info.Width, info.Height
}

// addProfileTools registers CRUD + apply tools for named, saved edit-cell
// param profiles (see server.ProfileParams). Profiles are global, not
// scoped to a project.
func (s *Server) addProfileTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_list_profiles",
		Description: "List every saved edit profile (a named, reusable set of edit-cell params, with resize stored as a resolution-independent scale percentage).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		profiles, err := s.db.ListProfiles()
		if err != nil {
			return toolError(err)
		}
		out := make([]*profileResponse, 0, len(profiles))
		for _, p := range profiles {
			out = append(out, profileToResponse(p))
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_save_profile",
		Description: "Save a named edit profile. Either pass cellId to derive the profile from an existing edit " +
			"cell's current params (its resize, if any, is converted to a percentage of its source resolution), " +
			"or pass params directly as {margin, speed, scalePct, bitrateKbps, lockAspect, skipDenoise}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name   string          `json:"name"`
		CellID string          `json:"cellId,omitempty"`
		Params json.RawMessage `json:"params,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		var pp server.ProfileParams
		switch {
		case args.CellID != "":
			cell, err := s.db.GetCell(args.CellID)
			if err != nil {
				return toolError(err)
			}
			if cell.Kind != store.KindEdit {
				return toolError(errf("cellId must refer to an edit cell"))
			}
			var ep server.EditParams
			if err := json.Unmarshal([]byte(cell.ParamsJSON), &ep); err != nil {
				return toolError(errf("parsing cell params: %w", err))
			}
			srcW, srcH := sourceResolutionFor(s.db, cell)
			pp = server.DeriveProfileParams(ep, srcW, srcH)
		case len(args.Params) > 0:
			if err := json.Unmarshal(args.Params, &pp); err != nil {
				return toolError(errf("parsing params: %w", err))
			}
		default:
			return toolError(errf("either cellId or params is required"))
		}
		paramsJSON, err := json.Marshal(pp)
		if err != nil {
			return toolError(err)
		}
		p, err := s.db.CreateProfile(args.Name, string(paramsJSON))
		if err != nil {
			return toolError(err)
		}
		return nil, profileToResponse(p), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_apply_profile",
		Description: "Apply a saved edit profile to an edit cell, resolving its scale percentage against the cell's source resolution and overwriting the cell's params.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProfileID string `json:"profileId"`
		CellID    string `json:"cellId"`
	}) (*mcp.CallToolResult, any, error) {
		profile, err := s.db.GetProfile(args.ProfileID)
		if err != nil {
			return toolError(err)
		}
		cell, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		if cell.Kind != store.KindEdit {
			return toolError(errf("cellId must refer to an edit cell"))
		}
		if cell.Status == store.StatusRunning {
			return toolError(errf("cannot edit a running cell"))
		}
		var pp server.ProfileParams
		if err := json.Unmarshal([]byte(profile.ParamsJSON), &pp); err != nil {
			return toolError(errf("parsing profile params: %w", err))
		}
		srcW, srcH := sourceResolutionFor(s.db, cell)
		ep := server.ResolveProfile(pp, srcW, srcH)
		epJSON, err := json.Marshal(ep)
		if err != nil {
			return toolError(err)
		}
		if err := s.db.UpdateCellParams(args.CellID, string(epJSON)); err != nil {
			return toolError(err)
		}
		c, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		return nil, cellToResponse(c), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_delete_profile",
		Description: "Delete a saved edit profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProfileID string `json:"profileId"`
	}) (*mcp.CallToolResult, any, error) {
		if err := s.db.DeleteProfile(args.ProfileID); err != nil {
			return toolError(err)
		}
		return nil, map[string]bool{"deleted": true}, nil
	})
}
