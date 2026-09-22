package gt06n_tcp

import (
	"context"
	"fmt"

	"go-tracker-service/internal/models"
	"go-tracker-service/internal/pipeline"
	"go-tracker-service/internal/protocols/gt06n"
)

// Handler implements the GT06N binary protocol over TCP.
//
// This is a per-connection handler that:
//  1. Receives complete GT06N frames extracted from the TCP stream
//     (the transport layer uses a bufio.SplitFunc that searches for
//     0x78 0x78 start bits and 0x0D 0x0A stop bits)
//  2. Decodes the binary payload (login, GPS/LBS, status, heartbeat, alarm)
//  3. Forwards decoded locations through the pipeline (Filter -> Geofence -> Publish)
//  4. Returns the CRC-validated ACK bytes the transport layer must write back
//
// Per-connection session state tracks the device IMEI (from login) and whether
// the connection has successfully authenticated.
//
// Single-protocol, single-purpose handler — hardcoded for GT06N on port 5023.
type Handler struct {
	port     string
	pipeline *pipeline.Pipeline
}

func NewHandler(port string, pipe *pipeline.Pipeline) *Handler {
	return &Handler{
		port:     port,
		pipeline: pipe,
	}
}

func (h *Handler) ProtocolName() string {
	return "gt06n_tcp"
}

func (h *Handler) ConnectionType() string {
	return "tcp"
}

func (h *Handler) Port() string {
	return h.port
}

// Session tracks the state of a single persistent connection.
type Session struct {
	IMEI     string
	LoggedIn bool
}

// NewSession creates a fresh per-connection session.
func NewSession() *Session {
	return &Session{}
}

// HandleFrame is the core entry point for each decoded GT06N raw frame.
// It returns the ACK bytes the transport layer must write back to the device
// (nil if no ACK is required — fire-and-forget location packets), and the
// decoded position (nil for login/heartbeat frames).
//
// The transport goroutine calls this once per complete frame.
func (h *Handler) HandleFrame(ctx context.Context, rawFrame []byte, session *Session) (*FrameResult, error) {
	result := &FrameResult{}

	// Step 1: CRC validation — reject corrupted frames early
	if !gt06n.ValidateCRC(rawFrame) {
		return nil, fmt.Errorf("frame failed CRC validation")
	}

	// Step 2: Decode message type from the frame header
	msgType := gt06n.ExtractMessageType(rawFrame)

	// Step 3: Route based on message type
	switch msgType {
	case gt06n.MsgLogin:
		// Login — extract IMEI, validate (mocked), ACK back
		pos, ack, err := gt06n.Decode(rawFrame, session.IMEI)
		if err != nil {
			return nil, fmt.Errorf("login decode: %w", err)
		}
		_ = pos
		result.Ack = ack

		// Extract and store IMEI from the frame
		if f, err := gt06n.DecodeFrame(rawFrame); err == nil && len(f.Data) >= 9 {
			session.IMEI = extractIMEI(f.Data)
			session.LoggedIn = true
		}

		return result, nil

	case gt06n.MsgHeartbeat:
		// Heartbeat — decode and ACK to keep the connection alive
		pos, ack, err := gt06n.Decode(rawFrame, session.IMEI)
		if err != nil {
			return nil, fmt.Errorf("heartbeat decode: %w", err)
		}
		result.Ack = ack
		result.Position = pos
		return result, nil

	case gt06n.MsgStatus, gt06n.MsgGPSLBSStat1, gt06n.MsgGPSLBSStat2,
		gt06n.MsgGPSLBSStat3, gt06n.MsgGPSLBSStat4, gt06n.MsgGPSLBSStat5,
		gt06n.MsgAlarmModule:
		// Status / alarm — decode and ACK
		pos, ack, err := gt06n.Decode(rawFrame, session.IMEI)
		if err != nil {
			return nil, fmt.Errorf("status decode: %w", err)
		}
		result.Ack = ack
		result.Position = pos
		return result, nil

	default:
		// GPS / LBS / location packets — fire-and-forget (no ACK)
		pos, ack, err := gt06n.Decode(rawFrame, session.IMEI)
		if err != nil {
			return nil, fmt.Errorf("location decode: %w", err)
		}
		result.Ack = ack
		result.Position = pos
		return result, nil
	}
}

// ProcessPosition pushes a decoded Position through the middleware pipeline
// (FilterHandler -> GeofenceHandler -> PublishHandler).
//
// This must be called by the transport layer after HandleFrame returns a
// result with a non-nil Position.
func (h *Handler) ProcessPosition(result *FrameResult) {
	if result == nil || result.Position == nil || h.pipeline == nil {
		return
	}
	h.pipeline.Process(result.Position)
}

// Result carries the decoded ACK + position for a single frame.
type FrameResult struct {
	Ack      []byte
	Position *models.Position
}

// extractIMEI converts the 8 BCD bytes of a login packet into the 15-digit IMEI.
// Mirrors Java: ByteBufUtil.hexDump(buf.readSlice(8)).substring(1)
func extractIMEI(data []byte) string {
	imei := fmt.Sprintf("%016X", data[1:9])
	return imei[1:]
}
