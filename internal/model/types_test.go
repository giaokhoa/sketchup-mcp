package model

import "testing"

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
