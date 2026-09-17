package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProfileSaveListApplyDeleteFlow(t *testing.T) {
	_, ts := newTestServer(t)

	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	projectID := proj["id"].(string)
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	_, editResp := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
		"kind": "edit", "parentCellId": sourceID,
		"params": map[string]any{"margin": "0.3s", "speed": 1.5, "width": 960, "height": 540, "bitrateKbps": 2000},
	})
	editID := editResp["id"].(string)

	// Save as a profile. No source video is probeable in this test, so the
	// resize can't be expressed as a percentage and comes back as 0 (i.e.
	// "keep original") — this exercises the "unknown resolution" fallback,
	// not the full probe round trip (covered by TestDeriveProfileParamsRoundTrip).
	resp, saved := doJSON(t, http.MethodPost, ts.URL+"/api/profiles", map[string]any{
		"name": "my-profile", "cellId": editID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("save profile status = %d, body = %v", resp.StatusCode, saved)
	}
	if saved["name"] != "my-profile" {
		t.Fatalf("name = %v", saved["name"])
	}
	params := saved["params"].(map[string]any)
	if params["margin"] != "0.3s" || params["speed"] != 1.5 || params["bitrateKbps"] != float64(2000) {
		t.Fatalf("saved profile params = %v", params)
	}
	profileID := saved["id"].(string)

	// Saving with a duplicate name is rejected.
	resp2, _ := doJSON(t, http.MethodPost, ts.URL+"/api/profiles", map[string]any{
		"name": "my-profile", "cellId": editID,
	})
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate save status = %d, want 409", resp2.StatusCode)
	}

	// List should show it.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/profiles", nil)
	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer listResp.Body.Close()
	var list []map[string]any
	json.NewDecoder(listResp.Body).Decode(&list)
	if len(list) != 1 || list[0]["id"] != profileID {
		t.Fatalf("list = %v", list)
	}

	// Apply it to a second, fresh edit cell.
	_, editResp2 := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
		"kind": "edit", "parentCellId": sourceID, "params": map[string]any{},
	})
	editID2 := editResp2["id"].(string)

	resp3, applied := doJSON(t, http.MethodPost, ts.URL+"/api/cells/"+editID2+"/apply-profile", map[string]any{
		"profileId": profileID,
	})
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("apply status = %d, body = %v", resp3.StatusCode, applied)
	}
	appliedParams := applied["params"].(map[string]any)
	if appliedParams["margin"] != "0.3s" || appliedParams["speed"] != 1.5 {
		t.Fatalf("applied params = %v", appliedParams)
	}

	// Delete it.
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/profiles/"+profileID, nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", delResp.StatusCode)
	}

	resp4, applyAfterDelete := doJSON(t, http.MethodPost, ts.URL+"/api/cells/"+editID2+"/apply-profile", map[string]any{
		"profileId": profileID,
	})
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("apply after delete status = %d, body = %v, want 404", resp4.StatusCode, applyAfterDelete)
	}
}

func TestPatchProfileRenameAndUpdateParams(t *testing.T) {
	_, ts := newTestServer(t)

	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)
	_, editResp := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+proj["id"].(string)+"/cells", map[string]any{
		"kind": "edit", "parentCellId": sourceID,
		"params": map[string]any{"margin": "0.2s", "speed": 1.0},
	})
	_, saved := doJSON(t, http.MethodPost, ts.URL+"/api/profiles", map[string]any{
		"name": "original-name", "cellId": editResp["id"].(string),
	})
	profileID := saved["id"].(string)

	// Rename only: params must survive unchanged.
	resp, renamed := doJSON(t, http.MethodPatch, ts.URL+"/api/profiles/"+profileID, map[string]any{
		"name": "renamed",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d, body = %v", resp.StatusCode, renamed)
	}
	if renamed["name"] != "renamed" {
		t.Fatalf("name = %v", renamed["name"])
	}
	renamedParams := renamed["params"].(map[string]any)
	if renamedParams["margin"] != "0.2s" {
		t.Fatalf("params should be unchanged after rename-only patch: %v", renamedParams)
	}

	// Params-only update: name must survive unchanged.
	resp2, updated := doJSON(t, http.MethodPatch, ts.URL+"/api/profiles/"+profileID, map[string]any{
		"params": map[string]any{"margin": "0.5s", "speed": 2.0, "scalePct": 75, "bitrateKbps": 1000, "lockAspect": true, "skipDenoise": true},
	})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("params update status = %d, body = %v", resp2.StatusCode, updated)
	}
	if updated["name"] != "renamed" {
		t.Fatalf("name should survive a params-only patch: %v", updated["name"])
	}
	updatedParams := updated["params"].(map[string]any)
	if updatedParams["margin"] != "0.5s" || updatedParams["scalePct"] != float64(75) {
		t.Fatalf("params not updated: %v", updatedParams)
	}
}
