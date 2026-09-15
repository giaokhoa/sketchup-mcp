package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEntityRefValidation(t *testing.T) {
	t.Parallel()

	valid := EntityRef{
		SessionID:    "11111111-1111-4111-8111-111111111111",
		ModelGUID:    "model-guid",
		PersistentID: 42,
		Revision:     3,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid EntityRef rejected: %v", err)
	}

	tests := []EntityRef{
		{ModelGUID: "model-guid", PersistentID: 42},
		{SessionID: valid.SessionID, PersistentID: 42},
		{SessionID: valid.SessionID, ModelGUID: "model-guid", PersistentID: 0},
		{SessionID: valid.SessionID, ModelGUID: "model-guid", PersistentID: -1},
	}
	for _, ref := range tests {
		if err := ref.Validate(); err == nil {
			t.Fatalf("invalid EntityRef accepted: %#v", ref)
		}
	}
}

func TestZeroRevisionsRemainPresentInToolOutputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		field string
	}{
		{name: "summary", value: SummaryOutput{}, field: `"revision":0`},
		{name: "selection", value: SelectionOutput{}, field: `"revision":0`},
		{name: "inspect", value: InspectOutput{}, field: `"current_revision":0`},
	}
	for _, test := range tests {
		data, err := json.Marshal(test.value)
		if err != nil {
			t.Fatalf("%s marshal failed: %v", test.name, err)
		}
		if !strings.Contains(string(data), test.field) {
			t.Fatalf("%s omitted zero revision: %s", test.name, data)
		}
	}
}
