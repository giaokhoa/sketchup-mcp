package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ModelBounds struct {
	MinMm    Vec3
	MaxMm    Vec3
	SizeMm   Vec3
	CenterMm Vec3
}

type SlotMeta struct {
	Id           string
	PageIndex    int
	BoundsMm     Rect
	DefaultScale float64
	Perspective  bool
}

type Runner struct {
	cfg          Config
	fixture      Fixture
	report       *Report
	session      *mcp.ClientSession
	stderr       bytes.Buffer
	sessionId    string
	pid          int
	modelGuid    string
	revision     uint64
	bounds       ModelBounds
	slots        map[string]SlotMeta
	sectionRefs  map[string]map[string]any
	viewportRefs map[string]map[string]any
	opCounter    int
}

func newRunner(cfg Config, fixture Fixture, report *Report) *Runner {
	return &Runner{
		cfg:          cfg,
		fixture:      fixture,
		report:       report,
		slots:        map[string]SlotMeta{},
		sectionRefs:  map[string]map[string]any{},
		viewportRefs: map[string]map[string]any{},
	}
}

func (r *Runner) Close() {
	if r.session != nil {
		_ = r.session.Close()
	}
}

func (r *Runner) connect(ctx context.Context) error {
	command := exec.Command(r.cfg.McpExe)
	command.Stderr = &r.stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "live-layout-e2e", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return fmt.Errorf("connect packaged MCP: %w; stderr=%q", err, r.stderr.String())
	}
	r.session = session
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list MCP tools: %w", err)
	}
	found := map[string]bool{}
	for _, tool := range list.Tools {
		found[tool.Name] = true
	}
	required := []string{
		"sketchup.sessions.list", "model.summary", "model.bounds",
		"section_plane.create", "scene.create", "model.file.save_copy",
		"layout.template.inspect", "layout.document.create", "layout.viewport.add",
		"layout.dimension.add", "layout.text.add", "layout.line.add",
		"layout.rectangle.add", "layout.panel.validate", "layout.export",
	}
	for _, name := range required {
		if !found[name] {
			return fmt.Errorf("required MCP tool %q is missing", name)
		}
	}
	return nil
}

