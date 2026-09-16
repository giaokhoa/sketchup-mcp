//go:build windows

package layoutworker

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
)

const (
	documentVersion2023 = 23

	documentUnitsDecimalMillimeters = 3
	dimensionUnitsDecimalMillimeters = 5
	dimensionRotationHorizontal      = 0
	dimensionVerticalBelow           = 2

	imageResolutionHigh = 2
	imageFormatPNG       = 0

	renderModeHybrid = 1

	formattedTextAnchorCenterCenter = 7
	textAlignmentCenter             = 2
)

type loRef struct {
	Ptr uintptr
}

type loPoint2D struct {
	X float64
	Y float64
}

type loPoint3D struct {
	X float64
	Y float64
	Z float64
}

type loRect struct {
	UpperLeft  loPoint2D
	LowerRight loPoint2D
}

type loAPI struct {
	dll     *syscall.LazyDLL
	runtime string
	major   uintptr
	minor   uintptr
}

func Generate(ctx context.Context, spec model.LayoutDrawingSpec) (model.LayoutA3SheetOutput, error) {
	var output model.LayoutA3SheetOutput
	if err := ValidateSpec(spec); err != nil {
		return output, err
	}
	if err := ctx.Err(); err != nil {
		return output, err
	}
	if err := os.MkdirAll(spec.OutputDirectory, 0o755); err != nil {
		return output, fmt.Errorf("create output directory: %w", err)
	}

	api, err := loadLayOutAPI()
	if err != nil {
		return output, err
	}
	api.proc("LOInitialize").Call()
	defer api.proc("LOTerminate").Call()
	api.proc("LOGetAPIVersion").Call(
		uintptr(unsafe.Pointer(&api.major)),
		uintptr(unsafe.Pointer(&api.minor)),
	)
	if api.major < 11 {
		return output, fmt.Errorf("LayOut runtime API %d.%d is older than validated API 11.0", api.major, api.minor)
	}

	layoutPath, pdfPath, pngBase, qaPath := outputPaths(spec)
	for _, path := range []string{layoutPath, pdfPath, qaPath} {
		if _, err := os.Stat(path); err == nil {
			return output, fmt.Errorf("refusing to overwrite existing output %s", path)
		} else if !os.IsNotExist(err) {
			return output, err
		}
	}

	var doc loRef
	if err := api.call("LODocumentCreateEmpty", uintptr(unsafe.Pointer(&doc.Ptr))); err != nil {
		return output, err
	}
	defer api.proc("LODocumentRelease").Call(uintptr(unsafe.Pointer(&doc.Ptr)))

	var pageInfo loRef
	if err := api.call("LODocumentGetPageInfo", doc.Ptr, uintptr(unsafe.Pointer(&pageInfo.Ptr))); err != nil {
		return output, err
	}
	if err := api.call("LOPageInfoSetWidth", pageInfo.Ptr, floatArg(spec.PageWidthMM/25.4)); err != nil {
		return output, err
	}
	if err := api.call("LOPageInfoSetHeight", pageInfo.Ptr, floatArg(spec.PageHeightMM/25.4)); err != nil {
		return output, err
	}
	_ = api.call("LOPageInfoSetShowMargins", pageInfo.Ptr, 0)
	_ = api.call("LOPageInfoSetPrintMargins", pageInfo.Ptr, 0)
	_ = api.call("LOPageInfoSetOutputResolution", pageInfo.Ptr, imageResolutionHigh)
	if err := api.call("LODocumentSetUnits", doc.Ptr, documentUnitsDecimalMillimeters, floatArg(1.0)); err != nil {
		return output, err
	}

	if err := addSheetFrame(api, doc, spec.PageWidthMM/25.4, spec.PageHeightMM/25.4); err != nil {
		return output, err
	}

	viewRefs := make(map[string]loRef, len(spec.Views))
	viewTitles := make([]string, 0, len(spec.Views))
	for _, view := range spec.Views {
		if err := ctx.Err(); err != nil {
			return output, err
		}
		rect := loRect{
			UpperLeft:  loPoint2D{X: view.Rect.Left, Y: view.Rect.Top},
			LowerRight: loPoint2D{X: view.Rect.Right, Y: view.Rect.Bottom},
		}
		var viewport loRef
		if err := api.call(
			"LOSketchUpModelCreate",
			uintptr(unsafe.Pointer(&viewport.Ptr)),
			cString(spec.SnapshotPath),
			uintptr(unsafe.Pointer(&rect)),
		); err != nil {
			return output, fmt.Errorf("create viewport %s: %w", view.ID, err)
		}
		if err := api.call("LOSketchUpModelSetCurrentScene", viewport.Ptr, uintptr(view.SceneIndex)); err != nil {
			return output, fmt.Errorf("set scene for %s: %w", view.ID, err)
		}
		perspective := uintptr(0)
		if view.Perspective {
			perspective = 1
		}
		if err := api.call("LOSketchUpModelSetPerspective", viewport.Ptr, perspective); err != nil {
			return output, fmt.Errorf("set perspective for %s: %w", view.ID, err)
		}
		if !view.Perspective {
			_ = api.call("LOSketchUpModelSetPreserveScaleOnResize", viewport.Ptr, 1)
			if err := api.call("LOSketchUpModelSetScale", viewport.Ptr, floatArg(view.Scale)); err != nil {
				return output, fmt.Errorf("set scale for %s: %w", view.ID, err)
			}
		}
		_ = api.call("LOSketchUpModelSetRenderMode", viewport.Ptr, renderModeHybrid)

		entity := api.returnRef("LOSketchUpModelToEntity", viewport.Ptr)
		if entity.Ptr == 0 {
			return output, fmt.Errorf("viewport %s converted to invalid entity", view.ID)
		}
		if err := api.call("LODocumentAddEntityUsingIndexes", doc.Ptr, entity.Ptr, 0, 0); err != nil {
			return output, fmt.Errorf("add viewport %s: %w", view.ID, err)
		}
		_ = api.call("LOSketchUpModelRender", viewport.Ptr)
		viewRefs[view.ID] = viewport
		viewTitles = append(viewTitles, view.Title)

		if err := addViewLabel(api, doc, view); err != nil {
			return output, fmt.Errorf("add view label %s: %w", view.ID, err)
		}
	}

	dimStyle, err := createDimensionStyle(api)
	if err != nil {
		return output, err
	}
	defer api.proc("LOStyleRelease").Call(uintptr(unsafe.Pointer(&dimStyle.Ptr)))

	for _, dim := range spec.Dimensions {
		if err := ctx.Err(); err != nil {
			return output, err
		}
		viewport, ok := viewRefs[dim.ViewID]
		if !ok {
			return output, fmt.Errorf("dimension %s references missing viewport %s", dim.ID, dim.ViewID)
		}
		if err := addAssociativeDimension(api, doc, viewport, dimStyle, dim); err != nil {
			return output, fmt.Errorf("dimension %s: %w", dim.ID, err)
		}
	}

	if err := addNotes(api, doc, spec.Notes, spec.PageWidthMM/25.4, spec.PageHeightMM/25.4); err != nil {
		return output, err
	}

	if err := api.call("LODocumentSaveToFile", doc.Ptr, cString(layoutPath), documentVersion2023); err != nil {
		return output, err
	}
	var invalid loRef
	if err := api.call("LODocumentExportToPDF", doc.Ptr, cString(pdfPath), invalid.Ptr); err != nil {
		return output, err
	}
	if err := api.call(
		"LODocumentExportToImageSet",
		doc.Ptr,
		cString(spec.OutputDirectory),
		cString(pngBase),
		imageFormatPNG,
		invalid.Ptr,
	); err != nil {
		return output, err
	}

	pngPath, err := findExportedPNG(spec.OutputDirectory, pngBase)
	if err != nil {
		return output, err
	}

	checks := []model.LayoutQACheck{
		{Name: "runtime_api", Passed: api.major >= 11, Details: fmt.Sprintf("%d.%d at %s", api.major, api.minor, api.runtime)},
		{Name: "page_a3_landscape", Passed: closeEnough(spec.PageWidthMM, 420) && closeEnough(spec.PageHeightMM, 297), Details: fmt.Sprintf("%.1f x %.1f mm", spec.PageWidthMM, spec.PageHeightMM)},
		{Name: "six_viewports", Passed: len(spec.Views) == 6, Details: fmt.Sprintf("%d viewports", len(spec.Views))},
		{Name: "associative_dimensions", Passed: len(spec.Dimensions) > 0, Details: fmt.Sprintf("%d PID-connected dimensions", len(spec.Dimensions))},
		fileCheck("layout_file", layoutPath),
		fileCheck("pdf_file", pdfPath),
		fileCheck("png_preview", pngPath),
	}
	report := model.LayoutQAReport{Passed: true, Checks: checks}
	for _, check := range checks {
		if !check.Passed {
			report.Passed = false
			break
		}
	}
	if err := writeQA(qaPath, report); err != nil {
		return output, fmt.Errorf("write QA report: %w", err)
	}

	output = model.LayoutA3SheetOutput{
		SKPPath:        spec.SnapshotPath,
		LayOutPath:     layoutPath,
		PDFPath:        pdfPath,
		PNGPath:        pngPath,
		QAPath:         qaPath,
		PageWidthMM:    spec.PageWidthMM,
		PageHeightMM:   spec.PageHeightMM,
		ViewportCount:  len(spec.Views),
		DimensionCount: len(spec.Dimensions),
		Scenes:         viewTitles,
		QA:             report,
	}
	return output, nil
}

