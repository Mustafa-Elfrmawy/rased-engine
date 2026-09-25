package gt06n

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/Mustafa-Elfrmawy/rased-engine/pkg/crc"
)

// GT06N frame delimiters
const (
	frameStartShort = 0x7878 // Standard frame header (1-byte length)
	frameStartLong  = 0x7979 // Extended frame header (2-byte length)
	frameStop       = 0x0D0A // Stop bits \r\n
)

// MinFrameSize is the smallest valid GT06N frame:
// [2 start] [1 len] [1 type] [2 serial] [2 CRC] [2 stop] = 10 bytes
const MinFrameSize = 10

// MaxFrameSize sanity limit to prevent memory issues from malformed packets.
const MaxFrameSize = 2048

// GT06NFrame represents a single parsed GT06N binary frame.
type GT06NFrame struct {
	Raw      []byte // Complete frame bytes including start/stop
	Extended bool   // true if 0x7979 header, false if 0x7878
	Data     []byte // Payload between start+length and CRC+stop (for decoding)
}

// DecodeFrame validates a raw byte slice as a GT06N frame and extracts
// the data portion for protocol decoding.
//
// Translation of Gt06FrameDecoder.java decode() method:
//   - Checks start bits (0x78 0x78 or 0x79 0x79)
//   - Reads length field (1 or 2 bytes depending on header)
//   - Validates stop bits (0x0D 0x0A) are at the correct position
//   - Computes CRC16/X25 and verifies frame integrity
func DecodeFrame(raw []byte) (*GT06NFrame, error) {
	if len(raw) < MinFrameSize {
		return nil, fmt.Errorf("frame too short: %d bytes", len(raw))
	}

	if len(raw) > MaxFrameSize {
		return nil, fmt.Errorf("frame too long: %d bytes", len(raw))
	}

	// Check start bits
	header := binary.BigEndian.Uint16(raw[0:2])
	extended := header == frameStartLong

	var length int
	if extended {
		if len(raw) < 4 {
			return nil, fmt.Errorf("extended frame too short for length field")
		}
		length = int(binary.BigEndian.Uint16(raw[2:4]))
	} else {
		length = int(raw[2])
	}

	// Total frame size.
	// GT06N length field counts everything from the byte AFTER itself through
	// the CRC: type(1) + payload + serial(2) + CRC(2). So the physical frame is:
	//   2(start) + 1(length field) + length + 2(stop)           = length + 5
	//   2(start) + 2(length fields) + length + 2(stop) (extended) = length + 6
	frameSize := length + 5
	if extended {
		frameSize = length + 6
	}
	if len(raw) < frameSize {
		return nil, fmt.Errorf("incomplete frame: need %d bytes, got %d", frameSize, len(raw))
	}

	// Validate stop bits at position [frameSize-2:]
	if raw[frameSize-2] != 0x0D || raw[frameSize-1] != 0x0A {
		return nil, fmt.Errorf("invalid stop bits: got 0x%02X 0x%02X, expected 0x0D 0x0A",
			raw[frameSize-2], raw[frameSize-1])
	}

	// CRC covers bytes from position 2 (length field) to frameSize-4 (serial end)
	// This matches Java: Checksum.crc16(CRC16_X25, buf.nioBuffer(2, writerIndex - 2))
	// where writerIndex = frameSize (before stop bits), so size = frameSize - 2 - 2 = frameSize - 4
	crcData := raw[2 : frameSize-4]
	expectedCRC := binary.BigEndian.Uint16(raw[frameSize-4 : frameSize-2])
	actualCRC := crc.CRC16X25(crcData)

	if actualCRC != expectedCRC {
		return nil, fmt.Errorf("CRC mismatch: expected 0x%04X, got 0x%04X", expectedCRC, actualCRC)
	}

	// Extract data portion: everything between length field end and CRC start
	// For standard: data = raw[3 : frameSize-4]
	// For extended: data = raw[4 : frameSize-4]
	var dataStart int
	if extended {
		dataStart = 4
	} else {
		dataStart = 3
	}

	return &GT06NFrame{
		Raw:      raw[:frameSize],
		Extended: extended,
		Data:     raw[dataStart : frameSize-4],
	}, nil
}

