package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/giaokhoa/sketchup-mcp/internal/layoutworker"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "generate" {
		fmt.Fprintln(os.Stderr, "usage: layout-worker generate --spec <drawing-spec.json>")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	specPath := fs.String("spec", "", "drawing specification JSON")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "--spec is required")
		os.Exit(2)
	}
	spec, err := layoutworker.LoadSpec(*specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load drawing spec:", err)
		os.Exit(1)
	}
	out, err := layoutworker.Generate(context.Background(), spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode output:", err)
		os.Exit(1)
	}
}
