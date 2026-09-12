package store

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateProjectAlsoCreatesSourceCell(t *testing.T) {
	db := openTestDB(t)

	p, source, err := db.CreateProject("My Project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected a generated project ID")
	}
	if source.Kind != KindSource || source.Seq != 1 {
		t.Fatalf("source cell = %+v, want kind=source seq=1", source)
	}
	if source.ParentCellID != nil {
		t.Fatalf("source cell should have no parent, got %v", source.ParentCellID)
	}
	if source.DisplayName() != "Source" {
		t.Fatalf("DisplayName = %q, want Source", source.DisplayName())
	}

	got, err := db.GetProject(p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "My Project" {
		t.Fatalf("Name = %q", got.Name)
	}

	cells, err := db.ListCellsByProject(p.ID)
	if err != nil {
		t.Fatalf("ListCellsByProject: %v", err)
	}
	if len(cells) != 1 || cells[0].ID != source.ID {
		t.Fatalf("expected exactly the source cell, got %+v", cells)
	}

	list, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d projects, want 1", len(list))
	}

	if err := db.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := db.GetProject(p.ID); err == nil {
		t.Fatal("expected error getting deleted project")
	}
}

func TestGetProjectMissingErrors(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetProject("nonexistent"); err == nil {
		t.Fatal("expected error for missing project")
	}
}

func TestSetSourceCellVideo(t *testing.T) {
	db := openTestDB(t)
	_, source, err := db.CreateProject("P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if err := db.SetSourceCellVideo(source.ID, "/tmp/video.mp4", "video.mp4"); err != nil {
		t.Fatalf("SetSourceCellVideo: %v", err)
	}
	got, err := db.GetCell(source.ID)
	if err != nil {
		t.Fatalf("GetCell: %v", err)
	}
	if got.OutputPath == nil || *got.OutputPath != "/tmp/video.mp4" {
		t.Fatalf("OutputPath = %v", got.OutputPath)
	}
	if got.SourceFilename == nil || *got.SourceFilename != "video.mp4" {
		t.Fatalf("SourceFilename = %v", got.SourceFilename)
	}
	if got.Status != StatusDone {
		t.Fatalf("Status = %q, want done", got.Status)
	}
}