func loadLayOutAPI() (*loAPI, error) {
	runtime := strings.TrimSpace(os.Getenv("SKETCHUP_MCP_LAYOUT_RUNTIME"))
	if runtime == "" {
		matches, _ := filepath.Glob(`C:\Program Files\SketchUp\SketchUp *\SketchUp\LayOutAPI.dll`)
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		if len(matches) == 0 {
			return nil, fmt.Errorf("LayOutAPI.dll not found; set SKETCHUP_MCP_LAYOUT_RUNTIME")
		}
		runtime = filepath.Dir(matches[0])
	}
	dllPath := filepath.Join(runtime, "LayOutAPI.dll")
	if _, err := os.Stat(dllPath); err != nil {
		return nil, fmt.Errorf("LayOut runtime not found at %s: %w", dllPath, err)
	}
	_ = os.Setenv("PATH", runtime+";"+os.Getenv("PATH"))
	dll := syscall.NewLazyDLL(dllPath)
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("load LayOutAPI.dll: %w", err)
	}
	return &loAPI{dll: dll, runtime: runtime}, nil
}

func (a *loAPI) proc(name string) *syscall.LazyProc {
	return a.dll.NewProc(name)
}

func (a *loAPI) call(name string, args ...uintptr) error {
	r, _, callErr := a.proc(name).Call(args...)
	if r != 0 {
		return fmt.Errorf("%s failed: SUResult=%d (%v)", name, r, callErr)
	}
	return nil
}