func (r *Runner) call(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, map[string]any, error) {
	callCtx, cancel := context.WithTimeout(ctx, r.cfg.ToolTimeout)
	defer cancel()
	result, err := r.session.CallTool(callCtx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, nil, fmt.Errorf("%s transport: %w", name, err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return result, nil, fmt.Errorf("%s structured content encode: %w", name, err)
	}
	out := map[string]any{}
	if string(data) != "null" {
		if err := json.Unmarshal(data, &out); err != nil {
			return result, nil, fmt.Errorf("%s structured content decode: %w; raw=%s", name, err, data)
		}
	}
	if result.IsError {
		return result, out, fmt.Errorf("%s returned tool error: %s", name, data)
	}
	return result, out, nil
}

func (r *Runner) discoverTarget(ctx context.Context) error {
	_, out, err := r.call(ctx, "sketchup.sessions.list", map[string]any{})
	if err != nil {
		return err
	}
	items, err := sliceField(out, "sessions")
	if err != nil {
		return err
	}
	for _, raw := range items {
		session, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := stringField(session, "session_id")
		pid, _ := intField(session, "pid")
		match := false
		if r.cfg.SessionId != "" {
			match = id == r.cfg.SessionId
		} else {
			match = pid == r.cfg.Pid
		}
		if !match {
			continue
		}
		version, _ := stringField(session, "sketchup_version")
		r.sessionId = id
		r.pid = pid
		r.report.SessionId = id
		r.report.SketchUpPid = pid
		r.report.SketchUpVersion = version
		break
	}
	if r.sessionId == "" {
		return fmt.Errorf("target SketchUp session not found")
	}
	summary, err := r.readSummary(ctx)
	if err != nil {
		return err
	}
	modelPath, _ := stringField(summary, "path")
	if !samePath(modelPath, r.cfg.SourceModel) {
		return fmt.Errorf("open model path %q does not match expected source model %q", modelPath, r.cfg.SourceModel)
	}
	r.report.ModelGuid = r.modelGuid
	r.report.StartRevision = r.revision
	return nil
}

func (r *Runner) readSummary(ctx context.Context) (map[string]any, error) {
	_, out, err := r.call(ctx, "model.summary", map[string]any{"session_id": r.sessionId})
	if err != nil {
		return nil, err
	}
	guid, err := stringField(out, "model_guid")
	if err != nil || guid == "" {
		return nil, fmt.Errorf("model.summary missing model_guid")
	}
	revision, err := uintField(out, "revision")
	if err != nil {
		return nil, err
	}
	if r.modelGuid != "" && guid != r.modelGuid {
		return nil, fmt.Errorf("model GUID changed from %s to %s", r.modelGuid, guid)
	}
	if revision < r.revision {
		return nil, fmt.Errorf("model revision regressed from %d to %d", r.revision, revision)
	}
	r.modelGuid = guid
	r.revision = revision
	return out, nil
}

func (r *Runner) readBounds(ctx context.Context) error {
	_, out, err := r.call(ctx, "model.bounds", map[string]any{"session_id": r.sessionId})
	if err != nil {
		return err
	}
	guid, _ := stringField(out, "model_guid")
	revision, _ := uintField(out, "revision")
	if guid != r.modelGuid || revision != r.revision {
		return fmt.Errorf("model.bounds identity mismatch guid=%q revision=%d", guid, revision)
	}
	r.bounds = ModelBounds{
		MinMm:    vec3Field(out, "min_mm"),
		MaxMm:    vec3Field(out, "max_mm"),
		SizeMm:   vec3Field(out, "size_mm"),
		CenterMm: vec3Field(out, "center_mm"),
	}
	want := r.fixture.ExpectedModel.SizeMm
	got := r.bounds.SizeMm
	tol := r.fixture.ExpectedModel.ToleranceMm
	if math.Abs(got.X-want.X) > tol || math.Abs(got.Y-want.Y) > tol || math.Abs(got.Z-want.Z) > tol {
		return fmt.Errorf("model bounds size %.3fx%.3fx%.3f mm differs from expected %.3fx%.3fx%.3f mm", got.X, got.Y, got.Z, want.X, want.Y, want.Z)
	}
	return nil
}

func (r *Runner) inspectTemplate(ctx context.Context) error {
	_, out, err := r.call(ctx, "layout.template.inspect", map[string]any{
		"session_id":    r.sessionId,
		"template_path": r.cfg.Template,
	})
	if err != nil {
		return err
	}
	schema, _ := intField(out, "schema_version")
	if schema != r.fixture.RequiredTemplate.SchemaVersion {
		return fmt.Errorf("template schema_version=%d want=%d", schema, r.fixture.RequiredTemplate.SchemaVersion)
	}
	kind, _ := stringField(out, "template_kind")
	if r.fixture.RequiredTemplate.TemplateKind != "" && kind != r.fixture.RequiredTemplate.TemplateKind {
		return fmt.Errorf("template_kind=%q want=%q", kind, r.fixture.RequiredTemplate.TemplateKind)
	}
	slotIDs := map[string]bool{}
	slotItems, err := sliceField(out, "slots")
	if err != nil {
		return err
	}
	for _, raw := range slotItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := stringField(item, "slot_id")
		boundsMap, _ := mapField(item, "bounds_mm")
		slot := SlotMeta{
			Id:           id,
			PageIndex:    mustInt(item, "page_index"),
			BoundsMm:     rectFromMap(boundsMap),
			DefaultScale: mustFloat(item, "default_scale_denominator"),
			Perspective:  mustBool(item, "perspective"),
		}
		if id != "" {
			slotIDs[id] = true
			r.slots[id] = slot
		}
	}
	if err := requireIDs("template slot", slotIDs, r.fixture.RequiredTemplate.Slots); err != nil {
		return err
	}
	panelIDs := collectIDs(out, "panels", "panel_id")
	if err := requireIDs("template panel", panelIDs, r.fixture.RequiredTemplate.Panels); err != nil {
		return err
	}
	styleIDs := collectIDs(out, "styles", "style_id")
	if err := requireIDs("template style", styleIDs, r.fixture.RequiredTemplate.Styles); err != nil {
		return err
	}
	layerIDs := collectIDs(out, "layers", "name")
	if err := requireIDs("template layer", layerIDs, r.fixture.RequiredTemplate.Layers); err != nil {
		return err
	}
	return nil
}

