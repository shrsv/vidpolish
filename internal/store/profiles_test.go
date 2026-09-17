package store

import "testing"

func TestProfileCRUD(t *testing.T) {
	db := openTestDB(t)

	p, err := db.CreateProfile("half-size", `{"scalePct":50}`)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected a generated profile ID")
	}

	got, err := db.GetProfile(p.ID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Name != "half-size" || got.ParamsJSON != `{"scalePct":50}` {
		t.Fatalf("GetProfile = %+v", got)
	}

	byName, err := db.GetProfileByName("half-size")
	if err != nil || byName.ID != p.ID {
		t.Fatalf("GetProfileByName = %+v, %v", byName, err)
	}

	if err := db.UpdateProfile(p.ID, "renamed", `{"scalePct":75}`); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got, _ = db.GetProfile(p.ID)
	if got.Name != "renamed" || got.ParamsJSON != `{"scalePct":75}` {
		t.Fatalf("UpdateProfile not applied: %+v", got)
	}

	list, err := db.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("ListProfiles = %+v", list)
	}

	if err := db.DeleteProfile(p.ID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, err := db.GetProfile(p.ID); err == nil {
		t.Fatal("expected error getting deleted profile")
	}
}

func TestCreateProfileRejectsDuplicateName(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.CreateProfile("dup", "{}"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if _, err := db.CreateProfile("dup", "{}"); err == nil {
		t.Fatal("expected error creating a profile with a duplicate name")
	}
}

func TestGetProfileMissingErrors(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetProfile("nonexistent"); err == nil {
		t.Fatal("expected error for missing profile")
	}
	if _, err := db.GetProfileByName("nonexistent"); err == nil {
		t.Fatal("expected error for missing profile by name")
	}
}
