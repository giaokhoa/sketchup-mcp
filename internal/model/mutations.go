package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

type MutationEnvelope struct {
	SessionID         string `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
	OperationID       string `json:"operation_id" jsonschema:"idempotency key unique within one SketchUp bridge session"`
	ExpectedModelGUID string `json:"expected_model_guid" jsonschema:"model GUID identity epoch expected by the caller"`
	ExpectedRevision  uint64 `json:"expected_revision" jsonschema:"model revision that must still be current before the mutation"`
}

func (e MutationEnvelope) Validate() error {
	if e.SessionID == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(e.OperationID) == "" {
		return errors.New("operation_id is required")
	}
	if len(e.OperationID) > 128 {
		return errors.New("operation_id must be at most 128 bytes")
	}
	if e.ExpectedModelGUID == "" {
		return errors.New("expected_model_guid is required")
	}
	return nil
}

type BridgeMutation struct {
	OperationID       string `json:"operation_id"`
	ExpectedModelGUID string `json:"expected_model_guid"`
	ExpectedRevision  uint64 `json:"expected_revision"`
	UndoLabel         string `json:"undo_label"`
}

func (e MutationEnvelope) bridgeMutation(undoLabel string) BridgeMutation {
	return BridgeMutation{
		OperationID:       e.OperationID,
		ExpectedModelGUID: e.ExpectedModelGUID,
		ExpectedRevision:  e.ExpectedRevision,
		UndoLabel:         undoLabel,
	}
}

type Vector3 struct {
	X float64 `json:"x" jsonschema:"x coordinate in SketchUp internal inches"`
	Y float64 `json:"y" jsonschema:"y coordinate in SketchUp internal inches"`
	Z float64 `json:"z" jsonschema:"z coordinate in SketchUp internal inches"`
}

func (v Vector3) finite() bool {
	return finite(v.X) && finite(v.Y) && finite(v.Z)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type TranslateInput struct {
	MutationEnvelope
	EntityRef         EntityRef `json:"entity_ref" jsonschema:"durable persistent-id entity reference"`
	TranslationInches Vector3   `json:"translation_inches" jsonschema:"translation vector in SketchUp internal inches"`
}

func (i TranslateInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if err := i.EntityRef.Validate(); err != nil {
		return fmt.Errorf("entity_ref: %w", err)
	}
	if i.EntityRef.SessionID != i.SessionID {
		return errors.New("entity_ref session_id must match session_id")
	}
	if i.EntityRef.ModelGUID != i.ExpectedModelGUID {
		return errors.New("entity_ref model_guid must match expected_model_guid")
	}
	if i.EntityRef.Revision != i.ExpectedRevision {
		return errors.New("entity_ref revision must match expected_revision")
	}
	if !i.TranslationInches.finite() {
		return errors.New("translation_inches values must be finite")
	}
	if i.TranslationInches.X == 0 && i.TranslationInches.Y == 0 && i.TranslationInches.Z == 0 {
		return errors.New("translation_inches must move the entity")
	}
	return nil
}

func (i TranslateInput) BridgePayload() any {
	return struct {
		Mutation          BridgeMutation `json:"mutation"`
		EntityRef         EntityRef      `json:"entity_ref"`
		TranslationInches Vector3        `json:"translation_inches"`
	}{
		Mutation:          i.bridgeMutation("SketchUp MCP: Translate"),
		EntityRef:         i.EntityRef,
		TranslationInches: i.TranslationInches,
	}
}

type DeleteInput struct {
	MutationEnvelope
	EntityRef EntityRef `json:"entity_ref" jsonschema:"durable persistent-id entity reference to delete"`
}

func (i DeleteInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if err := i.EntityRef.Validate(); err != nil {
		return fmt.Errorf("entity_ref: %w", err)
	}
	if i.EntityRef.SessionID != i.SessionID {
		return errors.New("entity_ref session_id must match session_id")
	}
	if i.EntityRef.ModelGUID != i.ExpectedModelGUID {
		return errors.New("entity_ref model_guid must match expected_model_guid")
	}
	if i.EntityRef.Revision != i.ExpectedRevision {
		return errors.New("entity_ref revision must match expected_revision")
	}
	return nil
}

func (i DeleteInput) BridgePayload() any {
	return struct {
		Mutation  BridgeMutation `json:"mutation"`
		EntityRef EntityRef      `json:"entity_ref"`
	}{
		Mutation:  i.bridgeMutation("SketchUp MCP: Delete Entity"),
		EntityRef: i.EntityRef,
	}
}

type RGBColor struct {
	R int `json:"r" jsonschema:"red channel from 0 through 255"`
	G int `json:"g" jsonschema:"green channel from 0 through 255"`
	B int `json:"b" jsonschema:"blue channel from 0 through 255"`
}

func (c RGBColor) Validate() error {
	if c.R < 0 || c.R > 255 || c.G < 0 || c.G > 255 || c.B < 0 || c.B > 255 {
		return errors.New("material color channels must be integers from 0 through 255")
	}
	return nil
}

type MaterialSpec struct {
	Name  string   `json:"name" jsonschema:"bounded deterministic SketchUp material name"`
	Color RGBColor `json:"color" jsonschema:"solid RGB color"`
}

func (m MaterialSpec) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("material name is required")
	}
	if len(m.Name) > 128 {
		return errors.New("material name must be at most 128 bytes")
	}
	return m.Color.Validate()
}

type MaterialSetInput struct {
	MutationEnvelope
	EntityRef EntityRef    `json:"entity_ref" jsonschema:"durable persistent-id entity reference"`
	Material  MaterialSpec `json:"material" jsonschema:"deterministic solid material to assign"`
}

func (i MaterialSetInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if err := i.EntityRef.Validate(); err != nil {
		return fmt.Errorf("entity_ref: %w", err)
	}
	if i.EntityRef.SessionID != i.SessionID {
		return errors.New("entity_ref session_id must match session_id")
	}
	if i.EntityRef.ModelGUID != i.ExpectedModelGUID {
		return errors.New("entity_ref model_guid must match expected_model_guid")
	}
	if i.EntityRef.Revision != i.ExpectedRevision {
		return errors.New("entity_ref revision must match expected_revision")
	}
	return i.Material.Validate()
}

func (i MaterialSetInput) BridgePayload() any {
	return struct {
		Mutation  BridgeMutation `json:"mutation"`
		EntityRef EntityRef      `json:"entity_ref"`
		Material  MaterialSpec   `json:"material"`
	}{
		Mutation:  i.bridgeMutation("SketchUp MCP: Set Material"),
		EntityRef: i.EntityRef,
		Material:  i.Material,
	}
}

type NameSetInput struct {
	MutationEnvelope
	EntityRef EntityRef `json:"entity_ref" jsonschema:"durable persistent-id entity reference"`
	Name      string    `json:"name" jsonschema:"human-readable instance or group name, at most 128 UTF-8 bytes"`
}

func (i NameSetInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if err := i.EntityRef.Validate(); err != nil {
		return fmt.Errorf("entity_ref: %w", err)
	}
	if i.EntityRef.SessionID != i.SessionID {
		return errors.New("entity_ref session_id must match session_id")
	}
	if i.EntityRef.ModelGUID != i.ExpectedModelGUID {
		return errors.New("entity_ref model_guid must match expected_model_guid")
	}
	if i.EntityRef.Revision != i.ExpectedRevision {
		return errors.New("entity_ref revision must match expected_revision")
	}
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("name is required")
	}
	if len(i.Name) > 128 {
		return errors.New("name must be at most 128 bytes")
	}
	return nil
}

func (i NameSetInput) BridgePayload() any {
	return struct {
		Mutation  BridgeMutation `json:"mutation"`
		EntityRef EntityRef      `json:"entity_ref"`
		Name      string         `json:"name"`
	}{
		Mutation:  i.bridgeMutation("SketchUp MCP: Set Entity Name"),
		EntityRef: i.EntityRef,
		Name:      i.Name,
	}
}

type BoxDimensions struct {
	Width  float64 `json:"width" jsonschema:"box width in SketchUp internal inches, greater than zero"`
	Depth  float64 `json:"depth" jsonschema:"box depth in SketchUp internal inches, greater than zero"`
	Height float64 `json:"height" jsonschema:"box height in SketchUp internal inches, greater than zero"`
}

func (d BoxDimensions) Validate() error {
	if !finite(d.Width) || !finite(d.Depth) || !finite(d.Height) {
		return errors.New("dimensions_inches values must be finite")
	}
	if d.Width <= 0 || d.Depth <= 0 || d.Height <= 0 {
		return errors.New("dimensions_inches width, depth, and height must be greater than zero")
	}
	return nil
}

type CreateBoxInput struct {
	MutationEnvelope
	OriginInches     Vector3       `json:"origin_inches" jsonschema:"box group origin in current edit context, in SketchUp internal inches"`
	DimensionsInches BoxDimensions `json:"dimensions_inches" jsonschema:"positive box dimensions in SketchUp internal inches"`
}

func (i CreateBoxInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if !i.OriginInches.finite() {
		return errors.New("origin_inches values must be finite")
	}
	return i.DimensionsInches.Validate()
}

func (i CreateBoxInput) BridgePayload() any {
	return struct {
		Mutation         BridgeMutation `json:"mutation"`
		OriginInches     Vector3        `json:"origin_inches"`
		DimensionsInches BoxDimensions  `json:"dimensions_inches"`
	}{
		Mutation:         i.bridgeMutation("SketchUp MCP: Create Box"),
		OriginInches:     i.OriginInches,
		DimensionsInches: i.DimensionsInches,
	}
}

type UndoInput struct {
	MutationEnvelope
}

func (i UndoInput) Validate() error {
	return i.MutationEnvelope.Validate()
}

func (i UndoInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
	}{
		Mutation: i.bridgeMutation("SketchUp MCP: Undo"),
	}
}

type MutationOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	Error       *ToolError `json:"error,omitempty"`
}

type TranslateOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	EntityRef   *EntityRef `json:"entity_ref,omitempty"`
	Error       *ToolError `json:"error,omitempty"`
}

type CreateBoxOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	EntityRef   *EntityRef `json:"entity_ref,omitempty"`
	Error       *ToolError `json:"error,omitempty"`
}

type DeleteOutput struct {
	OperationID         string     `json:"operation_id,omitempty"`
	ModelGUID           string     `json:"model_guid,omitempty"`
	Revision            uint64     `json:"revision"`
	DeletedPersistentID int64      `json:"deleted_persistent_id,omitempty"`
	Error               *ToolError `json:"error,omitempty"`
}

type MaterialInfo struct {
	Name  string   `json:"name"`
	Color RGBColor `json:"color"`
}

type MaterialSetOutput struct {
	OperationID string        `json:"operation_id,omitempty"`
	ModelGUID   string        `json:"model_guid,omitempty"`
	Revision    uint64        `json:"revision"`
	EntityRef   *EntityRef    `json:"entity_ref,omitempty"`
	Material    *MaterialInfo `json:"material,omitempty"`
	Error       *ToolError    `json:"error,omitempty"`
}

type NameSetOutput struct {
	OperationID string     `json:"operation_id,omitempty"`
	ModelGUID   string     `json:"model_guid,omitempty"`
	Revision    uint64     `json:"revision"`
	EntityRef   *EntityRef `json:"entity_ref,omitempty"`
	Name        string     `json:"name,omitempty"`
	Error       *ToolError `json:"error,omitempty"`
}

type UndoOutput MutationOutput
