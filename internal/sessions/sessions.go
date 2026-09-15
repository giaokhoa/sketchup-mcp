package sessions

// ListInput is intentionally empty until the SketchUp bridge is introduced.
type ListInput struct{}

// Session is the minimal public identity for a discovered SketchUp session.
type Session struct {
	SessionID string `json:"session_id" jsonschema:"opaque SketchUp MCP session identifier"`
}

// ListOutput is the structured result of sketchup.sessions.list.
type ListOutput struct {
	Sessions []Session `json:"sessions" jsonschema:"discovered SketchUp desktop sessions"`
}

// List returns no sessions until the SketchUp bridge and discovery layer exist.
func List() ListOutput {
	return ListOutput{Sessions: []Session{}}
}
