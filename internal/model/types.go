package model

import (
	"errors"
	"fmt"
)

const (
	ErrorInvalidRequest                = "INVALID_REQUEST"
	ErrorSessionNotFound               = "SESSION_NOT_FOUND"
	ErrorModelChanged                  = "MODEL_CHANGED"
	ErrorEntityNotFound                = "ENTITY_NOT_FOUND"
	ErrorEntityTypeNotSupported        = "ENTITY_TYPE_NOT_SUPPORTED"
	ErrorEntityNoSupportedPersistentID = "ENTITY_HAS_NO_SUPPORTED_PERSISTENT_ID"
	ErrorStaleRevision                 = "STALE_REVISION"
	ErrorOperationIDReuse              = "OPERATION_ID_REUSE"
	ErrorOperationInProgress           = "OPERATION_IN_PROGRESS"
	ErrorInvalidEntityReference        = "INVALID_ENTITY_REFERENCE"
	ErrorEntityDeleted                 = "ENTITY_DELETED"
	ErrorLockedEntityOrContext         = "LOCKED_ENTITY_OR_CONTEXT"
	ErrorInvalidDimensions             = "INVALID_DIMENSIONS"
	ErrorInvalidTransform              = "INVALID_TRANSFORM"
	ErrorSketchUpOperationFailed       = "SKETCHUP_OPERATION_FAILED"
	ErrorMaterialNameConflict           = "MATERIAL_NAME_CONFLICT"
)

type ToolError struct {
	Code    string         `json:"code" jsonschema:"stable machine-readable error code"`
	Message string         `json:"message" jsonschema:"human-readable error message"`
	Details map[string]any `json:"details,omitempty" jsonschema:"optional structured error details"`
}

type EntityRef struct {
	SessionID    string `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
	ModelGUID    string `json:"model_guid" jsonschema:"SketchUp model GUID identity epoch"`
	PersistentID int64  `json:"persistent_id" jsonschema:"SketchUp Entity persistent_id"`
	Revision     uint64 `json:"revision" jsonschema:"model revision observed with this reference"`
}

func (r EntityRef) Validate() error {
	if r.SessionID == "" {
		return errors.New("session_id is required")
	}
	if r.ModelGUID == "" {
		return errors.New("model_guid is required")
	}
	if r.PersistentID <= 0 {
		return fmt.Errorf("persistent_id must be positive, got %d", r.PersistentID)
	}
	return nil
}

type SummaryInput struct {
	SessionID string `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
}

func (i SummaryInput) Validate() error {
	if i.SessionID == "" {
		return errors.New("session_id is required")
	}
	return nil
}

type Units struct {
	LengthUnit       string `json:"length_unit" jsonschema:"model display length unit"`
	LengthUnitCode   int    `json:"length_unit_code" jsonschema:"SketchUp LengthUnit option value"`
	LengthFormat     string `json:"length_format" jsonschema:"model display length format"`
	LengthFormatCode int    `json:"length_format_code" jsonschema:"SketchUp LengthFormat option value"`
	LengthPrecision  int    `json:"length_precision" jsonschema:"model display length precision"`
}

type Bounds struct {
	Min []float64 `json:"min" jsonschema:"minimum XYZ in SketchUp internal inches"`
	Max []float64 `json:"max" jsonschema:"maximum XYZ in SketchUp internal inches"`
}

type EditPathEntity struct {
	Ref  *EntityRef `json:"ref,omitempty" jsonschema:"durable entity reference when supported"`
	Type string     `json:"type" jsonschema:"SketchUp entity typename"`
	Name string     `json:"name,omitempty" jsonschema:"entity or definition name"`
}

type EditContext struct {
	Depth         int              `json:"depth" jsonschema:"active edit nesting depth"`
	Path          []EditPathEntity `json:"path" jsonschema:"active group/component edit path"`
	EditTransform []float64        `json:"edit_transform" jsonschema:"16-value SketchUp edit transform matrix"`
}

