package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type Vec2 struct {
	X float64
	Y float64
}

type Vec3 struct {
	X float64
	Y float64
	Z float64
}

type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type ExpectedModel struct {
	SizeMm      Vec3
	ToleranceMm float64
}

type TemplateRequirement struct {
	SchemaVersion int
	TemplateKind  string
	Slots         []string
	Panels        []string
	Styles        []string
	Layers        []string
}

type SectionSpec struct {
	Name    string
	Symbol  string
	PointMm Vec3
	Normal  Vec3
}

type SceneSpec struct {
	Name                 string
	EyeMm                Vec3
	TargetMm             Vec3
	Up                   Vec3
	Perspective          bool
	OrthographicHeightMm float64
	FovDegrees           float64
	SectionId            string
	DisplaySectionPlane  bool
}

type PresentationStep struct {
	Kind    string
	Id      string
	Section *SectionSpec
	Scene   *SceneSpec
}

type ViewportSpec struct {
	Id               string
	SlotId           string
	SceneName        string
	StandardView     string
	Perspective      bool
	ScaleDenominator float64
	RenderMode       string
	LayerName        string
	PanelId          string
	FitMarginMm      float64
}

type DimensionSpec struct {
	Id           string
	ViewportId   string
	PageIndex    int
	LayerName    string
	StartPointMm Vec3
	EndPointMm   Vec3
	StartPidPath string
	EndPidPath   string
	OffsetMm     float64
	Alignment    string
	StyleId      string
	PanelId      string
}

type TextSpec struct {
	Id         string
	PageIndex  int
	LayerName  string
	BoundsMm   Rect
	Text       string
	FontSizePt float64
	Bold       bool
	Alignment  string
	StyleId    string
	PanelId    string
}

type LineSpec struct {
	Id          string
	PageIndex   int
	LayerName   string
	StartMm     Vec2
	EndMm       Vec2
	StrokeWidth float64
	StyleId     string
	PanelId     string
}

type RectangleSpec struct {
	Id          string
	PageIndex   int
	LayerName   string
	BoundsMm    Rect
	StrokeWidth float64
	StyleId     string
	PanelId     string
}

type Fixture struct {
	Name             string
	OutputBase       string
	ExpectedModel    ExpectedModel
	RequiredTemplate TemplateRequirement
	Presentation     []PresentationStep
	Viewports        []ViewportSpec
	Dimensions       []DimensionSpec
	Texts            []TextSpec
	Lines            []LineSpec
	Rectangles       []RectangleSpec
	PanelMarginMm    float64
	ExportDpi        int
}

func loadFixture(path string) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture: %w", err)
	}
	var fixture Fixture
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, fmt.Errorf("decode fixture: %w", err)
	}
	if err := fixture.Validate(); err != nil {
		return Fixture{}, err
	}
	return fixture, nil
}

func (f Fixture) Validate() error {
	if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.OutputBase) == "" {
		return fmt.Errorf("fixture name and outputBase are required")
	}
	if f.ExpectedModel.ToleranceMm <= 0 {
		return fmt.Errorf("expectedModel.toleranceMm must be positive")
	}
	if f.RequiredTemplate.SchemaVersion <= 0 {
		return fmt.Errorf("requiredTemplate.schemaVersion must be positive")
	}
	if len(f.RequiredTemplate.Slots) == 0 || len(f.RequiredTemplate.Panels) == 0 || len(f.RequiredTemplate.Styles) == 0 {
		return fmt.Errorf("required template slots, panels, and styles must be declared")
	}
	sectionIDs := map[string]bool{}
	for i, step := range f.Presentation {
		switch strings.ToLower(step.Kind) {
		case "section":
			if step.Section == nil || step.Scene != nil || strings.TrimSpace(step.Id) == "" {
				return fmt.Errorf("presentation[%d] section step is malformed", i)
			}
			if sectionIDs[step.Id] {
				return fmt.Errorf("duplicate section id %q", step.Id)
			}
			sectionIDs[step.Id] = true
		case "scene":
			if step.Scene == nil || step.Section != nil {
				return fmt.Errorf("presentation[%d] scene step is malformed", i)
			}
			if step.Scene.SectionId != "" && !sectionIDs[step.Scene.SectionId] {
				return fmt.Errorf("presentation[%d] references section %q before it is created", i, step.Scene.SectionId)
			}
		default:
			return fmt.Errorf("presentation[%d].kind must be section or scene", i)
		}
	}
	viewportIDs := map[string]bool{}
	for i, viewport := range f.Viewports {
		if viewport.Id == "" || viewport.SlotId == "" || viewport.PanelId == "" {
			return fmt.Errorf("viewports[%d] id, slotId, and panelId are required", i)
		}
		if viewportIDs[viewport.Id] {
			return fmt.Errorf("duplicate viewport id %q", viewport.Id)
		}
		viewportIDs[viewport.Id] = true
		hasScene := strings.TrimSpace(viewport.SceneName) != ""
		hasView := strings.TrimSpace(viewport.StandardView) != ""
		if hasScene == hasView {
			return fmt.Errorf("viewports[%d] must provide exactly one of sceneName or standardView", i)
		}
	}
	for i, dimension := range f.Dimensions {
		if dimension.Id == "" || !viewportIDs[dimension.ViewportId] || dimension.PanelId == "" {
			return fmt.Errorf("dimensions[%d] has invalid id, viewportId, or panelId", i)
		}
	}
	if f.PanelMarginMm < 0 || f.ExportDpi <= 0 {
		return fmt.Errorf("panelMarginMm must be non-negative and exportDpi must be positive")
	}
	return nil
}

func populatedPanels(f Fixture) []string {
	set := map[string]bool{}
	for _, v := range f.Viewports {
		set[v.PanelId] = true
	}
	for _, v := range f.Dimensions {
		set[v.PanelId] = true
	}
	for _, v := range f.Texts {
		set[v.PanelId] = true
	}
	for _, v := range f.Lines {
		set[v.PanelId] = true
	}
	for _, v := range f.Rectangles {
		set[v.PanelId] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		if id != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
