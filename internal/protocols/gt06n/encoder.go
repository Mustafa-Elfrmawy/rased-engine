package gt06n

import (
	"encoding/binary"
	"fmt"

	"github.com/Mustafa-Elfrmawy/rased-engine/pkg/crc"
)

// buildACK constructs a Server ACK packet for the GT06N protocol.
//
// Translation of Gt06ProtocolEncoder.java sendResponse():
//
//	ACK structure:
//	[0x78 0x78] or [0x79 0x79]  — start bits
//	[length]                      — 1 or 2 bytes
//	[type]                        — echoed message type
//	[serial: 2 bytes]             — echoed serial number (big-endian)
//	[CRC16: 2 bytes]              — CRC over length..serial
//	[0x0D 0x0A]                   — stop bits
//
// For login ACK, length = 5 (type + serial(2) + CRC(2))
func buildACK(extended bool, msgType int, serial uint16) []byte {
	var buf []byte

	if extended {
		buf = make([]byte, 11) // 2(start) + 2(len) + 1(type) + 2(serial) + 2(CRC) + 2(stop)
		binary.BigEndian.PutUint16(buf[0:2], frameStartLong)
		binary.BigEndian.PutUint16(buf[2:4], 5) // length
		buf[4] = byte(msgType)
		binary.BigEndian.PutUint16(buf[5:7], serial)
		// CRC over bytes[2:7]
		crcVal := crc.CRC16X25(buf[2:7])
		binary.BigEndian.PutUint16(buf[7:9], crcVal)
		buf[9] = 0x0D
		buf[10] = 0x0A
	} else {
		buf = make([]byte, 10) // 2(start) + 1(len) + 1(type) + 2(serial) + 2(CRC) + 2(stop)
		binary.BigEndian.PutUint16(buf[0:2], frameStartShort)
		buf[2] = 5 // length
		buf[3] = byte(msgType)
		binary.BigEndian.PutUint16(buf[4:6], serial)
		// CRC over bytes[2:6]
		crcVal := crc.CRC16X25(buf[2:6])
		binary.BigEndian.PutUint16(buf[6:8], crcVal)
		buf[8] = 0x0D
		buf[9] = 0x0A
	}

	return buf
}

// EncodeCommand builds a GT06N command packet for sending to the device.
//
// Translation of Gt06ProtocolEncoder.java encodeContent():
//
//	Command structure:
//	[0x78 0x78]           — start bits
//	[length]              — 1 byte (total payload size)
//	[0x80]                — MSG_COMMAND_0 type
//	[cmdLen]              — 1 byte (4 + content length)
//	[0x00 0x00 0x00 0x00] — 4 zero bytes (server flag)
//	[content bytes]       — ASCII command string
//	[serial: 2 bytes]     — message sequence number
//	[CRC16: 2 bytes]      — CRC over length..serial
//	[0x0D 0x0A]           — stop bits
func EncodeCommand(command string, serial uint16) []byte {
	contentLen := 4 + len(command) // 4 zero bytes + command string
	// GT06N length field counts everything from the byte after itself through
	// the CRC: type(1) + cmdLen(1) + content + serial(2) + CRC(2)
	length := 1 + 1 + contentLen + 2 + 2

	// Physical frame: 2(start) + 1(length byte) + length + 2(stop)
	frameSize := length + 5
	buf := make([]byte, frameSize)

	// Start bits
	binary.BigEndian.PutUint16(buf[0:2], frameStartShort)

	// Length
	buf[2] = byte(length)

	// Message type
	buf[3] = MsgCmd0

	// Command length (4 zero bytes + content)
	buf[4] = byte(contentLen)

	// 4 zero bytes (server flag / reserved)
	// buf[5:9] = 0x00 (already zero from make)

	// Command string
	copy(buf[9:9+len(command)], command)

	// Serial number
	serialStart := 9 + len(command)
	binary.BigEndian.PutUint16(buf[serialStart:serialStart+2], serial)

	// CRC over bytes[2 : serialStart+2]
	crcStart := 2
	crcEnd := serialStart + 2
	crcVal := crc.CRC16X25(buf[crcStart:crcEnd])
	binary.BigEndian.PutUint16(buf[crcEnd:crcEnd+2], crcVal)

	// Stop bits
	buf[crcEnd+2] = 0x0D
	buf[crcEnd+3] = 0x0A

	return buf
}

// EncodeEngineCutOff builds a command to cut off the vehicle engine.
// This sends the "Relay,1#" command to activate the relay.
//
// Translation of Gt06ProtocolEncoder.java TYPE_ENGINE_STOP:
//   - Default: "Relay,1#"
//   - G109 model: "DYD#"
//   - Alternative: "DYD,<password>#"
func EncodeEngineCutOff(serial uint16) []byte {
	return EncodeCommand("Relay,1#", serial)
}

// EncodeEngineResume builds a command to resume the vehicle engine.
// This sends the "Relay,0#" command to deactivate the relay.
//
// Translation of Gt06ProtocolEncoder.java TYPE_ENGINE_RESUME:
//   - Default: "Relay,0#"
//   - G109 model: "HFYD#"
//   - Alternative: "HFYD,<password>#"
func EncodeEngineResume(serial uint16) []byte {
	return EncodeCommand("Relay,0#", serial)
}

// EncodeCustomCommand wraps any arbitrary text command into a GT06N packet.
func EncodeCustomCommand(command string, serial uint16) []byte {
	return EncodeCommand(command, serial)
}

// EncodeAddressResponse builds the response for MSG_ADDRESS_REQUEST (0x2A).
//
// Translation of Gt06ProtocolDecoder.java MSG_ADDRESS_REQUEST case.
// Java calls sendResponse(channel, true /*extended*/, MSG_ADDRESS_RESPONSE, 0, content)
// where content = [8]{len} [0x00000000]{reserved} + "NA&&NA&&0##".
//
// Extended frame layout:
//
//	[0x79 0x79]{start} [length:2] [0x97]{type} + content + [serial:2] [CRC:2] [0D 0A]
func EncodeAddressResponse(serial uint16) []byte {
	response := "NA&&NA&&0##"
	contentLen := 1 + 4 + len(response)     // len byte + reserved(4) + response
	length := 1 + contentLen + 2 + 2        // type + content + serial + CRC
	frameSize := length + 6                 // extended: 2(start) + 2(length fields) + length + 2(stop)

	buf := make([]byte, frameSize)

	binary.BigEndian.PutUint16(buf[0:2], frameStartLong)
	binary.BigEndian.PutUint16(buf[2:4], uint16(length))
	buf[4] = MsgAddressResp

	// Content: length byte + 4 reserved zero bytes + response string
	buf[5] = byte(len(response) + 4)
	// buf[6:10] = zeros (reserved)
	copy(buf[10:10+len(response)], response)

	serialStart := 10 + len(response)
	binary.BigEndian.PutUint16(buf[serialStart:serialStart+2], serial)

	crcStart := 2
	crcEnd := serialStart + 2
	crcVal := crc.CRC16X25(buf[crcStart:crcEnd])
	binary.BigEndian.PutUint16(buf[crcEnd:crcEnd+2], crcVal)

	buf[crcEnd+2] = 0x0D
	buf[crcEnd+3] = 0x0A

	return buf
}

// FormatHex formats a byte slice as a hex string for logging.
func FormatHex(data []byte) string {
	return fmt.Sprintf("%X", data)
}
