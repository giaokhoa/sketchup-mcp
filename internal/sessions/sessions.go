package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/giaokhoa/sketchup-mcp/internal/bridge"
)

type ListInput struct{}

type Model struct {
	GUID     string `json:"guid" jsonschema:"SketchUp model GUID"`
	Title    string `json:"title" jsonschema:"SketchUp model title"`
	Revision uint64 `json:"revision" jsonschema:"in-memory model revision"`
}

type Session struct {
	SessionID             string `json:"session_id" jsonschema:"opaque SketchUp MCP session identifier"`
	PID                   int    `json:"pid" jsonschema:"SketchUp process identifier"`
	SketchUpVersion       string `json:"sketchup_version" jsonschema:"SketchUp desktop version"`
	Model                 Model  `json:"model" jsonschema:"current model snapshot"`
	BridgeEndpoint        string `json:"bridge_endpoint" jsonschema:"loopback bridge endpoint"`
	BridgeProtocolVersion int    `json:"bridge_protocol_version" jsonschema:"private bridge protocol version"`
	LastSeenAt            string `json:"last_seen_at" jsonschema:"RFC3339 timestamp of latest authenticated response"`
}

type ListOutput struct {
	Sessions []Session `json:"sessions" jsonschema:"healthy SketchUp desktop sessions"`
}

type registryEntry struct {
	descriptor bridge.Descriptor
	client     *bridge.Client
	session    Session
}

type Registry struct {
	directory string
	logger    *slog.Logger
	ioTimeout time.Duration
	now       func() time.Time

	refreshMu sync.Mutex
	mu        sync.RWMutex
	entries   map[string]*registryEntry
}

func DefaultDiscoveryDirectory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve per-user cache directory: %w", err)
	}
	return filepath.Join(root, "SketchUpMCP", "sessions"), nil
}

func NewDefaultRegistry(logger *slog.Logger) (*Registry, error) {
	directory, err := DefaultDiscoveryDirectory()
	if err != nil {
		return nil, err
	}
	return NewRegistry(directory, logger, 5*time.Second), nil
}

func NewRegistry(directory string, logger *slog.Logger, ioTimeout time.Duration) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	if ioTimeout <= 0 {
		ioTimeout = 5 * time.Second
	}
	return &Registry{
		directory: directory,
		logger:    logger,
		ioTimeout: ioTimeout,
		now:       time.Now,
		entries:   make(map[string]*registryEntry),
	}
}

func (r *Registry) List(ctx context.Context) (ListOutput, error) {
	if err := r.Refresh(ctx); err != nil {
		return ListOutput{}, err
	}
	r.mu.RLock()
	sessions := make([]Session, 0, len(r.entries))
	for _, entry := range r.entries {
		sessions = append(sessions, entry.session)
	}
	r.mu.RUnlock()

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].SessionID < sessions[j].SessionID
	})
	return ListOutput{Sessions: sessions}, nil
}

func (r *Registry) Refresh(ctx context.Context) error {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()

	descriptors, err := r.loadDescriptors()
	if err != nil {
		return err
	}

	r.mu.RLock()
	oldEntries := make(map[string]*registryEntry, len(r.entries))
	for id, entry := range r.entries {
		oldEntries[id] = entry
	}
	r.mu.RUnlock()

	next := make(map[string]*registryEntry, len(descriptors))
	for _, descriptor := range descriptors {
		if err := ctx.Err(); err != nil {
			return err
		}
		old := oldEntries[descriptor.SessionID]
		client, info, err := r.probe(ctx, descriptor, old)
		if err != nil {
			r.logger.Debug("ignoring unhealthy SketchUp bridge",
				"session_id", descriptor.SessionID, "error", err)
			continue
		}
		next[descriptor.SessionID] = &registryEntry{
			descriptor: descriptor,
			client:     client,
			session: Session{
				SessionID:       descriptor.SessionID,
				PID:             info.PID,
				SketchUpVersion: info.SketchUpVersion,
				Model: Model{
					GUID:     info.Model.GUID,
					Title:    info.Model.Title,
					Revision: info.Model.Revision,
				},
				BridgeEndpoint:        descriptor.Endpoint(),
				BridgeProtocolVersion: descriptor.ProtocolVersion,
				LastSeenAt:            r.now().UTC().Format(time.RFC3339Nano),
			},
		}
	}

	r.mu.Lock()
	r.entries = next
	r.mu.Unlock()

	for id, old := range oldEntries {
		if current := next[id]; current == nil || current.client != old.client {
			_ = old.client.Close()
		}
	}
	return nil
}

func (r *Registry) Close() error {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()

	r.mu.Lock()
	entries := r.entries
	r.entries = make(map[string]*registryEntry)
	r.mu.Unlock()

	for _, entry := range entries {
		_ = entry.client.Close()
	}
	return nil
}

func (r *Registry) probe(
	ctx context.Context,
	descriptor bridge.Descriptor,
	old *registryEntry,
) (*bridge.Client, bridge.SessionInfo, error) {
	if old != nil && old.descriptor.SameConnection(descriptor) {
		info, err := old.client.SessionInfo(ctx)
		if err == nil {
			return old.client, info, nil
		}
		_ = old.client.Close()
	}

	client, err := bridge.Dial(ctx, descriptor, r.ioTimeout)
	if err != nil {
		return nil, bridge.SessionInfo{}, err
	}
	info, err := client.SessionInfo(ctx)
	if err != nil {
		_ = client.Close()
		return nil, bridge.SessionInfo{}, err
	}
	return client, info, nil
}

func (r *Registry) loadDescriptors() ([]bridge.Descriptor, error) {
	entries, err := os.ReadDir(r.directory)
	if os.IsNotExist(err) {
		return []bridge.Descriptor{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read SketchUp bridge discovery directory: %w", err)
	}

	descriptors := make([]bridge.Descriptor, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(r.directory, entry.Name())
		descriptor, err := readDescriptor(path)
		if err != nil {
			r.logger.Debug("ignoring invalid SketchUp bridge descriptor",
				"file", entry.Name(), "error", err)
			continue
		}
		if entry.Name() != descriptor.SessionID+".json" {
			r.logger.Debug("ignoring mismatched SketchUp bridge descriptor filename",
				"file", entry.Name(), "session_id", descriptor.SessionID)
			continue
		}
		descriptors = append(descriptors, descriptor)
	}
	return descriptors, nil
}

func readDescriptor(path string) (bridge.Descriptor, error) {
	file, err := os.Open(path)
	if err != nil {
		return bridge.Descriptor{}, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, bridge.MaxDescriptorBytes+1))
	if err != nil {
		return bridge.Descriptor{}, err
	}
	if len(data) > bridge.MaxDescriptorBytes {
		return bridge.Descriptor{}, errors.New("descriptor exceeds size limit")
	}

	var descriptor bridge.Descriptor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return bridge.Descriptor{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return bridge.Descriptor{}, errors.New("descriptor contains trailing JSON")
		}
		return bridge.Descriptor{}, err
	}
	if err := bridge.ValidateDescriptor(descriptor); err != nil {
		return bridge.Descriptor{}, err
	}
	return descriptor, nil
}