func (a *loAPI) returnRef(name string, args ...uintptr) loRef {
	r, _, _ := a.proc(name).Call(args...)
	return loRef{Ptr: r}
}

func cString(value string) uintptr {
	p, err := syscall.BytePtrFromString(value)
	if err != nil {
		panic(err)
	}
	return uintptr(unsafe.Pointer(p))
}

func floatArg(value float64) uintptr {
	return uintptr(math.Float64bits(value))
}

func closeEnough(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}

func addSheetFrame(api *loAPI, doc loRef, width, height float64) error {
	margin := 0.14
	if err := addRectangle(api, doc, loRect{
		UpperLeft:  loPoint2D{margin, margin},
		LowerRight: loPoint2D{width - margin, height - margin},
	}); err != nil {
		return err
	}
	topSplit := height * 0.475
	lines := [][4]float64{
		{margin, topSplit, width - margin, topSplit},
		{width * 0.335, margin, width * 0.335, topSplit},
		{width * 0.705, margin, width * 0.705, topSplit},
		{width * 0.435, topSplit, width * 0.435, height - margin},
		{width * 0.647, topSplit, width * 0.647, height - margin},
	}
	for _, line := range lines {
		if err := addLine(api, doc, loPoint2D{line[0], line[1]}, loPoint2D{line[2], line[3]}); err != nil {
			return err
		}
	}
	return nil
}

func addRectangle(api *loAPI, doc loRef, bounds loRect) error {
	var rectangle loRef
	if err := api.call("LORectangleCreate", uintptr(unsafe.Pointer(&rectangle.Ptr)), uintptr(unsafe.Pointer(&bounds))); err != nil {
		return err
	}
	entity := api.returnRef("LORectangleToEntity", rectangle.Ptr)
	if entity.Ptr == 0 {
		return fmt.Errorf("LORectangleToEntity returned invalid")
	}
	if err := api.call("LODocumentAddEntityUsingIndexes", doc.Ptr, entity.Ptr, 0, 0); err != nil {
		return err
	}
	api.proc("LORectangleRelease").Call(uintptr(unsafe.Pointer(&rectangle.Ptr)))
	return nil
}

