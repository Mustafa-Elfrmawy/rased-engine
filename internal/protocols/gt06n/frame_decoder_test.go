package gt06n

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Mustafa-Elfrmawy/rased-engine/pkg/crc"
)

// buildFrame constructs a valid non-extended GT06N frame with the given type,
// payload bytes, and serial number by computing the correct length + CRC.
func buildFrame(msgType byte, payload, serial []byte) []byte {
	frame := make([]byte, 0, 64)
	frame = append(frame, 0x78, 0x78)
	// length = type(1) + payload + serial(2) + CRC(2)
	frame = append(frame, byte(1+len(payload)+2+2))
	frame = append(frame, msgType)
	frame = append(frame, payload...)
	crcBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(crcBytes, crc.CRC16X25(frame[2:]))
	frame = append(frame, serial...)
	frame = append(frame, crcBytes...)
	frame = append(frame, 0x0D, 0x0A)
	return frame
}

func TestGT06NSplitFuncSingleFrame(t *testing.T) {
	frame := buildFrame(MsgHeartbeat, []byte{0x07}, []byte{0x00, 0x01})

	advance, token, err := GT06NSplitFunc(frame, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if advance != len(frame) {
		t.Errorf("advance = %d, want %d", advance, len(frame))
	}
	if !bytes.Equal(token, frame) {
		t.Errorf("token != frame")
	}
}

func TestGT06NSplitFuncFragmented(t *testing.T) {
	// Simulate TCP fragmentation: a single frame split into 3 arrival chunks.
	frame := buildFrame(MsgHeartbeat, []byte{0x07}, []byte{0x00, 0x01})
	chunks := [][]byte{
		frame[0:3],   // start + length
		frame[3:8],   // partial
		frame[8:],    // remainder
	}

	var collected [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(concat(chunks)))
	scanner.Split(GT06NSplitFunc)
	for scanner.Scan() {
		token := make([]byte, len(scanner.Bytes()))
		copy(token, scanner.Bytes())
		collected = append(collected, token)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if len(collected) != 1 {
		t.Fatalf("collected %d frames, want 1", len(collected))
	}
	if !bytes.Equal(collected[0], frame) {
		t.Errorf("reassembled frame mismatch")
	}
}

func TestGT06NSplitFuncMergedFrames(t *testing.T) {
	// Simulate TCP coalescing: two frames arriving in a single read.
	f1 := buildFrame(MsgHeartbeat, []byte{0x07}, []byte{0x00, 0x01})
	f2 := buildFrame(MsgGPSLBS1, make([]byte, 26), []byte{0x12, 0x34})

	merged := append(append([]byte{}, f1...), f2...)

	var collected [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(merged))
	scanner.Split(GT06NSplitFunc)
	for scanner.Scan() {
		token := make([]byte, len(scanner.Bytes()))
		copy(token, scanner.Bytes())
		collected = append(collected, token)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if len(collected) != 2 {
		t.Fatalf("collected %d frames, want 2", len(collected))
	}
	if !bytes.Equal(collected[0], f1) {
		t.Errorf("first frame mismatch: %X vs %X", collected[0], f1)
	}
	if !bytes.Equal(collected[1], f2) {
		t.Errorf("second frame mismatch: %X vs %X", collected[1], f2)
	}
}

func TestGT06NSplitFuncGarbageRecovery(t *testing.T) {
	// Simulate a device sending junk before a valid frame (stream sync).
	frame := buildFrame(MsgHeartbeat, []byte{0x07}, []byte{0x00, 0x01})
	garbage := []byte{0x00, 0xFF, 0x12, 0x34}
	stream := append(append([]byte{}, garbage...), frame...)

	var collected [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Split(GT06NSplitFunc)
	for scanner.Scan() {
		token := make([]byte, len(scanner.Bytes()))
		copy(token, scanner.Bytes())
		collected = append(collected, token)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	// Expect at least the valid frame to be extracted
	found := false
	for _, c := range collected {
		if bytes.Equal(c, frame) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("did not extract the valid frame; collected %d tokens", len(collected))
	}
}

func TestGT06NSplitFuncIncomplete(t *testing.T) {
	// A partial frame at end of stream without enough data.
	frame := buildFrame(MsgHeartbeat, []byte{0x07}, []byte{0x00, 0x01})
	partial := frame[:5]

	advance, token, err := GT06NSplitFunc(partial, false)
	if err != nil {
		t.Fatalf("mid-stream partial should not error: %v", err)
	}
	if advance != 0 || token != nil {
		t.Errorf("mid-stream partial should wait for more data (advance=%d token=%v)", advance, token)
	}

	// At EOF, incomplete frames must be reported as an error (dropped)
	_, _, err = GT06NSplitFunc(partial, true)
	if err == nil {
		t.Errorf("incomplete frame at EOF should error")
	}
}

func concat(chunks [][]byte) []byte {
	var out []byte
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}