func (r *Runner) callMutation(ctx context.Context, name string, fields map[string]any, expectAdvance bool) (*mcp.CallToolResult, map[string]any, error) {
	before := r.revision
	args := map[string]any{
		"session_id":          r.sessionId,
		"operation_id":        r.nextOperationId(name),
		"expected_model_guid": r.modelGuid,
		"expected_revision":   before,
	}
	for key, value := range fields {
		args[key] = value
	}
	result, out, err := r.call(ctx, name, args)
	if err != nil {
		return result, out, err
	}
	guid, err := stringField(out, "model_guid")
	if err != nil || guid != r.modelGuid {
		return result, out, fmt.Errorf("%s returned unexpected model_guid %q", name, guid)
	}
	revision, err := uintField(out, "revision")
	if err != nil {
		return result, out, err
	}
	want := before
	if expectAdvance {
		want++
	}
	if revision != want {
		return result, out, fmt.Errorf("%s returned revision %d want %d", name, revision, want)
	}
	if expectAdvance {
		if _, err := r.readSummary(ctx); err != nil {
			return result, out, err
		}
		if r.revision != revision {
			return result, out, fmt.Errorf("%s summary revision=%d output revision=%d", name, r.revision, revision)
		}
	}
	return result, out, nil
}

func (r *Runner) preparePresentation(ctx context.Context) error {
	for _, step := range r.fixture.Presentation {
		switch strings.ToLower(step.Kind) {
		case "section":
			spec := step.Section
			_, out, err := r.callMutation(ctx, "section_plane.create", map[string]any{
				"point_mm": vec3Map(spec.PointMm),
				"normal":   vec3Map(spec.Normal),
				"name":     spec.Name,
				"symbol":   spec.Symbol,
			}, true)
			if err != nil {
				return fmt.Errorf("section %s: %w", step.Id, err)
			}
			ref, err := mapField(out, "entity_ref")
			if err != nil {
				return fmt.Errorf("section %s missing entity_ref: %w", step.Id, err)
			}
			r.sectionRefs[step.Id] = ref
		case "scene":
			spec := step.Scene
			var sectionRef any
			if spec.SectionId != "" {
				ref := r.sectionRefs[spec.SectionId]
				if ref == nil {
					return fmt.Errorf("scene %q missing section ref %q", spec.Name, spec.SectionId)
				}
				sectionRef = ref
			}
			_, _, err := r.callMutation(ctx, "scene.create", map[string]any{
				"name":                   spec.Name,
				"eye_mm":                 vec3Map(spec.EyeMm),
				"target_mm":              vec3Map(spec.TargetMm),
				"up":                     vec3Map(spec.Up),
				"perspective":            spec.Perspective,
				"orthographic_height_mm": spec.OrthographicHeightMm,
				"fov_degrees":            spec.FovDegrees,
				"section_plane_ref":      sectionRef,
				"display_section_plane":  spec.DisplaySectionPlane,
			}, true)
			if err != nil {
				return fmt.Errorf("scene %q: %w", spec.Name, err)
			}
		}
	}
	return nil
}

func (r *Runner) saveModelCopy(ctx context.Context) error {
	path := r.outputPath(".skp")
	_, out, err := r.callMutation(ctx, "model.file.save_copy", map[string]any{"output_path": path}, false)
	if err != nil {
		return err
	}
	got, _ := stringField(out, "output_path")
	if !samePath(got, path) {
		return fmt.Errorf("save_copy output_path=%q want=%q", got, path)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("saved model copy missing: %w", err)
	}
	r.report.OutputPaths["skp"] = path
	return nil
}

func (r *Runner) createLayout(ctx context.Context) error {
	path := r.outputPath(".layout")
	_, out, err := r.callMutation(ctx, "layout.document.create", map[string]any{
		"layout_path":    path,
		"template_path":  r.cfg.Template,
		"page_width_mm":  0.0,
		"page_height_mm": 0.0,
	}, false)
	if err != nil {
		return err
	}
	got, _ := stringField(out, "layout_path")
	if !samePath(got, path) {
		return fmt.Errorf("layout.document.create path=%q want=%q", got, path)
	}
	r.report.OutputPaths["layout"] = path
	return nil
}