func addLine(api *loAPI, doc loRef, start, end loPoint2D) error {
	var path loRef
	if err := api.call(
		"LOPathCreate",
		uintptr(unsafe.Pointer(&path.Ptr)),
		uintptr(unsafe.Pointer(&start)),
		uintptr(unsafe.Pointer(&end)),
	); err != nil {
		return err
	}
	entity := api.returnRef("LOPathToEntity", path.Ptr)
	if entity.Ptr == 0 {
		return fmt.Errorf("LOPathToEntity returned invalid")
	}
	if err := api.call("LODocumentAddEntityUsingIndexes", doc.Ptr, entity.Ptr, 0, 0); err != nil {
		return err
	}
	api.proc("LOPathRelease").Call(uintptr(unsafe.Pointer(&path.Ptr)))
	return nil
}

func addViewLabel(api *loAPI, doc loRef, view model.LayoutViewSpec) error {
	x := (view.Rect.Left + view.Rect.Right) / 2
	y := view.Rect.Bottom + 0.22
	if err := addText(api, doc, view.Title, loPoint2D{x, y}, 9, true); err != nil {
		return err
	}
	scale := "KHÔNG THEO TỶ LỆ"
	if !view.Perspective {
		scale = fmt.Sprintf("TỶ LỆ: 1:%.0f", 1.0/view.Scale)
	} else {
		scale = "TỶ LỆ: KHÔNG THEO TỶ LỆ"
	}
	return addText(api, doc, scale, loPoint2D{x, y + 0.22}, 6, false)
}

func addText(api *loAPI, doc loRef, value string, point loPoint2D, size float64, bold bool) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var text loRef
	if err := api.call(
		"LOFormattedTextCreateAtPoint",
		uintptr(unsafe.Pointer(&text.Ptr)),
		uintptr(unsafe.Pointer(&point)),
		formattedTextAnchorCenterCenter,
		cString(value),
	); err != nil {
		return err
	}
	entity := api.returnRef("LOFormattedTextToEntity", text.Ptr)
	if entity.Ptr == 0 {
		return fmt.Errorf("LOFormattedTextToEntity returned invalid")
	}
	var style loRef
	if err := api.call("LOStyleCreate", uintptr(unsafe.Pointer(&style.Ptr))); err != nil {
		return err
	}
	defer api.proc("LOStyleRelease").Call(uintptr(unsafe.Pointer(&style.Ptr)))
	_ = api.call("LOStyleSetFontSize", style.Ptr, floatArg(size))
	if bold {
		_ = api.call("LOStyleSetTextBold", style.Ptr, 1)
	}
	_ = api.call("LOStyleSetTextAlignment", style.Ptr, textAlignmentCenter)
	_ = api.call("LOEntitySetStyle", entity.Ptr, style.Ptr)
	if err := api.call("LODocumentAddEntityUsingIndexes", doc.Ptr, entity.Ptr, 0, 0); err != nil {
		return err
	}
	api.proc("LOFormattedTextRelease").Call(uintptr(unsafe.Pointer(&text.Ptr)))
	return nil
}

func createDimensionStyle(api *loAPI) (loRef, error) {
	var style loRef
	if err := api.call("LOStyleCreate", uintptr(unsafe.Pointer(&style.Ptr))); err != nil {
		return style, err
	}
	if err := api.call("LOStyleSetDimensionRotationAlignment", style.Ptr, dimensionRotationHorizontal); err != nil {
		return style, err
	}
	if err := api.call("LOStyleSetDimensionVerticalAlignment", style.Ptr, dimensionVerticalBelow); err != nil {
		return style, err
	}
	if err := api.call("LOStyleSetDimensionUnits", style.Ptr, dimensionUnitsDecimalMillimeters, floatArg(1.0)); err != nil {
		return style, err
	}
	if err := api.call("LOStyleSetSuppressDimensionUnits", style.Ptr, 1); err != nil {
		return style, err
	}
	_ = api.call("LOStyleSetFontSize", style.Ptr, floatArg(7.0))
	_ = api.call("LOStyleSetStrokeWidth", style.Ptr, floatArg(0.35))
	return style, nil
}

