package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func validEnvelope() MutationEnvelope {
	return MutationEnvelope{
		SessionID:         "11111111-1111-4111-8111-111111111111",
		OperationID:       "operation-1",
		ExpectedModelGUID: "model-guid",
		ExpectedRevision:  7,
	}
}

func validRef() EntityRef {
	return EntityRef{
		SessionID:    "11111111-1111-4111-8111-111111111111",
		ModelGUID:    "model-guid",
		PersistentID: 42,
		Revision:     7,
	}
}

func TestTranslateInputRequiresMatchingDurableReference(t *testing.T) {
	input := TranslateInput{
		MutationEnvelope:  validEnvelope(),
		EntityRef:         validRef(),
		TranslationInches: Vector3{X: 1},
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	input.EntityRef.Revision = 6
	err := input.Validate()
	if err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("Validate() error = %v, want revision mismatch", err)
	}
}

func TestCreateBoxInputRejectsNonPositiveDimensions(t *testing.T) {
	input := CreateBoxInput{
		MutationEnvelope: validEnvelope(),
		OriginInches:     Vector3{},
		DimensionsInches: BoxDimensions{Width: 4, Depth: 5, Height: 0},
	}
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() = nil, want invalid dimensions")
	}
}

func TestMutationBridgePayloadUsesADRMutationEnvelope(t *testing.T) {
	input := TranslateInput{
		MutationEnvelope:  validEnvelope(),
		EntityRef:         validRef(),
		TranslationInches: Vector3{X: 1, Y: 2, Z: 3},
	}
	data, err := json.Marshal(input.BridgePayload())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{"mutation", "entity_ref", "translation_inches"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("bridge payload missing %q: %s", key, data)
		}
	}
	mutation, ok := payload["mutation"].(map[string]any)
	if !ok {
		t.Fatalf("mutation = %#v, want object", payload["mutation"])
	}
	for _, key := range []string{"operation_id", "expected_model_guid", "expected_revision", "undo_label"} {
		if _, ok := mutation[key]; !ok {
			t.Fatalf("mutation envelope missing %q: %s", key, data)
		}
	}
	if _, leaked := payload["session_id"]; leaked {
		t.Fatalf("session_id belongs to the outer bridge request, not payload: %s", data)
	}
}
