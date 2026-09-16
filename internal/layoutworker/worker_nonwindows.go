//go:build !windows

package layoutworker

import (
	"context"
	"errors"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
)

func Generate(context.Context, model.LayoutDrawingSpec) (model.LayoutA3SheetOutput, error) {
	return model.LayoutA3SheetOutput{}, errors.New("LayOut worker requires Windows with SketchUp Desktop runtime")
}
