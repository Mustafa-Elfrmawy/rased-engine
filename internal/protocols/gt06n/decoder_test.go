package gt06n

import (
	"encoding/binary"
	"testing"

	"github.com/Mustafa-Elfrmawy/rased-engine/pkg/crc"
)

// buildLoginFrame constructs a syntactically valid GT06N login packet:
//
//	[78 78]{start} [0F]{length=15} [01]{type=MSG_LOGIN}
//	[0102030405060708]{imei bytes}
//	[0001]{terminal type} [0054]{serial}
//	[CRC]{2} [0D 0A]{stop}
func buildLoginFrame() []byte {
	frame := make([]byte, 0, 21)
	frame = append(frame, 0x78, 0x78) // start bits
	frame = append(frame, 0x0F)       // length
	frame = append(frame, MsgLogin)   // type

	// IMEI bytes (8) + terminal type (2)
	imei := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	frame = append(frame, imei...)
	frame = append(frame, 0x00, 0x01) // terminal type

	// serial number (2 bytes)
	serial := []byte{0x2B, 0x76} // 11126
	frame = append(frame, serial...)

	// CRC over frame[2 : len] (length..serial)
	crcVal := crc.CRC16X25(frame[2:])
	crcBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(crcBytes, crcVal)
	frame = append(frame, crcBytes...)

	// stop bits
	frame = append(frame, 0x0D, 0x0A)

	return frame
}

func TestDecodeFrameLogin(t *testing.T) {
	frame := buildLoginFrame()

	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFrame error: %v", err)
	}

	if decoded.Extended {
		t.Errorf("expected non-extended frame")
	}
	if len(decoded.Data) != 13 {
		t.Errorf("data length = %d, want 13", len(decoded.Data))
	}
}

func TestValidateCRC(t *testing.T) {
	frame := buildLoginFrame()
	if !ValidateCRC(frame) {
		t.Errorf("ValidateCRC returned false for a valid frame")
	}

	// Corrupt one byte in the middle, CRC should fail
	bad := make([]byte, len(frame))
	copy(bad, frame)
	bad[5] ^= 0xFF
	if ValidateCRC(bad) {
		t.Errorf("ValidateCRC passed for corrupted frame")
	}
}

func TestExtractMessageType(t *testing.T) {
	frame := buildLoginFrame()
	if typ := ExtractMessageType(frame); typ != MsgLogin {
		t.Errorf("ExtractMessageType = 0x%02X, want 0x01", typ)
	}
}

func TestDecodeLoginACK(t *testing.T) {
	frame := buildLoginFrame()

	pos, ack, err := Decode(frame, "")
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if pos != nil {
		t.Errorf("login should return nil position")
	}
	if ack == nil {
		t.Fatalf("login should return an ACK")
	}

	// ACK structure check: [78 78][05][01][serial 2][crc 2][0D 0A]
	if len(ack) != 10 {
		t.Fatalf("ACK length = %d, want 10", len(ack))
	}
	if ack[0] != 0x78 || ack[1] != 0x78 {
		t.Errorf("ACK start bits wrong: %X", ack[0:2])
	}
	if ack[2] != 0x05 {
		t.Errorf("ACK length field = 0x%02X, want 0x05", ack[2])
	}
	if ack[3] != MsgLogin {
		t.Errorf("ACK type = 0x%02X, want 0x01", ack[3])
	}
	// Serial echoed back: 0x2B76
	if ack[4] != 0x2B || ack[5] != 0x76 {
		t.Errorf("ACK serial = %02X%02X, want 2B76", ack[4], ack[5])
	}
	// Stop bits
	if ack[8] != 0x0D || ack[9] != 0x0A {
		t.Errorf("ACK stop bits wrong: %X %X", ack[8], ack[9])
	}
	// CRC must validate: CRC over ack[2:6]
	expectedCRC := crc.CRC16X25(ack[2:6])
	actualCRC := binary.BigEndian.Uint16(ack[6:8])
	if expectedCRC != actualCRC {
		t.Errorf("ACK CRC = 0x%04X, want 0x%04X", actualCRC, expectedCRC)
	}
	// And ValidateCRC on the whole ACK frame must pass
	if !ValidateCRC(ack) {
		t.Errorf("ACK frame failed ValidateCRC")
	}
}

