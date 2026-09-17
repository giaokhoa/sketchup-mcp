package main

import (
	"path/filepath"
	"testing"
)

func TestCheckedInFixturesValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		path           string
		wantViewports  int
		wantDimensions int
		wantPanels     int
	}{
		{"cabinet", "cabinet/spec.json", 6, 52, 6},
		{"cabinet-minimal", "cabinet-minimal/spec.json", 1, 1, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture, err := loadFixture(filepath.Join("..", "..", "testdata", "e2e", tc.path))
			if err != nil {
				t.Fatalf("loadFixture() error = %v", err)
			}
			if len(fixture.Viewports) != tc.wantViewports {
				t.Fatalf("viewports = %d, want %d", len(fixture.Viewports), tc.wantViewports)
			}
			if len(fixture.Dimensions) != tc.wantDimensions {
				t.Fatalf("dimensions = %d, want %d", len(fixture.Dimensions), tc.wantDimensions)
			}
			if got := len(populatedPanels(fixture)); got != tc.wantPanels {
				t.Fatalf("populated panels = %d, want %d", got, tc.wantPanels)
			}
		})
	}
}

func TestSecondFixtureChangesCompositionWithoutRunnerChanges(t *testing.T) {
	full, err := loadFixture(filepath.Join("..", "..", "testdata", "e2e", "cabinet", "spec.json"))
	if err != nil {
		t.Fatal(err)
	}
	minimal, err := loadFixture(filepath.Join("..", "..", "testdata", "e2e", "cabinet-minimal", "spec.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Viewports) == len(minimal.Viewports) || len(full.Dimensions) == len(minimal.Dimensions) {
		t.Fatalf("fixtures do not demonstrate composition variation: full=%d/%d minimal=%d/%d",
			len(full.Viewports), len(full.Dimensions), len(minimal.Viewports), len(minimal.Dimensions))
	}
}
