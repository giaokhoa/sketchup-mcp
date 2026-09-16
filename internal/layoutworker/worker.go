package layoutworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
)

func LoadSpec(path string) (model.LayoutDrawingSpec, error) {
	var spec model.LayoutDrawingSpec
	data, err := os.ReadFile(path)
	if err != nil {
		return spec, err
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, err
	}
	if err := ValidateSpec(spec); err != nil {
		return spec, err
	}
	return spec, nil
}

func ValidateSpec(spec model.LayoutDrawingSpec) error {
	if spec.SchemaVersion != 1 {
		return fmt.Errorf("unsupported drawing spec schema_version %d", spec.SchemaVersion)
	}
	if strings.TrimSpace(spec.SnapshotPath) == "" || !filepath.IsAbs(spec.SnapshotPath) {
		return errors.New("snapshot_path must be absolute")
	}
	if strings.TrimSpace(spec.OutputDirectory) == "" || !filepath.IsAbs(spec.OutputDirectory) {
		return errors.New("output_directory must be absolute")
	}
	if strings.TrimSpace(spec.BaseName) == "" {
		return errors.New("base_name is required")
	}
	if spec.PageWidthMM <= 0 || spec.PageHeightMM <= 0 {
		return errors.New("page dimensions must be positive")
	}
	if len(spec.Views) != 6 {
		return fmt.Errorf("expected exactly 6 views, got %d", len(spec.Views))
	}
	ids := make(map[string]struct{}, len(spec.Views))
	for i, view := range spec.Views {
		if strings.TrimSpace(view.ID) == "" || strings.TrimSpace(view.Title) == "" {
			return fmt.Errorf("views[%d] id/title are required", i)
		}
		if _, ok := ids[view.ID]; ok {
			return fmt.Errorf("duplicate view id %q", view.ID)
		}
		ids[view.ID] = struct{}{}
		if view.SceneIndex < 1 {
			return fmt.Errorf("views[%d] scene_index must be at least 1", i)
		}
		if view.Rect.Right <= view.Rect.Left || view.Rect.Bottom <= view.Rect.Top {
			return fmt.Errorf("views[%d] paper rectangle is invalid", i)
		}
		if !view.Perspective && view.Scale <= 0 {
			return fmt.Errorf("views[%d] orthographic scale must be positive", i)
		}
	}
	for i, dim := range spec.Dimensions {
		if strings.TrimSpace(dim.ID) == "" {
			return fmt.Errorf("dimensions[%d] id is required", i)
		}
		if _, ok := ids[dim.ViewID]; !ok {
			return fmt.Errorf("dimensions[%d] references unknown view %q", i, dim.ViewID)
		}
		if dim.Start.PersistentIDPath == "" || dim.End.PersistentIDPath == "" {
			return fmt.Errorf("dimensions[%d] must use persistent-id connection paths", i)
		}
	}
	return nil
}

func outputPaths(spec model.LayoutDrawingSpec) (layout, pdf, pngBase, qa string) {
	layout = filepath.Join(spec.OutputDirectory, spec.BaseName+".layout")
	pdf = filepath.Join(spec.OutputDirectory, spec.BaseName+".pdf")
	pngBase = spec.BaseName + "-preview"
	qa = filepath.Join(spec.OutputDirectory, spec.BaseName+".qa.json")
	return
}

func writeQA(path string, report model.LayoutQAReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func fileCheck(name, path string) model.LayoutQACheck {
	info, err := os.Stat(path)
	if err != nil {
		return model.LayoutQACheck{Name: name, Passed: false, Details: err.Error()}
	}
	if info.Size() <= 0 {
		return model.LayoutQACheck{Name: name, Passed: false, Details: "file is empty"}
	}
	return model.LayoutQACheck{Name: name, Passed: true, Details: fmt.Sprintf("%d bytes", info.Size())}
}
