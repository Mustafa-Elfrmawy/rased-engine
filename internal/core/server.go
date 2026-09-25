package core

import (
	"bufio"
	"context"
	"errors"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/protocols/gt06n"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/protocols/gt06n_tcp"
	"io"
	"log"
	"net"
	"os"
	"time"
)

const (
	// ConnectionTimeout is the idle timeout for a persistent GT06N connection.
	// If no heartbeats or data are received within this window, the connection
	// is closed to free socket FDs and goroutines (dead vehicle in a tunnel, etc).
	ConnectionTimeout = 2 * time.Minute

	// WriteTimeout bounds a single ACK write to the device.
	WriteTimeout = 5 * time.Second
)

// Server is the GT06N TCP ingestion engine.
//
// It blindly assumes any data arriving on port 5023 follows the GT06 protocol
// (Connection-Based Routing, Layer 1). Each accepted connection runs in its
// own goroutine and is managed with SetReadDeadline for idle detection.
//
// Single-protocol focus: no generic protocol registry or multiplexer. This
// server is hardcoded for GT06N only, matching the architecture docs.
type Server struct {
	handler *gt06n_tcp.Handler
}

// NewServer creates the GT06N TCP server.
func NewServer(handler *gt06n_tcp.Handler) *Server {
	return &Server{
		handler: handler,
	}
}

// Start begins listening on the configured GT06N port and blocks forever.
func (s *Server) Start(addr string) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on TCP %s: %v", addr, err)
	}
	defer listener.Close()

	log.Printf("GT06N listener started on %s", addr)

	for {
		// log.Fatalf("GT06N listener started on %v", s.handler.Port)
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("TCP Accept error: %v", err)
			continue
		}
		// One lightweight goroutine per accepted connection
		go s.handleConnection(conn)
	}
}

// handleConnection manages the full lifecycle of a single GT06N connection.
//
// Responsibilities:
//  1. Enable TCP keep-alive
//  2. Set idle read deadline (resets on every successful frame read)
//  3. Run the custom bufio.Scanner SplitFunc to extract complete GT06N frames
//  4. Decode and route each frame, writing back selective ACKs
//  5. Close cleanly on timeout or disconnect
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr()

	
	// Enable OS-level TCP KeepAlive as a secondary safety net
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(2 * time.Minute)
	}

	// Per-connection session state (IMEI from login)
	session := gt06n_tcp.NewSession()

	// Custom buffer scanner: splits the continuous TCP stream into complete
	// GT06N frames by searching for 0x78 0x78 start bits and 0x0D 0x0A stop bits.
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), gt06n.MaxFrameSize)
	scanner.Split(gt06n.GT06NSplitFunc)

	for {
		// Application-level idle detection: reset the read deadline before
		// each read attempt. Every successful frame read below resets it.
		if err := conn.SetReadDeadline(time.Now().Add(ConnectionTimeout)); err != nil {
			log.Printf("Failed to set read deadline for %v: %v", remoteAddr, err)
			return
		}

		if !scanner.Scan() {
			// Disconnection or timeout
			if err := scanner.Err(); err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					log.Printf("TCP connection from %v closed due to idle timeout", remoteAddr)
				} else if err != io.EOF {
					log.Printf("TCP read error from %v: %v", remoteAddr, err)
				}
			} else {
				log.Printf("TCP client %v disconnected gracefully", remoteAddr)
			}
			return
		}

		frame := scanner.Bytes()
		if len(frame) == 0 {
			continue
		}

		// Copy the frame since scanner.Bytes() reuses its internal buffer
		frameCopy := make([]byte, len(frame))
		copy(frameCopy, frame)

		// Decode & route the frame
		result, err := s.handler.HandleFrame(context.Background(), frameCopy, session)
		if err != nil {
			log.Printf("[GT06N] Frame error from %v: %v", remoteAddr, err)
			continue
		}

		// Write the selective ACK back (nil for fire-and-forget location packets)
		if result != nil && len(result.Ack) > 0 {
			_ = conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if _, err := conn.Write(result.Ack); err != nil {
				log.Printf("TCP write error to %v: %v", remoteAddr, err)
			}
		}

		// Send decoded positions through the middleware pipeline
		s.handler.ProcessPosition(result)
	}
}
