package layoutworker

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
)

func validSpec(t *testing.T) model.LayoutDrawingSpec {
	t.Helper()
	root := t.TempDir()
	snapshot := filepath.Join(root, "cabinet.skp")
	views := []model.LayoutViewSpec{
		{ID: "plan", Title: "PLAN", SceneIndex: 1, Rect: model.LayoutPaperRect{Left: 0, Top: 0, Right: 4, Bottom: 3}, Scale: 1.0 / 15.0},
		{ID: "front", Title: "FRONT", SceneIndex: 2, Rect: model.LayoutPaperRect{Left: 4, Top: 0, Right: 8, Bottom: 3}, Scale: 1.0 / 15.0},
		{ID: "section_a", Title: "A-A", SceneIndex: 3, Rect: model.LayoutPaperRect{Left: 8, Top: 0, Right: 12, Bottom: 3}, Scale: 1.0 / 10.0},
		{ID: "section_b", Title: "B-B", SceneIndex: 4, Rect: model.LayoutPaperRect{Left: 0, Top: 4, Right: 6, Bottom: 8}, Scale: 1.0 / 15.0},
		{ID: "side", Title: "SIDE", SceneIndex: 5, Rect: model.LayoutPaperRect{Left: 6, Top: 4, Right: 9, Bottom: 8}, Scale: 1.0 / 10.0},
		{ID: "iso", Title: "ISO", SceneIndex: 6, Rect: model.LayoutPaperRect{Left: 9, Top: 4, Right: 14, Bottom: 8}, Perspective: true},
	}
	return model.LayoutDrawingSpec{
		SchemaVersion:   1,
		SnapshotPath:    snapshot,
		OutputDirectory: root,
		BaseName:        "cabinet-a3",
		PageWidthMM:     420,
		PageHeightMM:    297,
		Views:           views,
		Dimensions: []model.LayoutDimensionSpec{
			{
				ID: "overall-width", ViewID: "front",
				Start: model.LayoutDimensionEndpoint{
					Point: model.LayoutPoint3D{X: 0, Y: 0, Z: 0},
					PersistentIDPath: "10.20.30",
				},
				End: model.LayoutDimensionEndpoint{
					Point: model.LayoutPoint3D{X: 10, Y: 0, Z: 0},
					PersistentIDPath: "10.20.30",
				},
				Offset: model.LayoutPoint3D{Z: 1},
			},
		},
	}
}

func TestValidateSpecAcceptsSixViewsAndAssociativeDimensions(t *testing.T) {
	spec := validSpec(t)
	if err := ValidateSpec(spec); err != nil {
		t.Fatalf("ValidateSpec() error = %v", err)
	}
}

func TestValidateSpecRejectsMissingPIDConnection(t *testing.T) {
	spec := validSpec(t)
	spec.Dimensions[0].Start.PersistentIDPath = ""
	if err := ValidateSpec(spec); err == nil {
		t.Fatal("ValidateSpec() = nil, want persistent-id error")
	}
}

func TestValidateSpecRejectsWrongViewCount(t *testing.T) {
	spec := validSpec(t)
	spec.Views = spec.Views[:5]
	if err := ValidateSpec(spec); err == nil {
		t.Fatal("ValidateSpec() = nil, want six-view error")
	}
}

func TestNonWindowsGenerateIsExplicit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows guard test")
	}
	spec := validSpec(t)
	if _, err := Generate(t.Context(), spec); err == nil {
		t.Fatal("Generate() = nil error on non-Windows")
	}
}