type Counts struct {
	TopLevelEntities int `json:"top_level_entities" jsonschema:"root model entity count without recursive traversal"`
	Definitions      int `json:"definitions" jsonschema:"component definition count"`
	Materials        int `json:"materials" jsonschema:"material count"`
	Tags             int `json:"tags" jsonschema:"tag/layer count"`
	Selection        int `json:"selection" jsonschema:"current selection count"`
}

type SummaryOutput struct {
	SessionID         string      `json:"session_id,omitempty"`
	ModelGUID         string      `json:"model_guid,omitempty"`
	Revision          uint64      `json:"revision"`
	Title             string      `json:"title,omitempty"`
	Path              string      `json:"path,omitempty"`
	Units             Units       `json:"units"`
	ActiveEditContext EditContext `json:"active_edit_context"`
	Counts            Counts      `json:"counts"`
	Error             *ToolError  `json:"error,omitempty"`
}

type SelectionInput struct {
	SessionID string `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
}

func (i SelectionInput) Validate() error {
	if i.SessionID == "" {
		return errors.New("session_id is required")
	}
	return nil
}

type SelectionEntity struct {
	Ref    *EntityRef `json:"ref,omitempty"`
	Type   string     `json:"type"`
	Name   string     `json:"name,omitempty"`
	Bounds *Bounds    `json:"bounds,omitempty"`
	Error  *ToolError `json:"error,omitempty"`
}

type SelectionOutput struct {
	SessionID     string            `json:"session_id,omitempty"`
	ModelGUID     string            `json:"model_guid,omitempty"`
	Revision      uint64            `json:"revision"`
	SelectedCount int               `json:"selected_count"`
	ReturnedCount int               `json:"returned_count"`
	Truncated     bool              `json:"truncated"`
	Entities      []SelectionEntity `json:"entities"`
	Error         *ToolError        `json:"error,omitempty"`
}

type InspectInput EntityRef

func (i InspectInput) Ref() EntityRef {
	return EntityRef(i)
}

func (i InspectInput) Validate() error {
	return i.Ref().Validate()
}

type DefinitionInfo struct {
	Name        string `json:"name,omitempty"`
	EntityCount int    `json:"entity_count"`
}

type EntityDetails struct {
	Ref            EntityRef       `json:"ref"`
	Type           string          `json:"type"`
	Name           string          `json:"name,omitempty"`
	Bounds         Bounds          `json:"bounds"`
	Transformation []float64       `json:"transformation"`
	Material       string          `json:"material,omitempty"`
	Tag            string          `json:"tag,omitempty"`
	Definition     *DefinitionInfo `json:"definition,omitempty"`
}

type InspectOutput struct {
	CurrentRevision  uint64         `json:"current_revision"`
	RevisionMismatch bool           `json:"revision_mismatch"`
	Entity           *EntityDetails `json:"entity,omitempty"`
	Error            *ToolError     `json:"error,omitempty"`
}

type ChildrenInput EntityRef

func (i ChildrenInput) Ref() EntityRef {
	return EntityRef(i)
}

func (i ChildrenInput) Validate() error {
	return i.Ref().Validate()
}

type ChildEntity struct {
	Ref      *EntityRef `json:"ref,omitempty"`
	Type     string     `json:"type"`
	Name     string     `json:"name,omitempty"`
	Material string     `json:"material,omitempty"`
	Error    *ToolError `json:"error,omitempty"`
}

type ChildrenOutput struct {
	CurrentRevision     uint64        `json:"current_revision"`
	RevisionMismatch    bool          `json:"revision_mismatch"`
	RawEntityCount      int           `json:"raw_entity_count"`
	SupportedChildCount int           `json:"supported_child_count"`
	ReturnedCount       int           `json:"returned_count"`
	Truncated           bool          `json:"truncated"`
	Children            []ChildEntity `json:"children"`
	Error               *ToolError    `json:"error,omitempty"`
}