func TestNextSeqIncrementsAndNeverReuses(t *testing.T) {
	db := openTestDB(t)
	p, _, err := db.CreateProject("P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	seq2, err := db.NextSeq(p.ID)
	if err != nil {
		t.Fatalf("NextSeq: %v", err)
	}
	if seq2 != 2 {
		t.Fatalf("first NextSeq after source cell (seq 1) = %d, want 2", seq2)
	}

	seq3, err := db.NextSeq(p.ID)
	if err != nil {
		t.Fatalf("NextSeq: %v", err)
	}
	if seq3 != 3 {
		t.Fatalf("second NextSeq = %d, want 3", seq3)
	}
}

func TestCellCRUDAndCascadeDelete(t *testing.T) {
	db := openTestDB(t)
	p, source, err := db.CreateProject("P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	seq, err := db.NextSeq(p.ID)
	if err != nil {
		t.Fatalf("NextSeq: %v", err)
	}
	editCell, err := db.CreateCell(p.ID, seq, source.ID, nil, KindEdit, `{"speed":1.25}`, 1)
	if err != nil {
		t.Fatalf("CreateCell (edit): %v", err)
	}
	if editCell.Status != StatusIdle {
		t.Fatalf("initial status = %q, want idle", editCell.Status)
	}
	if editCell.DisplayName() != "Edit #2" {
		t.Fatalf("DisplayName = %q, want 'Edit #2'", editCell.DisplayName())
	}

	uploadSeq, err := db.NextSeq(p.ID)
	if err != nil {
		t.Fatalf("NextSeq: %v", err)
	}
	customName := "Upload draft"
	uploadCell, err := db.CreateCell(p.ID, uploadSeq, editCell.ID, &customName, KindUpload, `{"title":"x"}`, 2)
	if err != nil {
		t.Fatalf("CreateCell (upload): %v", err)
	}
	if uploadCell.ParentCellID == nil || *uploadCell.ParentCellID != editCell.ID {
		t.Fatalf("ParentCellID = %v, want %q", uploadCell.ParentCellID, editCell.ID)
	}
	if uploadCell.DisplayName() != "Upload draft" {
		t.Fatalf("DisplayName = %q, want custom name", uploadCell.DisplayName())
	}

	// Referring by seq or by name should both resolve.
	bySeq, err := db.GetCellByRef(p.ID, "2")
	if err != nil || bySeq.ID != editCell.ID {
		t.Fatalf("GetCellByRef(2) = %+v, %v", bySeq, err)
	}
	byName, err := db.GetCellByRef(p.ID, "upload draft")
	if err != nil || byName.ID != uploadCell.ID {
		t.Fatalf("GetCellByRef(upload draft) = %+v, %v", byName, err)
	}

	cells, err := db.ListCellsByProject(p.ID)
	if err != nil {
		t.Fatalf("ListCellsByProject: %v", err)
	}
	if len(cells) != 3 || cells[0].ID != source.ID || cells[1].ID != editCell.ID || cells[2].ID != uploadCell.ID {
		t.Fatalf("cells not in position order: %+v", cells)
	}

	if err := db.UpdateCellStatus(editCell.ID, StatusRunning, "denoising..."); err != nil {
		t.Fatalf("UpdateCellStatus: %v", err)
	}
	got, _ := db.GetCell(editCell.ID)
	if got.Status != StatusRunning || got.StatusMessage == nil || *got.StatusMessage != "denoising..." {
		t.Fatalf("status not updated: %+v", got)
	}

	if err := db.SetCellOutput(editCell.ID, "/tmp/out.mp4"); err != nil {
		t.Fatalf("SetCellOutput: %v", err)
	}
	got, _ = db.GetCell(editCell.ID)
	if got.Status != StatusDone || got.OutputPath == nil || *got.OutputPath != "/tmp/out.mp4" {
		t.Fatalf("output not set: %+v", got)
	}

	if err := db.SetCellYouTubeURL(uploadCell.ID, "https://youtu.be/abc"); err != nil {
		t.Fatalf("SetCellYouTubeURL: %v", err)
	}
	got, _ = db.GetCell(uploadCell.ID)
	if got.YouTubeURL == nil || *got.YouTubeURL != "https://youtu.be/abc" {
		t.Fatalf("youtube url not set: %+v", got)
	}
	if got.Status != StatusIdle {
		t.Fatalf("SetCellYouTubeURL should not change status, got %q", got.Status)
	}

	if err := db.RenameCell(editCell.ID, "renamed"); err != nil {
		t.Fatalf("RenameCell: %v", err)
	}
	if err := db.UpdateCellParams(editCell.ID, `{"speed":1.5}`); err != nil {
		t.Fatalf("UpdateCellParams: %v", err)
	}
	got, _ = db.GetCell(editCell.ID)
	if got.DisplayName() != "renamed" || got.ParamsJSON != `{"speed":1.5}` {
		t.Fatalf("rename/params not applied: %+v", got)
	}

	// Deleting the project should cascade-delete its cells.
	if err := db.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := db.GetCell(editCell.ID); err == nil {
		t.Fatal("expected cell to be cascade-deleted with its project")
	}
}

func TestDeleteCellMissingErrors(t *testing.T) {
	db := openTestDB(t)
	if err := db.DeleteCell("nonexistent"); err == nil {
		t.Fatal("expected error deleting missing cell")
	}
}

func TestReorderCellsSwapsWithinKindOnly(t *testing.T) {
	db := openTestDB(t)
	p, source, err := db.CreateProject("P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	var editIDs []string
	for i := range 3 {
		seq, err := db.NextSeq(p.ID)
		if err != nil {
			t.Fatalf("NextSeq: %v", err)
		}
		c, err := db.CreateCell(p.ID, seq, source.ID, nil, KindEdit, "{}", i+1)
		if err != nil {
			t.Fatalf("CreateCell: %v", err)
		}
		editIDs = append(editIDs, c.ID)
	}
	// One upload cell interleaved at a higher position, to confirm
	// reordering edit cells doesn't disturb it.
	uploadSeq, err := db.NextSeq(p.ID)
	if err != nil {
		t.Fatalf("NextSeq: %v", err)
	}
	uploadCell, err := db.CreateCell(p.ID, uploadSeq, editIDs[0], nil, KindUpload, "{}", 99)
	if err != nil {
		t.Fatalf("CreateCell: %v", err)
	}

	// Reverse the edit cells' order.
	reversed := []string{editIDs[2], editIDs[1], editIDs[0]}
	if err := db.ReorderCells(p.ID, KindEdit, reversed); err != nil {
		t.Fatalf("ReorderCells: %v", err)
	}

	cells, err := db.ListCellsByProject(p.ID)
	if err != nil {
		t.Fatalf("ListCellsByProject: %v", err)
	}
	var gotEditOrder []string
	for _, c := range cells {
		if c.Kind == KindEdit {
			gotEditOrder = append(gotEditOrder, c.ID)
		}
		if c.ID == uploadCell.ID && c.Position != 99 {
			t.Fatalf("upload cell position disturbed by reordering edit cells: %d", c.Position)
		}
	}
	if len(gotEditOrder) != 3 || gotEditOrder[0] != editIDs[2] || gotEditOrder[1] != editIDs[1] || gotEditOrder[2] != editIDs[0] {
		t.Fatalf("edit cell order = %v, want reversed %v", gotEditOrder, reversed)
	}

	// seq numbers must be untouched by reordering.
	for _, c := range cells {
		if c.Kind != KindEdit {
			continue
		}
		if c.Seq < 2 || c.Seq > 4 {
			t.Fatalf("unexpected seq after reorder: %d", c.Seq)
		}
	}
}

func TestReorderCellsRejectsForeignOrWrongKindIDs(t *testing.T) {
	db := openTestDB(t)
	p, source, err := db.CreateProject("P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	seq, _ := db.NextSeq(p.ID)
	editCell, err := db.CreateCell(p.ID, seq, source.ID, nil, KindEdit, "{}", 1)
	if err != nil {
		t.Fatalf("CreateCell: %v", err)
	}

	if err := db.ReorderCells(p.ID, KindEdit, []string{editCell.ID, "does-not-exist"}); err == nil {
		t.Fatal("expected error for an unknown cell id")
	}
	if err := db.ReorderCells(p.ID, KindUpload, []string{editCell.ID}); err == nil {
		t.Fatal("expected error when the id is the wrong kind")
	}
}
