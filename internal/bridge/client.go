package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type ModelInfo struct {
	GUID     string `json:"guid"`
	Title    string `json:"title"`
	Revision uint64 `json:"revision"`
}

type SessionInfo struct {
	SessionID       string    `json:"session_id"`
	PID             int       `json:"pid"`
	SketchUpVersion string    `json:"sketchup_version"`
	Model           ModelInfo `json:"model"`
}

type Client struct {
	conn       net.Conn
	descriptor Descriptor
	ioTimeout  time.Duration
	mu         sync.Mutex
}

type helloRequest struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	Client          string `json:"client"`
	ClientNonce     string `json:"client_nonce"`
}

type helloResponse struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	SessionID       string `json:"session_id"`
	ServerNonce     string `json:"server_nonce"`
	MaxMessageBytes int    `json:"max_message_bytes"`
}

type authenticateRequest struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	Token           string `json:"token"`
}

type protocolError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

type authenticateResponse struct {
	Type            string         `json:"type"`
	ProtocolVersion int            `json:"protocol_version"`
	OK              bool           `json:"ok"`
	Error           *protocolError `json:"error,omitempty"`
}

type requestMessage struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	ID              string `json:"id"`
	SessionID       string `json:"session_id"`
	Operation       string `json:"operation"`
	TimeoutMS       int    `json:"timeout_ms"`
	Payload         any    `json:"payload"`
}

type responseMessage struct {
	Type            string          `json:"type"`
	ProtocolVersion int             `json:"protocol_version"`
	ID              string          `json:"id"`
	OK              bool            `json:"ok"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Error           *protocolError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    string
	Message string
	Details json.RawMessage
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("bridge %s: %s", e.Code, e.Message)
}

func Dial(ctx context.Context, descriptor Descriptor, ioTimeout time.Duration) (*Client, error) {
	if err := ValidateDescriptor(descriptor); err != nil {
		return nil, err
	}
	if ioTimeout <= 0 {
		ioTimeout = 5 * time.Second
	}
	dialer := net.Dialer{Timeout: ioTimeout}
	conn, err := dialer.DialContext(ctx, "tcp4", descriptor.Endpoint())
	if err != nil {
		return nil, fmt.Errorf("dial bridge session %s: %w", descriptor.SessionID, err)
	}

	client := &Client{conn: conn, descriptor: descriptor, ioTimeout: ioTimeout}
	if err := client.handshake(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Descriptor() Descriptor {
	return c.descriptor
}

func (c *Client) SessionInfo(ctx context.Context) (SessionInfo, error) {
	var info SessionInfo
	if err := c.request(ctx, "session.info", map[string]any{}, &info); err != nil {
		return SessionInfo{}, err
	}
	if info.SessionID != c.descriptor.SessionID {
		return SessionInfo{}, errors.New("session.info returned a different session_id")
	}
	return info, nil
}

func (c *Client) Call(ctx context.Context, operation string, payload any, output any) error {
	return c.request(ctx, operation, payload, output)
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) handshake(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.setDeadline(ctx); err != nil {
		return err
	}

	nonce, err := randomUUID()
	if err != nil {
		return err
	}
	if err := writeFrame(c.conn, helloRequest{
		Type: "hello", ProtocolVersion: ProtocolVersion,
		Client: "sketchup-mcp-go", ClientNonce: nonce,
	}); err != nil {
		return err
	}
	var hello helloResponse
	if err := readFrame(c.conn, &hello); err != nil {
		return err
	}
	if hello.Type != "hello" {
		return fmt.Errorf("unexpected bridge handshake message %q", hello.Type)
	}
	if hello.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got %d want %d", ErrProtocolVersion, hello.ProtocolVersion, ProtocolVersion)
	}
	if hello.SessionID != c.descriptor.SessionID {
		return errors.New("bridge hello session_id does not match descriptor")
	}
	if !validUUID(hello.ServerNonce) {
		return errors.New("bridge hello server_nonce must be a UUID")
	}
	if hello.MaxMessageBytes <= 0 || hello.MaxMessageBytes > MaxMessageBytes {
		return errors.New("bridge hello advertised invalid max_message_bytes")
	}

	if err := writeFrame(c.conn, authenticateRequest{
		Type: "authenticate", ProtocolVersion: ProtocolVersion, Token: c.descriptor.Token,
	}); err != nil {
		return err
	}
	var auth authenticateResponse
	if err := readFrame(c.conn, &auth); err != nil {
		return err
	}
	if auth.Type != "authenticate" {
		return fmt.Errorf("unexpected bridge authentication message %q", auth.Type)
	}
	if auth.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got %d want %d", ErrProtocolVersion, auth.ProtocolVersion, ProtocolVersion)
	}
	if !auth.OK {
		return ErrAuthentication
	}
	return nil
}

func (c *Client) request(ctx context.Context, operation string, payload any, output any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return net.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.setDeadline(ctx); err != nil {
		return err
	}
	id, err := randomUUID()
	if err != nil {
		return err
	}
	timeoutMS := int(c.ioTimeout / time.Millisecond)
	if timeoutMS < 100 {
		timeoutMS = 100
	}
	if timeoutMS > 30_000 {
		timeoutMS = 30_000
	}
	request := requestMessage{
		Type: "request", ProtocolVersion: ProtocolVersion, ID: id,
		SessionID: c.descriptor.SessionID, Operation: operation,
		TimeoutMS: timeoutMS, Payload: payload,
	}
	if err := writeFrame(c.conn, request); err != nil {
		return err
	}

	var response responseMessage
	if err := readFrame(c.conn, &response); err != nil {
		return err
	}
	if response.Type != "response" {
		return fmt.Errorf("unexpected bridge response message %q", response.Type)
	}
	if response.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got %d want %d", ErrProtocolVersion, response.ProtocolVersion, ProtocolVersion)
	}
	if response.ID != id {
		return errors.New("bridge response correlation id mismatch")
	}
	if !response.OK {
		if response.Error == nil {
			return errors.New("bridge returned an error without details")
		}
		return &ResponseError{Code: response.Error.Code, Message: response.Error.Message, Details: response.Error.Details}
	}
	if len(response.Payload) == 0 {
		return errors.New("bridge success response has no payload")
	}
	if err := json.Unmarshal(response.Payload, output); err != nil {
		return fmt.Errorf("decode bridge response payload: %w", err)
	}
	return nil
}
func (c *Client) setDeadline(ctx context.Context) error {
	deadline := time.Now().Add(c.ioTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	return c.conn.SetDeadline(deadline)
}

func randomUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate UUID: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}