// GT06NSplitFunc returns a bufio.SplitFunc that correctly fragments a TCP
// byte stream into individual GT06N frames.
//
// This solves the TCP fragmentation problem: since TCP is a stream protocol,
// multiple GT06N packets can arrive merged together, or a single packet can
// be split across multiple read calls. This function uses the length field
// embedded in the GT06N frame header to determine exact boundaries.
//
// Translation of Gt06FrameDecoder.java decode() logic:
//   - Scan for start bits (0x78 0x78 or 0x79 0x79)
//   - Read the length field to know exact frame size
//   - Return complete frames to the scanner
//
// We also support splitting on 0x0D 0x0A stop bits as a fallback,
// since all valid GT06N frames end with these bytes.
func GT06NSplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// Need at least 3 bytes to check header type and read length
	if len(data) < 3 {
		if atEOF && len(data) > 0 {
			return len(data), data, fmt.Errorf("incomplete GT06N frame at EOF")
		}
		return 0, nil, nil // request more data
	}

	// Find the start bits (0x78 0x78 or 0x79 0x79)
	header := binary.BigEndian.Uint16(data[0:2])

	var frameLength int
	if header == frameStartShort {
		// 2(start) + 1(length field) + length + 2(stop) = length + 5
		frameLength = int(data[2]) + 5
	} else if header == frameStartLong {
		if len(data) < 4 {
			return 0, nil, nil // need more data to read 2-byte length
		}
		// 2(start) + 2(length fields) + length + 2(stop) = length + 6
		frameLength = int(binary.BigEndian.Uint16(data[2:4])) + 6
	} else {
		// No valid start bits at position 0 — scan forward for the next 0x78
		for i := 1; i < len(data); i++ {
			if data[i] == 0x78 {
				return i, data[:i], nil // advance past garbage, return it as a token
			}
		}
		// No start bits found in entire buffer — discard
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}

	// Validate minimum and maximum frame sizes
	if frameLength < MinFrameSize || frameLength > MaxFrameSize {
		// Malformed length — skip the bad header byte and try next position
		for i := 1; i < len(data); i++ {
			if data[i] == 0x78 {
				return i, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}

	// Not enough data yet — request more from the TCP connection
	if len(data) < frameLength {
		if atEOF {
			return len(data), data, fmt.Errorf("incomplete GT06N frame at EOF")
		}
		return 0, nil, nil // wait for more data
	}

	// Validate stop bits before returning the frame
	if data[frameLength-2] != 0x0D || data[frameLength-1] != 0x0A {
		// Invalid stop bits — skip header and scan for next start
		for i := 1; i < len(data); i++ {
			if data[i] == 0x78 {
				return i, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}

	// Return the complete frame
	return frameLength, data[:frameLength], nil
}

// ValidateCRC checks if a complete GT06N frame has a valid CRC.
// Returns true if the CRC matches, false otherwise.
func ValidateCRC(frame []byte) bool {
	if len(frame) < MinFrameSize {
		return false
	}

	header := binary.BigEndian.Uint16(frame[0:2])
	var length int
	var frameSize int
	if header == frameStartLong {
		length = int(binary.BigEndian.Uint16(frame[2:4]))
		frameSize = length + 6
	} else {
		length = int(frame[2])
		frameSize = length + 5
	}
	if len(frame) < frameSize {
		return false
	}

	// Stop bits check
	if frame[frameSize-2] != 0x0D || frame[frameSize-1] != 0x0A {
		return false
	}

	// CRC check
	crcData := frame[2 : frameSize-4]
	expectedCRC := binary.BigEndian.Uint16(frame[frameSize-4 : frameSize-2])
	return crc.CRC16X25(crcData) == expectedCRC
}

// IsGT06NFrame checks if a byte slice starts with valid GT06N start bits.
func IsGT06NFrame(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	header := binary.BigEndian.Uint16(data[0:2])
	return header == frameStartShort || header == frameStartLong
}

// FrameLength extracts the total frame length (including start/stop) from
// the raw frame header. Returns 0 if the header is invalid.
func FrameLength(data []byte) int {
	if len(data) < 2 {
		return 0
	}
	header := binary.BigEndian.Uint16(data[0:2])
	if header == frameStartShort {
		if len(data) < 3 {
			return 0
		}
		return int(data[2]) + 5
	} else if header == frameStartLong {
		if len(data) < 4 {
			return 0
		}
		return int(binary.BigEndian.Uint16(data[2:4])) + 6
	}
	return 0
}

// ExtractMessageType returns the message type byte from a GT06N frame.
func ExtractMessageType(data []byte) byte {
	if len(data) < 4 {
		return 0
	}
	header := binary.BigEndian.Uint16(data[0:2])
	if header == frameStartShort {
		return data[3]
	} else if header == frameStartLong {
		return data[4]
	}
	return 0
}

// TrimFrame trims excess data after the frame's stop bits.
// Returns only the valid frame bytes.
func TrimFrame(data []byte) []byte {
	frameLen := FrameLength(data)
	if frameLen > 0 && frameLen <= len(data) {
		// Validate stop bits
		if bytes.HasSuffix(data[:frameLen], []byte{0x0D, 0x0A}) {
			return data[:frameLen]
		}
	}
	return data
}