func (r *Runner) addViewports(ctx context.Context) error {
	layout := r.outputPath(".layout")
	skp := r.outputPath(".skp")
	for _, spec := range r.fixture.Viewports {
		slot, ok := r.slots[spec.SlotId]
		if !ok {
			return fmt.Errorf("viewport %q references missing slot %q", spec.Id, spec.SlotId)
		}
		fields := map[string]any{
			"layout_path":       layout,
			"skp_path":          skp,
			"page_index":        slot.PageIndex,
			"layer_name":        spec.LayerName,
			"bounds_mm":         rectMap(slot.BoundsMm),
			"scene_name":        spec.SceneName,
			"standard_view":     spec.StandardView,
			"perspective":       spec.Perspective,
			"scale_denominator": spec.ScaleDenominator,
			"render_mode":       spec.RenderMode,
			"panel_id":          spec.PanelId,
			"require_fit":       true,
			"fit_margin_mm":     spec.FitMarginMm,
			"fit_model_bounds_mm": map[string]any{
				"min_mm": vec3Map(r.bounds.MinMm),
				"max_mm": vec3Map(r.bounds.MaxMm),
			},
		}
		_, out, err := r.callMutation(ctx, "layout.viewport.add", fields, false)
		if err != nil {
			return fmt.Errorf("viewport %s: %w", spec.Id, err)
		}
		if !mustBool(out, "fit_checked") || !mustBool(out, "fits_bounds") {
			return fmt.Errorf("viewport %s did not pass require_fit", spec.Id)
		}
		ref, err := mapField(out, "entity_ref")
		if err != nil {
			return fmt.Errorf("viewport %s missing entity_ref: %w", spec.Id, err)
		}
		r.viewportRefs[spec.Id] = ref
	}
	return nil
}

func (r *Runner) addAnnotations(ctx context.Context) error {
	layout := r.outputPath(".layout")
	for _, spec := range r.fixture.Texts {
		_, _, err := r.callMutation(ctx, "layout.text.add", map[string]any{
			"layout_path":  layout,
			"page_index":   spec.PageIndex,
			"layer_name":   spec.LayerName,
			"bounds_mm":    rectMap(spec.BoundsMm),
			"text":         spec.Text,
			"font_size_pt": spec.FontSizePt,
			"bold":         spec.Bold,
			"alignment":    spec.Alignment,
			"style_id":     spec.StyleId,
			"panel_id":     spec.PanelId,
		}, false)
		if err != nil {
			return fmt.Errorf("text %s: %w", spec.Id, err)
		}
	}
	for _, spec := range r.fixture.Lines {
		_, _, err := r.callMutation(ctx, "layout.line.add", map[string]any{
			"layout_path":  layout,
			"page_index":   spec.PageIndex,
			"layer_name":   spec.LayerName,
			"start_mm":     vec2Map(spec.StartMm),
			"end_mm":       vec2Map(spec.EndMm),
			"stroke_width": spec.StrokeWidth,
			"style_id":     spec.StyleId,
			"panel_id":     spec.PanelId,
		}, false)
		if err != nil {
			return fmt.Errorf("line %s: %w", spec.Id, err)
		}
	}
	for _, spec := range r.fixture.Rectangles {
		_, _, err := r.callMutation(ctx, "layout.rectangle.add", map[string]any{
			"layout_path":  layout,
			"page_index":   spec.PageIndex,
			"layer_name":   spec.LayerName,
			"bounds_mm":    rectMap(spec.BoundsMm),
			"stroke_width": spec.StrokeWidth,
			"style_id":     spec.StyleId,
			"panel_id":     spec.PanelId,
		}, false)
		if err != nil {
			return fmt.Errorf("rectangle %s: %w", spec.Id, err)
		}
	}
	return nil
}

func (r *Runner) addDimensions(ctx context.Context) error {
	layout := r.outputPath(".layout")
	for _, spec := range r.fixture.Dimensions {
		ref := r.viewportRefs[spec.ViewportId]
		if ref == nil {
			return fmt.Errorf("dimension %s references missing viewport %s", spec.Id, spec.ViewportId)
		}
		r.report.DimensionCount++
		_, out, err := r.callMutation(ctx, "layout.dimension.add", map[string]any{
			"layout_path":    layout,
			"page_index":     spec.PageIndex,
			"layer_name":     spec.LayerName,
			"viewport_ref":   ref,
			"start_point_mm": vec3Map(spec.StartPointMm),
			"end_point_mm":   vec3Map(spec.EndPointMm),
			"start_pid_path": spec.StartPidPath,
			"end_pid_path":   spec.EndPidPath,
			"offset_mm":      spec.OffsetMm,
			"alignment":      spec.Alignment,
			"style_id":       spec.StyleId,
			"panel_id":       spec.PanelId,
		}, false)
		if err != nil {
			return fmt.Errorf("dimension %s: %w", spec.Id, err)
		}
		if !mustBool(out, "connected") {
			return fmt.Errorf("dimension %s returned connected=false", spec.Id)
		}
		r.report.ConnectedDimensionCount++
	}
	return nil
}