func TestDecodeHeartbeat(t *testing.T) {
	// Build heartbeat frame:
	//   [78 78]{start} [09]{length} [23]{type} [07]{status}
	//   [01 2C]{battery 300 = 3.0V} [1E]{rssi 30} [00 01]{serial} [CRC] [0D 0A]
	//
	// length = type(1) + status(1) + battery(2) + rssi(1) + serial(2) + CRC(2) = 9 (0x09)
	frame := []byte{0x78, 0x78, 0x09, MsgHeartbeat, 0x07, 0x01, 0x2C, 0x1E, 0x00, 0x01}
	crcVal := crc.CRC16X25(frame[2:])
	crcBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(crcBytes, crcVal)
	frame = append(frame, crcBytes...)
	frame = append(frame, 0x0D, 0x0A)

	pos, ack, err := Decode(frame, "102030405060708")
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if pos == nil {
		t.Fatal("heartbeat should return a position")
	}
	if !pos.Armed || !pos.Ignition || !pos.Charging {
		t.Errorf("heartbeat flags wrong: armed=%v ignition=%v charging=%v", pos.Armed, pos.Ignition, pos.Charging)
	}
	if pos.Battery != 3.0 {
		t.Errorf("heartbeat battery = %v, want 3.0", pos.Battery)
	}
	if pos.RSSI != 30 {
		t.Errorf("heartbeat rssi = %v, want 30", pos.RSSI)
	}
	if ack == nil {
		t.Fatal("heartbeat should return an ACK")
	}
}

func TestDecodeGPSLBS(t *testing.T) {
	// 0x12 GPS+LBS packet:
	//   [78 78]{start} [1F]{length=31} [12]{type}
	//   GPS(18): time(6) | sat(1) | lat(4) | lon(4) | speed(1) | flags(2)
	//   LBS(8): mcc(2) mnc(1) lac(2) cid(3)
	//   [serial:2] [CRC:2] [0D 0A]
	//
	// lat raw = 5400000 (=> 5400000/60/30000 = 3.0 degrees)
	// lon raw = 7200000 (=> 7200000/60/30000 = 4.0 degrees)
	// flags = valid(bit12) | north(bit10) | east(bit11 clear) | course 90  => 0x145A
	// NOTE: strict Traccar translation — bit10 SET => latitude stays positive,
	//       bit11 CLEAR => longitude stays positive.
	payload := make([]byte, 0)
	payload = append(payload,
		0x17, 0x06, 0x09, 0x0A, 0x1E, 0x00, // time 2023-06-09 10:30:00
		0x08, // satellites 8
	)
	latRaw := uint32(5400000)
	lonRaw := uint32(7200000)
	latBytes := make([]byte, 4)
	lonBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(latBytes, latRaw)
	binary.BigEndian.PutUint32(lonBytes, lonRaw)
	payload = append(payload, latBytes...)
	payload = append(payload, lonBytes...)
	payload = append(payload, 0x48) // speed 72 km/h
	flags := uint16(0x1000 | 0x0400 | 90) // valid + north + course 90
	flagBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(flagBytes, flags)
	payload = append(payload, flagBytes...)

	// LBS: mcc=460, mnc=0, lac=100, cid=200
	lbs := make([]byte, 0)
	lbs = append(lbs, 0x01, 0xCC)       // mcc 460
	lbs = append(lbs, 0x00)             // mnc 0
	lbs = append(lbs, 0x00, 0x64)       // lac 100
	lbs = append(lbs, 0x00, 0x00, 0xC8) // cid 200
	payload = append(payload, lbs...)

	// Build frameData = [type 0x12] + payload + [serial 2]
	frameData := append([]byte{MsgGPSLBS1}, payload...)
	frameData = append(frameData, 0x12, 0x34) // serial

	// length = type(1) + payload + serial(2) + CRC(2) = len(frameData) + 2
	length := len(frameData) + 2

	frame := make([]byte, 0, 64)
	frame = append(frame, 0x78, 0x78)
	frame = append(frame, byte(length))
	frame = append(frame, frameData...)
	crcVal := crc.CRC16X25(frame[2:])
	crcBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(crcBytes, crcVal)
	frame = append(frame, crcBytes...)
	frame = append(frame, 0x0D, 0x0A)

	if !ValidateCRC(frame) {
		t.Fatalf("built frame failed CRC")
	}

	pos, ack, err := Decode(frame, "102030405060708")
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if pos == nil {
		t.Fatal("GPS+LBS should return a position")
	}
	if pos.Latitude != 3.0 {
		t.Errorf("latitude = %v, want 3.0", pos.Latitude)
	}
	if pos.Longitude != 4.0 {
		t.Errorf("longitude = %v, want 4.0", pos.Longitude)
	}
	if pos.Speed != 72 {
		t.Errorf("speed = %v, want 72", pos.Speed)
	}
	if pos.Course != 90 {
		t.Errorf("course = %v, want 90", pos.Course)
	}
	if !pos.Valid {
		t.Errorf("valid = false, want true")
	}
	if ack != nil {
		t.Errorf("fire-and-forget location packet should NOT return an ACK")
	}
	if pos.Network == nil || pos.Network.MCC != 460 {
		t.Errorf("network missing/incorrect: %+v", pos.Network)
	}
	if pos.Network.LAC != 100 || pos.Network.CID != 200 {
		t.Errorf("cell info incorrect: %+v", pos.Network)
	}
}

