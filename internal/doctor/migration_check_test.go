package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationReadinessCheck_AllDolt(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with Dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write Dolt metadata
	metadata := `{"backend": "dolt", "database": "dolt"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewMigrationReadinessCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK, got %v: %s", result.Status, result.Message)
	}

	readiness := check.Readiness()
	if !readiness.Ready {
		t.Errorf("Expected Ready=true, got false. Blockers: %v", readiness.Blockers)
	}
}

func TestMigrationReadinessCheck_SQLiteNeedsMigration(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with SQLite metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write SQLite metadata (or no backend field = defaults to SQLite)
	metadata := `{"backend": "sqlite", "database": "sqlite3"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewMigrationReadinessCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning for SQLite backend, got %v: %s", result.Status, result.Message)
	}

	readiness := check.Readiness()
	if readiness.Ready {
		t.Errorf("Expected Ready=false for SQLite backend, got true")
	}

	// Check that town-root rig is in the list
	found := false
	for _, rig := range readiness.Rigs {
		if rig.Name == "town-root" && rig.NeedsMigration {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected town-root to need migration, rigs: %v", readiness.Rigs)
	}
}

func TestUnmigratedRigCheck_AllDolt(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with Dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	metadata := `{"backend": "dolt"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewUnmigratedRigCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestUnmigratedRigCheck_SQLiteDetected(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with SQLite metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	metadata := `{"backend": "sqlite"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewUnmigratedRigCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning, got %v: %s", result.Status, result.Message)
	}

	// Should have town-root in details
	foundTownRoot := false
	for _, detail := range result.Details {
		if detail == "town-root" {
			foundTownRoot = true
			break
		}
	}
	if !foundTownRoot {
		t.Errorf("Expected 'town-root' in details, got: %v", result.Details)
	}
}

func TestBdSupportsDolt(t *testing.T) {
	check := &MigrationReadinessCheck{}

	tests := []struct {
		version string
		want    bool
	}{
		{"bd version 0.49.3 (commit)", true},
		{"bd version 0.40.0 (commit)", true},
		{"bd version 0.39.9 (commit)", false},
		{"bd version 0.30.0 (commit)", false},
		{"bd version 1.0.0 (commit)", true},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := check.bdSupportsDolt(tt.version)
		if got != tt.want {
			t.Errorf("bdSupportsDolt(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
