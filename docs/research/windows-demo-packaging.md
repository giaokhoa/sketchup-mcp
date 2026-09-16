# Windows demo packaging research gate

Verified: **2026-09-16**

Issue: #9

## GitHub Actions

Current official releases checked immediately before implementation:

- `actions/checkout`: v7.0.1 -> use `actions/checkout@v7`
- `actions/setup-go`: v7.0.0 -> use `actions/setup-go@v7`
- `actions/setup-python`: v7.0.0 -> use `actions/setup-python@v7`
- `actions/upload-artifact`: v7.0.1 -> use `actions/upload-artifact@v7`
- `ruby/setup-ruby` vendor guidance uses the maintained `@v1` major channel.

No third-party marketplace build or packaging action is required.

## Go Windows executable

Official Go guidance for reproducible builds recommends removing host path input
with `-trimpath`. The Go build command documents `-buildvcs=false` to omit VCS
stamping. The Go reproducible-build guidance also recommends disabling cgo when
it is not required so the host C toolchain is not an input.

Chosen build:

```text
CGO_ENABLED=0
GOOS=windows
GOARCH=amd64
go build -trimpath -buildvcs=false -o sketchup-mcp-windows-amd64.exe ./cmd/sketchup-mcp
```

The MCP host has no cgo requirement.

## SketchUp RBZ

SketchUp's official extension requirements say an RBZ is a ZIP archive renamed
to `.rbz` and must contain exactly two root items:

1. the root loader `.rb`;
2. a support folder with the same base name.

The existing payload already follows that layout.

For deterministic bytes, packaging uses:

- lexicographically sorted files;
- fixed ZIP timestamps;
- fixed file modes;
- ZIP stored entries rather than DEFLATE, avoiding compressor-version output
  differences.

CI builds the complete bundle twice in separate output directories and compares
the EXE, RBZ, and checksum file byte-for-byte before upload.

## Authoritative references

- https://go.dev/blog/rebuild
- https://go.dev/src/cmd/go/internal/work/build.go
- https://ruby.sketchup.com/file.extension_requirements.html
- official GitHub release pages for the actions listed above.