func (r *Runner) validatePanels(ctx context.Context) error {
	layout := r.outputPath(".layout")
	for _, panelId := range populatedPanels(r.fixture) {
		pageIndex, err := r.panelPageIndex(panelId)
		if err != nil {
			return err
		}
		_, out, err := r.call(ctx, "layout.panel.validate", map[string]any{
			"session_id":  r.sessionId,
			"layout_path": layout,
			"page_index":  pageIndex,
			"panel_id":    panelId,
			"margin_mm":   r.fixture.PanelMarginMm,
		})
		if err != nil {
			return fmt.Errorf("panel %s: %w", panelId, err)
		}
		fits := mustBool(out, "fits")
		entityCount := mustInt(out, "entity_count")
		violations, _ := sliceField(out, "violations")
		summary := PanelSummary{PanelId: panelId, PageIndex: pageIndex, EntityCount: entityCount, Fits: fits, ViolationCount: len(violations)}
		r.report.PanelValidations = append(r.report.PanelValidations, summary)
		if !fits || len(violations) != 0 {
			data, _ := json.Marshal(violations)
			return fmt.Errorf("panel %s containment failed: %s", panelId, data)
		}
	}
	return nil
}

func (r *Runner) exportOutputs(ctx context.Context) error {
	layout := r.outputPath(".layout")
	png := r.outputPath(".png")
	result, out, err := r.callMutation(ctx, "layout.export", map[string]any{
		"layout_path": layout,
		"output_path": png,
		"dpi":         r.fixture.ExportDpi,
	}, false)
	if err != nil {
		return err
	}
	mime, _ := stringField(out, "mime_type")
	if mime != "image/png" {
		return fmt.Errorf("PNG export mime_type=%q", mime)
	}
	var image *mcp.ImageContent
	for _, content := range result.Content {
		if item, ok := content.(*mcp.ImageContent); ok {
			image = item
			break
		}
	}
	if image == nil || image.MIMEType != "image/png" || !bytes.HasPrefix(image.Data, []byte{0x89, 'P', 'N', 'G'}) {
		return fmt.Errorf("PNG response is not native ImageContent with PNG signature")
	}
	if info, err := os.Stat(png); err != nil || info.Size() == 0 {
		return fmt.Errorf("PNG output missing or empty")
	}
	r.report.OutputPaths["png"] = png
	r.report.PngBytes = len(image.Data)

	pdf := r.outputPath(".pdf")
	_, out, err = r.callMutation(ctx, "layout.export", map[string]any{
		"layout_path": layout,
		"output_path": pdf,
		"dpi":         0,
	}, false)
	if err != nil {
		return err
	}
	if got, _ := stringField(out, "output_path"); !samePath(got, pdf) {
		return fmt.Errorf("PDF export path=%q want=%q", got, pdf)
	}
	data, err := os.ReadFile(pdf)
	if err != nil || len(data) < 4 || string(data[:4]) != "%PDF" {
		return fmt.Errorf("PDF file was not created with a valid PDF signature")
	}
	r.report.OutputPaths["pdf"] = pdf
	return nil
}

func (r *Runner) checkResponsive() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("live responsiveness check requires Windows")
	}
	script := fmt.Sprintf("$p=Get-Process -Id %d -ErrorAction Stop; if (-not $p.Responding) { exit 2 }; Write-Output $p.ProcessName", r.pid)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("SketchUp PID %d is missing or unresponsive: %v (%s)", r.pid, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *Runner) panelPageIndex(panelId string) (int, error) {
	found := false
	page := 0
	accept := func(candidate int) error {
		if !found {
			page = candidate
			found = true
			return nil
		}
		if page != candidate {
			return fmt.Errorf("panel %s spans multiple pages (%d and %d)", panelId, page, candidate)
		}
		return nil
	}
	for _, v := range r.fixture.Viewports {
		if v.PanelId == panelId {
			if err := accept(r.slots[v.SlotId].PageIndex); err != nil {
				return 0, err
			}
		}
	}
	for _, v := range r.fixture.Dimensions {
		if v.PanelId == panelId {
			if err := accept(v.PageIndex); err != nil {
				return 0, err
			}
		}
	}
	for _, v := range r.fixture.Texts {
		if v.PanelId == panelId {
			if err := accept(v.PageIndex); err != nil {
				return 0, err
			}
		}
	}
	for _, v := range r.fixture.Lines {
		if v.PanelId == panelId {
			if err := accept(v.PageIndex); err != nil {
				return 0, err
			}
		}
	}
	for _, v := range r.fixture.Rectangles {
		if v.PanelId == panelId {
			if err := accept(v.PageIndex); err != nil {
				return 0, err
			}
		}
	}
	if !found {
		return 0, fmt.Errorf("panel %s has no populated entities", panelId)
	}
	return page, nil
}

