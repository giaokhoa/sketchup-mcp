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


func TestDeleteInputRequiresMatchingDurableReference(t *testing.T) {
	input := DeleteInput{
		MutationEnvelope: validEnvelope(),
		EntityRef:        validRef(),
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	input.EntityRef.SessionID = "22222222-2222-4222-8222-222222222222"
	err := input.Validate()
	if err == nil || !strings.Contains(err.Error(), "session_id") {
		t.Fatalf("Validate() error = %v, want session mismatch", err)
	}
}

func TestMaterialSetInputValidatesNameAndRGB(t *testing.T) {
	input := MaterialSetInput{
		MutationEnvelope: validEnvelope(),
		EntityRef:        validRef(),
		Material: MaterialSpec{
			Name:  "cabinet-light-wood",
			Color: RGBColor{R: 222, G: 203, B: 176},
		},
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	input.Material.Color.R = 256
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() = nil, want invalid RGB")
	}

	input.Material.Color.R = 222
	input.Material.Name = "   "
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() = nil, want invalid material name")
	}
}

func TestDeleteAndMaterialBridgePayloadsUseMutationEnvelope(t *testing.T) {
	deleteInput := DeleteInput{MutationEnvelope: validEnvelope(), EntityRef: validRef()}
	materialInput := MaterialSetInput{
		MutationEnvelope: validEnvelope(),
		EntityRef:        validRef(),
		Material: MaterialSpec{
			Name:  "bronze",
			Color: RGBColor{R: 145, G: 92, B: 62},
		},
	}

	for name, payloadValue := range map[string]any{
		"delete":   deleteInput.BridgePayload(),
		"material": materialInput.BridgePayload(),
	} {
		data, err := json.Marshal(payloadValue)
		if err != nil {
			t.Fatalf("%s json.Marshal() error = %v", name, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("%s json.Unmarshal() error = %v", name, err)
		}
		if _, ok := payload["mutation"]; !ok {
			t.Fatalf("%s payload missing mutation: %s", name, data)
		}
		if _, ok := payload["entity_ref"]; !ok {
			t.Fatalf("%s payload missing entity_ref: %s", name, data)
		}
		if _, leaked := payload["session_id"]; leaked {
			t.Fatalf("%s session_id leaked into bridge payload: %s", name, data)
		}
	}
}


func TestNameSetInputValidatesNameAndReference(t *testing.T) {
	input := NameSetInput{
		MutationEnvelope: validEnvelope(),
		EntityRef:        validRef(),
		Name:             "Drawer Left Bottom",
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	input.Name = "   "
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() = nil, want empty-name error")
	}

	input.Name = strings.Repeat("x", 129)
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() = nil, want oversized-name error")
	}

	input.Name = "Door 1"
	input.EntityRef.Revision = 6
	err := input.Validate()
	if err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("Validate() error = %v, want revision mismatch", err)
	}
}

func TestNameSetBridgePayloadUsesMutationEnvelope(t *testing.T) {
	input := NameSetInput{
		MutationEnvelope: validEnvelope(),
		EntityRef:        validRef(),
		Name:             "Side Left",
	}
	data, err := json.Marshal(input.BridgePayload())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{"mutation", "entity_ref", "name"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %q: %s", key, data)
		}
	}
	if _, leaked := payload["session_id"]; leaked {
		t.Fatalf("session_id leaked into bridge payload: %s", data)
	}
}


func TestAssemblyCreateInputValidatesSiblingReferenceEnvelopeShape(t *testing.T) {
	second := validRef()
	second.PersistentID = 43
	input := AssemblyCreateInput{
		MutationEnvelope: validEnvelope(),
		Name:             "Drawer Assembly - Left",
		Children:         []EntityRef{validRef(), second},
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	input.Children[1].PersistentID = 42
	if err := input.Validate(); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("Validate() error = %v, want duplicate PID error", err)
	}

	input.Children[1] = second
	input.Children[1].Revision = 6
	if err := input.Validate(); err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("Validate() error = %v, want revision mismatch", err)
	}

	input.Children[1] = second
	input.Name = " "
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() = nil, want blank-name error")
	}
}

func TestAssemblyCreateBridgePayloadUsesMutationEnvelope(t *testing.T) {
	second := validRef()
	second.PersistentID = 43
	input := AssemblyCreateInput{
		MutationEnvelope: validEnvelope(),
		Name:             "Carcass",
		Children:         []EntityRef{validRef(), second},
	}
	data, err := json.Marshal(input.BridgePayload())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{"mutation", "name", "children"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %q: %s", key, data)
		}
	}
	if _, leaked := payload["session_id"]; leaked {
		t.Fatalf("session_id leaked into bridge payload: %s", data)
	}
}