func addAssociativeDimension(
	api *loAPI,
	doc loRef,
	viewport loRef,
	style loRef,
	spec model.LayoutDimensionSpec,
) error {
	start3 := loPoint3D{spec.Start.Point.X, spec.Start.Point.Y, spec.Start.Point.Z}
	end3 := loPoint3D{spec.End.Point.X, spec.End.Point.Y, spec.End.Point.Z}
	offset3 := loPoint3D{
		start3.X + spec.Offset.X,
		start3.Y + spec.Offset.Y,
		start3.Z + spec.Offset.Z,
	}
	var start2, end2, offset2 loPoint2D
	if err := api.call("LOSketchUpModelConvertModelPointToPaperPoint", viewport.Ptr, uintptr(unsafe.Pointer(&start3)), uintptr(unsafe.Pointer(&start2))); err != nil {
		return err
	}
	if err := api.call("LOSketchUpModelConvertModelPointToPaperPoint", viewport.Ptr, uintptr(unsafe.Pointer(&end3)), uintptr(unsafe.Pointer(&end2))); err != nil {
		return err
	}
	if err := api.call("LOSketchUpModelConvertModelPointToPaperPoint", viewport.Ptr, uintptr(unsafe.Pointer(&offset3)), uintptr(unsafe.Pointer(&offset2))); err != nil {
		return err
	}
	height := math.Hypot(offset2.X-start2.X, offset2.Y-start2.Y)
	perpX := end2.Y - start2.Y
	perpY := -(end2.X - start2.X)
	offsetX := offset2.X - start2.X
	offsetY := offset2.Y - start2.Y
	if perpX*offsetX+perpY*offsetY < 0 {
		height *= -1
	}
	if math.Abs(height) < 0.01 {
		return fmt.Errorf("paper-space dimension offset is too small")
	}

	var dim loRef
	if err := api.call(
		"LOLinearDimensionCreate",
		uintptr(unsafe.Pointer(&dim.Ptr)),
		uintptr(unsafe.Pointer(&start2)),
		uintptr(unsafe.Pointer(&end2)),
		floatArg(height),
	); err != nil {
		return err
	}
	defer api.proc("LOLinearDimensionRelease").Call(uintptr(unsafe.Pointer(&dim.Ptr)))
	entity := api.returnRef("LOLinearDimensionToEntity", dim.Ptr)
	if entity.Ptr == 0 {
		return fmt.Errorf("LOLinearDimensionToEntity returned invalid")
	}
	if err := api.call("LOEntitySetStyle", entity.Ptr, style.Ptr); err != nil {
		return err
	}
	if err := api.call("LODocumentAddEntityUsingIndexes", doc.Ptr, entity.Ptr, 0, 0); err != nil {
		return err
	}

	var startConn, endConn loRef
	if err := api.call(
		"LOConnectionPointCreateFromPID",
		uintptr(unsafe.Pointer(&startConn.Ptr)),
		viewport.Ptr,
		uintptr(unsafe.Pointer(&start3)),
		cString(spec.Start.PersistentIDPath),
	); err != nil {
		return fmt.Errorf("connect start PID %q: %w", spec.Start.PersistentIDPath, err)
	}
	defer api.proc("LOConnectionPointRelease").Call(uintptr(unsafe.Pointer(&startConn.Ptr)))
	if err := api.call(
		"LOConnectionPointCreateFromPID",
		uintptr(unsafe.Pointer(&endConn.Ptr)),
		viewport.Ptr,
		uintptr(unsafe.Pointer(&end3)),
		cString(spec.End.PersistentIDPath),
	); err != nil {
		return fmt.Errorf("connect end PID %q: %w", spec.End.PersistentIDPath, err)
	}
	defer api.proc("LOConnectionPointRelease").Call(uintptr(unsafe.Pointer(&endConn.Ptr)))
	return api.call("LOLinearDimensionConnectTo", dim.Ptr, startConn.Ptr, endConn.Ptr)
}

func addNotes(api *loAPI, doc loRef, notes []string, width, height float64) error {
	if len(notes) == 0 {
		return nil
	}
	x := width * 0.81
	y := height - 0.72
	for i, note := range notes {
		if err := addText(api, doc, note, loPoint2D{x, y + float64(i)*0.14}, 5.5, i == 0); err != nil {
			return err
		}
	}
	return nil
}

func findExportedPNG(dir, base string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, base+"*.png"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("LayOut image export did not produce PNG for base %q", base)
	}
	sort.Strings(matches)
	return matches[0], nil
}