func (r *Runner) nextOperationId(name string) string {
	r.opCounter++
	clean := strings.NewReplacer(".", "-", "_", "-").Replace(name)
	return fmt.Sprintf("live-e2e-%d-%03d-%s", r.report.StartedAt.UnixNano(), r.opCounter, clean)
}

func (r *Runner) outputPath(ext string) string {
	return filepath.Join(r.cfg.OutputDir, r.fixture.OutputBase+ext)
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(filepath.Clean(a))
	bb, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}

func requireIDs(kind string, found map[string]bool, required []string) error {
	for _, id := range required {
		if !found[id] {
			return fmt.Errorf("missing required %s metadata %q", kind, id)
		}
	}
	return nil
}

func collectIDs(root map[string]any, listKey, idKey string) map[string]bool {
	out := map[string]bool{}
	items, _ := sliceField(root, listKey)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := stringField(item, idKey)
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func vec2Map(v Vec2) map[string]any { return map[string]any{"x": v.X, "y": v.Y} }
func vec3Map(v Vec3) map[string]any { return map[string]any{"x": v.X, "y": v.Y, "z": v.Z} }
func rectMap(v Rect) map[string]any {
	return map[string]any{"x": v.X, "y": v.Y, "width": v.Width, "height": v.Height}
}
func rectFromMap(v map[string]any) Rect {
	return Rect{X: mustFloat(v, "x"), Y: mustFloat(v, "y"), Width: mustFloat(v, "width"), Height: mustFloat(v, "height")}
}
func vec3Field(root map[string]any, key string) Vec3 {
	item, _ := mapField(root, key)
	return Vec3{X: mustFloat(item, "x"), Y: mustFloat(item, "y"), Z: mustFloat(item, "z")}
}

func mapField(root map[string]any, key string) (map[string]any, error) {
	value, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("missing field %q", key)
	}
	item, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an object", key)
	}
	return item, nil
}

func sliceField(root map[string]any, key string) ([]any, error) {
	value, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("missing field %q", key)
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an array", key)
	}
	return items, nil
}

func stringField(root map[string]any, key string) (string, error) {
	value, ok := root[key]
	if !ok {
		return "", fmt.Errorf("missing field %q", key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string", key)
	}
	return text, nil
}

func intField(root map[string]any, key string) (int, error) {
	value, ok := root[key]
	if !ok {
		return 0, fmt.Errorf("missing field %q", key)
	}
	switch number := value.(type) {
	case float64:
		return int(number), nil
	case json.Number:
		v, err := strconv.Atoi(number.String())
		return v, err
	default:
		return 0, fmt.Errorf("field %q is not a number", key)
	}
}

func uintField(root map[string]any, key string) (uint64, error) {
	value, ok := root[key]
	if !ok {
		return 0, fmt.Errorf("missing field %q", key)
	}
	switch number := value.(type) {
	case float64:
		if number < 0 {
			return 0, fmt.Errorf("field %q is negative", key)
		}
		return uint64(number), nil
	case json.Number:
		return strconv.ParseUint(number.String(), 10, 64)
	default:
		return 0, fmt.Errorf("field %q is not a number", key)
	}
}

func boolField(root map[string]any, key string) (bool, error) {
	value, ok := root[key]
	if !ok {
		return false, fmt.Errorf("missing field %q", key)
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("field %q is not a bool", key)
	}
	return flag, nil
}

func mustInt(root map[string]any, key string) int { value, _ := intField(root, key); return value }
func mustFloat(root map[string]any, key string) float64 {
	value, ok := root[key]
	if !ok {
		return 0
	}
	number, _ := value.(float64)
	return number
}
func mustBool(root map[string]any, key string) bool { value, _ := boolField(root, key); return value }

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
