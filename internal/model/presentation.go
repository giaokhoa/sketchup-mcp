package model

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
)

type Point3MM struct {
	X float64 `json:"x" jsonschema:"x coordinate in millimeters"`
	Y float64 `json:"y" jsonschema:"y coordinate in millimeters"`
	Z float64 `json:"z" jsonschema:"z coordinate in millimeters"`
}

func (p Point3MM) Validate() error {
	if !finite(p.X) || !finite(p.Y) || !finite(p.Z) {
		return errors.New("point values must be finite")
	}
	return nil
}

type Direction3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func (v Direction3) Validate() error {
	if !finite(v.X) || !finite(v.Y) || !finite(v.Z) {
		return errors.New("direction values must be finite")
	}
	if math.Abs(v.X)+math.Abs(v.Y)+math.Abs(v.Z) == 0 {
		return errors.New("direction must be non-zero")
	}
	return nil
}

type ModelBoundsInput struct {
	SessionID string `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
}

func (i ModelBoundsInput) Validate() error {
	if strings.TrimSpace(i.SessionID) == "" {
		return errors.New("session_id is required")
	}
	return nil
}

type ModelBoundsOutput struct {
	SessionID string     `json:"session_id,omitempty"`
	ModelGUID string     `json:"model_guid,omitempty"`
	Revision  uint64     `json:"revision"`
	Empty      bool       `json:"empty"`
	MinMM     Point3MM   `json:"min_mm"`
	MaxMM     Point3MM   `json:"max_mm"`
	SizeMM    Point3MM   `json:"size_mm"`
	CenterMM  Point3MM   `json:"center_mm"`
	Error     *ToolError `json:"error,omitempty"`
}

type SectionPlaneCreateInput struct {
	MutationEnvelope
	PointMM Point3MM   `json:"point_mm"`
	Normal  Direction3 `json:"normal"`
	Name    string     `json:"name"`
	Symbol  string     `json:"symbol,omitempty"`
}

func (i SectionPlaneCreateInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if err := i.PointMM.Validate(); err != nil {
		return err
	}
	if err := i.Normal.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("name is required")
	}
	if len(i.Name) > 128 {
		return errors.New("name must be at most 128 bytes")
	}
	if len(i.Symbol) > 3 {
		return errors.New("symbol must be at most 3 bytes")
	}
	return nil
}

func (i SectionPlaneCreateInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		PointMM  Point3MM       `json:"point_mm"`
		Normal   Direction3     `json:"normal"`
		Name     string         `json:"name"`
		Symbol   string         `json:"symbol"`
	}{
		Mutation: i.bridgeMutation("SketchUp MCP: Create Section Plane"),
		PointMM:  i.PointMM,
		Normal:   i.Normal,
		Name:     i.Name,
		Symbol:   i.Symbol,
	}
}

type SectionPlaneCreateOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	EntityRef   *EntityRef `json:"entity_ref,omitempty"`
	Name        string     `json:"name,omitempty"`
	Symbol      string     `json:"symbol,omitempty"`
	Error       *ToolError `json:"error,omitempty"`
}

type SceneCreateInput struct {
	MutationEnvelope
	Name                 string     `json:"name" jsonschema:"stable SketchUp scene name for reusable presentation state and LayOut scene_name viewports"`
	EyeMM                Point3MM   `json:"eye_mm"`
	TargetMM             Point3MM   `json:"target_mm"`
	Up                   Direction3 `json:"up"`
	Perspective          bool       `json:"perspective"`
	OrthographicHeightMM float64    `json:"orthographic_height_mm" jsonschema:"required for orthographic scenes; zero for perspective"`
	FOVDegrees           float64    `json:"fov_degrees" jsonschema:"1 through 120 for perspective scenes; zero for orthographic"`
	SectionPlaneRef      *EntityRef `json:"section_plane_ref,omitempty" jsonschema:"optional section plane captured into this scene presentation state"`
	DisplaySectionPlane  bool       `json:"display_section_plane"`
}

func (i SceneCreateInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("name is required")
	}
	if len(i.Name) > 128 {
		return errors.New("name must be at most 128 bytes")
	}
	if err := i.EyeMM.Validate(); err != nil {
		return err
	}
	if err := i.TargetMM.Validate(); err != nil {
		return err
	}
	if err := i.Up.Validate(); err != nil {
		return err
	}
	if i.EyeMM == i.TargetMM {
		return errors.New("eye_mm and target_mm must differ")
	}
	if i.SectionPlaneRef != nil {
		if err := i.SectionPlaneRef.Validate(); err != nil {
			return err
		}
		if i.SectionPlaneRef.SessionID != i.SessionID ||
			i.SectionPlaneRef.ModelGUID != i.ExpectedModelGUID ||
			i.SectionPlaneRef.Revision != i.ExpectedRevision {
			return errors.New("section_plane_ref must match the mutation session, model, and revision")
		}
	}
	if i.Perspective {
		if !finite(i.FOVDegrees) || i.FOVDegrees < 1 || i.FOVDegrees > 120 {
			return errors.New("fov_degrees must be between 1 and 120 for perspective scenes")
		}
		if i.OrthographicHeightMM != 0 {
			return errors.New("orthographic_height_mm must be 0 for perspective scenes")
		}
	} else {
		if !finite(i.OrthographicHeightMM) || i.OrthographicHeightMM <= 0 {
			return errors.New("orthographic_height_mm must be greater than zero for orthographic scenes")
		}
		if i.FOVDegrees != 0 {
			return errors.New("fov_degrees must be 0 for orthographic scenes")
		}
	}
	return nil
}

func (i SceneCreateInput) BridgePayload() any {
	return struct {
		Mutation             BridgeMutation `json:"mutation"`
		Name                 string         `json:"name"`
		EyeMM                Point3MM       `json:"eye_mm"`
		TargetMM             Point3MM       `json:"target_mm"`
		Up                   Direction3     `json:"up"`
		Perspective          bool           `json:"perspective"`
		OrthographicHeightMM float64        `json:"orthographic_height_mm"`
		FOVDegrees           float64        `json:"fov_degrees"`
		SectionPlaneRef      *EntityRef     `json:"section_plane_ref"`
		DisplaySectionPlane  bool           `json:"display_section_plane"`
	}{
		Mutation:             i.bridgeMutation("SketchUp MCP: Create Scene"),
		Name:                 i.Name,
		EyeMM:                i.EyeMM,
		TargetMM:             i.TargetMM,
		Up:                   i.Up,
		Perspective:          i.Perspective,
		OrthographicHeightMM: i.OrthographicHeightMM,
		FOVDegrees:           i.FOVDegrees,
		SectionPlaneRef:      i.SectionPlaneRef,
		DisplaySectionPlane:  i.DisplaySectionPlane,
	}
}

type SceneCreateOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	Name        string     `json:"name,omitempty"`
	Index       int        `json:"index"`
	Error       *ToolError `json:"error,omitempty"`
}

type ModelSaveCopyInput struct {
	MutationEnvelope
	OutputPath string `json:"output_path" jsonschema:"absolute .skp path for a saved copy"`
}

func (i ModelSaveCopyInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(i.OutputPath) == "" || !filepath.IsAbs(i.OutputPath) {
		return errors.New("output_path must be absolute")
	}
	if strings.ToLower(filepath.Ext(i.OutputPath)) != ".skp" {
		return errors.New("output_path must end in .skp")
	}
	return nil
}

func (i ModelSaveCopyInput) BridgePayload() any {
	return struct {
		Mutation   BridgeMutation `json:"mutation"`
		OutputPath string         `json:"output_path"`
	}{
		Mutation:   i.bridgeMutation("SketchUp MCP: Save Model Copy"),
		OutputPath: i.OutputPath,
	}
}

type ModelSaveCopyOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	OutputPath  string     `json:"output_path,omitempty"`
	Error       *ToolError `json:"error,omitempty"`
}