func TestEncodeCommandCRC(t *testing.T) {
	cmd := EncodeEngineCutOff(0x0001)

	if !ValidateCRC(cmd) {
		t.Fatalf("EncodeCommand produced frame failing ValidateCRC: %X", cmd)
	}

	// Verify structure:
	//   [0:2] start 78 78
	//   [2]   length
	//   [3]   0x80 type
	//   [4]   cmdLen
	//   [5:9] reserved zeros
	//   [9:17] "Relay,1#" (8 chars)
	//   [17:19] serial 00 01
	//   [19:21] CRC
	//   [21:23] 0D 0A
	if cmd[0] != 0x78 || cmd[1] != 0x78 {
		t.Errorf("command start bits wrong")
	}
	if cmd[3] != MsgCmd0 {
		t.Errorf("command type = 0x%02X, want 0x80", cmd[3])
	}
	if cmd[4] != 12 {
		t.Errorf("cmdLen = %d, want 12 (4 reserved + 8 content)", cmd[4])
	}
	if string(cmd[9:17]) != "Relay,1#" {
		t.Errorf("command content = %q, want \"Relay,1#\"", cmd[9:17])
	}
	if cmd[17] != 0x00 || cmd[18] != 0x01 {
		t.Errorf("command serial wrong: %X %X", cmd[17], cmd[18])
	}
	if cmd[len(cmd)-2] != 0x0D || cmd[len(cmd)-1] != 0x0A {
		t.Errorf("command stop bits wrong")
	}
	// Length field should be 18 = 1(type)+1(cmdLen)+12(content)+2(serial)+2(CRC)
	if cmd[2] != 18 {
		t.Errorf("command length = %d, want 18", cmd[2])
	}
}

func TestEncodeAddressResponseCRC(t *testing.T) {
	resp := EncodeAddressResponse(0x0000)

	if !ValidateCRC(resp) {
		t.Fatalf("EncodeAddressResponse produced frame failing ValidateCRC: %X", resp)
	}
	// Must use extended start bits 0x79 0x79
	if resp[0] != 0x79 || resp[1] != 0x79 {
		t.Errorf("address response start = %X %X, want 79 79", resp[0], resp[1])
	}
	// Must contain the address response type 0x97
	if resp[4] != MsgAddressResp {
		t.Errorf("address response type = 0x%02X, want 0x97", resp[4])
	}
	if resp[len(resp)-2] != 0x0D || resp[len(resp)-1] != 0x0A {
		t.Errorf("address response stop bits wrong")
	}
}