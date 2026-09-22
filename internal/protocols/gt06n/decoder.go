package gt06n

import (
	"encoding/binary"
	"fmt"
	"time"

	"go-tracker-service/internal/models"
)

// GT06N message type constants — translated from Gt06ProtocolDecoder.java
const (
	MsgLogin       = 0x01
	MsgGPS         = 0x10
	MsgGPSLBS6     = 0x11
	MsgGPSLBS1     = 0x12
	MsgStatus      = 0x13
	MsgSatellite   = 0x14
	MsgString      = 0x15
	MsgGPSLBSStat1 = 0x16
	MsgGPSLBS2     = 0x22
	MsgGPSLBSStat2 = 0x26
	MsgGPSLBSStat3 = 0x27
	MsgLBSMulti1   = 0x28
	MsgLBSMulti2   = 0x2E
	MsgLBSMulti3   = 0x24
	MsgLBSWifi     = 0x2C
	MsgLBSExtend   = 0x18
	MsgLBSStatus   = 0x19
	MsgGPSPhone    = 0x1A
	MsgGPSLBSExt   = 0x1E
	MsgHeartbeat   = 0x23
	MsgAddressReq  = 0x2A
	MsgAddressResp = 0x97
	MsgGPSLBS5      = 0x31
	MsgGPSLBSStat4  = 0x32
	MsgWifi5        = 0x33
	MsgLBS3         = 0x34
	MsgGPSLBS3      = 0x37
	MsgGPSLBS4      = 0x2D
	MsgGPSLBSDriver = 0x25
	MsgGPSLBSStat5  = 0xA2
	MsgGPSLBS8     = 0x38
	MsgAlarmModule = 0x39
	MsgCmd0        = 0x80
	MsgCmd1        = 0x81
	MsgCmd2        = 0x82
	MsgInfo        = 0x94
	MsgSerial      = 0x9B
	MsgAlarm       = 0x95
)

// hasGps returns true if the message type contains GPS data.
// Translated from Gt06ProtocolDecoder.java hasGps()
func hasGps(msgType int) bool {
	switch msgType {
	case MsgGPS, MsgGPSLBS1, MsgGPSLBS2, MsgGPSLBSDriver, MsgGPSLBS3,
		MsgGPSLBS4, MsgGPSLBS5, MsgGPSLBS6, MsgGPSLBSStat1, MsgGPSLBSStat2,
		MsgGPSLBSStat3, MsgGPSLBSStat4, MsgGPSLBSStat5, MsgGPSPhone,
		MsgGPSLBSExt, MsgAlarmModule:
		return true
	default:
		return false
	}
}

// hasLbs returns true if the message type contains LBS (cell tower) data.
func hasLbs(msgType int) bool {
	switch msgType {
	case MsgLBSStatus, MsgGPSLBS1, MsgGPSLBS2, MsgGPSLBSDriver, MsgGPSLBS3,
		MsgGPSLBS4, MsgGPSLBS5, MsgGPSLBS6, MsgGPSLBSStat1, MsgGPSLBSStat2,
		MsgGPSLBSStat3, MsgGPSLBSStat4, MsgGPSLBSStat5, MsgAlarmModule:
		return true
	default:
		return false
	}
}

// hasStatus returns true if the message type contains device status data.
func hasStatus(msgType int) bool {
	switch msgType {
	case MsgStatus, MsgLBSStatus, MsgGPSLBSStat1, MsgGPSLBSStat2,
		MsgGPSLBSStat3, MsgGPSLBSStat4, MsgGPSLBSStat5, MsgAlarmModule:
		return true
	default:
		return false
	}
}

// Decode is the main entry point for decoding a GT06N binary frame.
// It translates the Java Gt06ProtocolDecoder.decode() method.
//
// Parameters:
//   - data: The complete raw frame bytes (including start/stop bits and CRC)
//   - imei: The device IMEI string (populated from login, stored per-connection)
//
// Returns:
//   - *models.Position: Decoded position (nil for login/unsupported packets)
//   - []byte: ACK response to send back (nil if no ACK needed)
//   - error: Any decoding error
func Decode(data []byte, imei string) (*models.Position, []byte, error) {
	frame, err := DecodeFrame(data)
	if err != nil {
		return nil, nil, fmt.Errorf("frame decode error: %w", err)
	}

	// Parse the data portion of the frame
	d := frame.Data
	if len(d) < 2 {
		return nil, nil, fmt.Errorf("frame data too short")
	}

	// Extract message type (first byte of data)
	msgType := int(d[0])

	// Position 0: type byte
	// Positions 1..N: payload
	// Last 2 bytes before CRC: serial number

	// Serial number is always at the end of the data, before CRC
	// Data length = type(1) + payload + serial(2)
	// Serial is at d[len(d)-2 : len(d)]
	serial := binary.BigEndian.Uint16(d[len(d)-2 : len(d)])

	switch msgType {
	case MsgLogin:
		return decodeLogin(frame, d, serial)
	case MsgHeartbeat:
		return decodeHeartbeat(frame, d, imei, serial)
	case MsgGPSLBS1, MsgGPSLBS2, MsgGPSLBS6:
		return decodeGPSLBS(frame, d, imei, msgType, serial)
	case MsgGPS:
		return decodeGPSOnly(frame, d, imei, msgType, serial)
	case MsgStatus:
		return decodeStatus(frame, d, imei, serial)
	case MsgGPSLBSStat1, MsgGPSLBSStat2, MsgGPSLBSStat3, MsgGPSLBSStat4:
		return decodeGPSLBSStatus(frame, d, imei, msgType, serial)
	default:
		// Unknown or unsupported message type — send ACK but return nil position
		ack := buildACK(frame.Extended, msgType, serial)
		return nil, ack, nil
	}
}

// decodeLogin handles MSG_LOGIN (0x01) packets.
//
// Login packet data layout (after start+length):
// [0]    type = 0x01
// [1:9]  IMEI (8 bytes, BCD-encoded — last nibble is parity, we skip it)
// [9:11] serial number (big-endian uint16)
//
// Translation of Gt06ProtocolDecoder.java MSG_LOGIN case:
//   - Extract IMEI from 8 bytes using hexDump().substring(1) — drops first nibble (parity)
//   - Call getDeviceSession() — we mock this to always return true
//   - Send ACK with the echoed serial number
func decodeLogin(frame *GT06NFrame, d []byte, serial uint16) (*models.Position, []byte, error) {
	if len(d) < 11 {
		return nil, nil, fmt.Errorf("login packet too short: %d bytes", len(d))
	}

	// Extract IMEI from 8 bytes BCD
	// Java: ByteBufUtil.hexDump(buf.readSlice(8)).substring(1)
	// This converts 8 bytes to16 hex chars, then drops the first char (parity nibble)
	imeiBytes := d[1:9]
	imei := fmt.Sprintf("%016X", imeiBytes)
	// Drop first nibble (parity) — Java substring(1)
	imei = imei[1:]

	// Mock device session validation — always returns true
	// In production, this would query MySQL/cache for registered devices
	_ = getDeviceSession(imei)

	// Build and return ACK response
	ack := buildACK(frame.Extended, MsgLogin, serial)

	return nil, ack, nil
}

// decodeHeartbeat handles MSG_HEARTBEAT (0x23) packets.
//
// Heartbeat packets carry status flags and optional battery/RSSI data.
// The server must ACK heartbeat packets to keep the connection alive.
//
// Translation of Gt06ProtocolDecoder.java MSG_HEARTBEAT case:
//   - Read status byte: bit0=armed, bit1=ignition, bit2=charging
//   - Optional: battery voltage (uint16 / 100.0), RSSI (uint8)
//   - Send ACK
func decodeHeartbeat(frame *GT06NFrame, d []byte, imei string, serial uint16) (*models.Position, []byte, error) {
	if len(d) < 2 { // type(1) + status(1) minimum
		return nil, nil, fmt.Errorf("heartbeat packet too short")
	}

	pos := &models.Position{
		DeviceID:   imei,
		DeviceType: "GT06N",
		Protocol:   "gt06n",
		Timestamp:  time.Now().UTC(),
		RawType:    MsgHeartbeat,
	}

	// Status byte: bit0=armed, bit1=ignition, bit2=charging
	status := d[1]
	pos.Armed = status&0x01 != 0
	pos.Ignition = status&0x02 != 0
	pos.Charging = status&0x04 != 0
	pos.StatusCode = int(status)

	// Optional battery and RSSI.
	// Java reads directly after the status byte in stream order:
	//   battery = readUnsignedShort() / 100.0   (2 bytes following status)
	//   rssi    = readUnsignedByte()
	if len(d) >= 4 {
		pos.Battery = float64(binary.BigEndian.Uint16(d[2:4])) / 100.0
	}
	if len(d) >= 5 {
		pos.RSSI = int(d[4])
	}

	// Heartbeat requires ACK — device firmware will retry if not acknowledged
	ack := buildACK(frame.Extended, MsgHeartbeat, serial)

	return pos, ack, nil
}

// decodeGPSLBS handles GPS+LBS combination packets (0x12, 0x22, 0x11).
//
// These are the most common location packets from GT06N devices.
// Data layout (after type byte):
//   - GPS data (18 bytes): time(6) + satellites(1) + lat(4) + lon(4) + speed(1) + flags(2)
//   - LBS data (variable): mcc(2) + mnc(1) + lac(2) + cid(3) + rssi(?)
//
// Translation of Gt06ProtocolDecoder.java hasGps() + decodeGps() + decodeLbs() paths
func decodeGPSLBS(frame *GT06NFrame, d []byte, imei string, msgType int, serial uint16) (*models.Position, []byte, error) {
	pos := &models.Position{
		DeviceID:   imei,
		DeviceType: "GT06N",
		Protocol:   "gt06n",
		Timestamp:  time.Now().UTC(),
		RawType:    msgType,
	}

	// Data starts at offset 1 (after type byte)
	buf := d[1:]

	// Decode GPS portion
	if err := decodeGPSData(pos, buf); err != nil {
		// GPS decode failed — log but continue with LBS data
		fmt.Printf("[GT06N] GPS decode error for device %s: %v\n", imei, err)
	}

	// Decode LBS portion (starts after GPS data: 6 time + 1 sat + 4 lat + 4 lon + 1 speed + 2 flags = 18 bytes)
	lbsStart := 18
	if len(buf) > lbsStart {
		decodeLBSData(pos, buf[lbsStart:])
	}

	return pos, nil, nil // standard location packets = fire-and-forget, NO ACK
}

// decodeGPSOnly handles MSG_GPS (0x10) packets — GPS without LBS.
func decodeGPSOnly(frame *GT06NFrame, d []byte, imei string, msgType int, serial uint16) (*models.Position, []byte, error) {
	pos := &models.Position{
		DeviceID:   imei,
		DeviceType: "GT06N",
		Protocol:   "gt06n",
		Timestamp:  time.Now().UTC(),
		RawType:    msgType,
	}

	buf := d[1:] // skip type byte
	if err := decodeGPSData(pos, buf); err != nil {
		return nil, nil, fmt.Errorf("GPS decode error: %w", err)
	}

	return pos, nil, nil // fire-and-forget
}

// decodeGPSData extracts GPS coordinates from the binary buffer.
//
// Translation of Gt06ProtocolDecoder.decodeGps():
//
//	GPS layout: [time:6] [satellites:1] [latitude:4] [longitude:4] [speed:1] [flags:2]
//
// Coordinate calculation:
//
//	lat = readUnsignedInt() / 60.0 / 30000.0
//	lon = readUnsignedInt() / 60.0 / 30000.0
//	flags bits: 0-9=course, 10=lat_north, 11=lon_east, 12=valid
func decodeGPSData(pos *models.Position, buf []byte) error {
	// Minimum GPS data: 6(time) + 1(sat) + 4(lat) + 4(lon) + 1(speed) + 2(flags) = 18 bytes
	if len(buf) < 18 {
		return fmt.Errorf("GPS data too short: %d bytes", len(buf))
	}

	// Time (BCD-encoded): year(1) + month(1) + day(1) + hour(1) + minute(1) + second(1)
	year := 2000 + int(buf[0])
	month := int(buf[1])
	day := int(buf[2])
	hour := int(buf[3])
	minute := int(buf[4])
	second := int(buf[5])
	pos.Timestamp = time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)

	// Satellite count (lower 4 bits of byte 6)
	pos.Satellites = int(buf[6]) & 0x0F

	// Latitude: 4 bytes big-endian unsigned int
	latRaw := binary.BigEndian.Uint32(buf[7:11])
	latitude := float64(latRaw) / 60.0 / 30000.0

	// Longitude: 4 bytes big-endian unsigned int
	lonRaw := binary.BigEndian.Uint32(buf[11:15])
	longitude := float64(lonRaw) / 60.0 / 30000.0

	// Speed: 1 byte (km/h)
	pos.Speed = float64(buf[15])

	// Flags: 2 bytes big-endian
	// Bit 10: N/S (0=N, 1=S)
	// Bit 11: E/W (0=E, 1=W)
	// Bit 12: GPS valid
	// Bits 0-9: Course
	flags := binary.BigEndian.Uint16(buf[16:18])
	pos.Course = float64(flags & 0x03FF)        // bits 0-9
	pos.Valid = flags&0x1000 != 0                // bit 12
	if flags&0x0400 == 0 { latitude = -latitude } // bit 10: N/S
	if flags&0x0800 != 0 { longitude = -longitude } // bit 11: E/W

	pos.Latitude = latitude
	pos.Longitude = longitude

	return nil
}

// decodeLBSData extracts LBS (cell tower) information from the buffer.
//
// Translation of Gt06ProtocolDecoder.decodeLbs():
//
//	LBS layout: [mcc:2] [mnc:1] [lac:2] [cid:3] [rssi:1]
//	MCC is masked with 0x7FFF to get the actual value.
func decodeLBSData(pos *models.Position, buf []byte) {
	// Minimum LBS: mcc(2) + mnc(1) + lac(2) + cid(3) = 8 bytes
	if len(buf) < 8 {
		return
	}

	mcc := int(binary.BigEndian.Uint16(buf[0:2]) & 0x7FFF)
	mnc := int(buf[2])
	lac := int(binary.BigEndian.Uint16(buf[3:5]))
	cid := int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7]) // 3-byte CID (24-bit)

	pos.Network = &models.NetworkInfo{
		MCC: mcc,
		MNC: mnc,
		LAC: lac,
		CID: cid,
	}

	// RSSI if present
	if len(buf) > 8 {
		pos.RSSI = int(buf[8])
	}
}

// decodeStatus handles MSG_STATUS (0x13) packets.
//
// Translation of Gt06ProtocolDecoder.java MSG_STATUS case:
//   - Read status byte: bit1=ignition, bit2=charging, bit7=blocked
//   - Bits 3-6 encode alarm type (vibration, power cut, low battery, SOS, etc.)
//   - Send ACK
func decodeStatus(frame *GT06NFrame, d []byte, imei string, serial uint16) (*models.Position, []byte, error) {
	if len(d) < 2 {
		return nil, nil, fmt.Errorf("status packet too short")
	}

	pos := &models.Position{
		DeviceID:   imei,
		DeviceType: "GT06N",
		Protocol:   "gt06n",
		Timestamp:  time.Now().UTC(),
		RawType:    MsgStatus,
	}

	statusByte := d[1]
	pos.StatusCode = int(statusByte)
	pos.Ignition = statusByte&0x02 != 0   // bit 1
	pos.Charging = statusByte&0x04 != 0    // bit 2
	pos.Blocked = statusByte&0x80 != 0     // bit 7

	// Decode alarm from bits 3-6
	alarm := decodeStatusAlarm(statusByte)
	if alarm != "" {
		pos.Alarms = append(pos.Alarms, alarm)
	}

	// Status packets require ACK
	ack := buildACK(frame.Extended, MsgStatus, serial)

	return pos, ack, nil
}

// decodeStatusAlarm extracts alarm code from status byte bits 3-6.
// Translation of Gt06ProtocolDecoder.decodeStatus() switch statement.
func decodeStatusAlarm(status byte) string {
	alarmBits := (status >> 3) & 0x07 // bits 3-6
	switch alarmBits {
	case 1:
		return models.AlarmVibration
	case 2:
		return models.AlarmPowerCut
	case 3:
		return models.AlarmLowBattery
	case 4:
		return models.AlarmSOS
	case 6:
		return models.AlarmGeofence
	case 7:
		return models.AlarmRemoving
	default:
		return ""
	}
}

// decodeGPSLBSStatus handles combined GPS+LBS+Status packets (0x16, 0x26, 0x27, 0x2D).
// These contain GPS, LBS, AND device status in one packet.
func decodeGPSLBSStatus(frame *GT06NFrame, d []byte, imei string, msgType int, serial uint16) (*models.Position, []byte, error) {
	pos := &models.Position{
		DeviceID:   imei,
		DeviceType: "GT06N",
		Protocol:   "gt06n",
		Timestamp:  time.Now().UTC(),
		RawType:    msgType,
	}

	buf := d[1:] // skip type byte

	// Decode GPS
	if hasGps(msgType) {
		decodeGPSData(pos, buf)
	}

	// Decode LBS (starts after GPS data = 18 bytes)
	lbsStart := 18
	if hasLbs(msgType) && len(buf) > lbsStart {
		// LBS data has a length prefix in status packets
		if hasStatus(msgType) {
			lbsLen := int(buf[lbsStart])
			if lbsLen > 0 && len(buf) > lbsStart+1 {
				decodeLBSData(pos, buf[lbsStart+1:])
			}
		} else {
			decodeLBSData(pos, buf[lbsStart:])
		}
	}

	// Decode status (after GPS + LBS)
	if hasStatus(msgType) {
		statusStart := findStatusStart(d, msgType)
		if statusStart > 0 && statusStart < len(d) {
			statusByte := d[statusStart]
			pos.StatusCode = int(statusByte)
			pos.Ignition = statusByte&0x02 != 0
			pos.Charging = statusByte&0x04 != 0
			pos.Blocked = statusByte&0x80 != 0
			alarm := decodeStatusAlarm(statusByte)
			if alarm != "" {
				pos.Alarms = append(pos.Alarms, alarm)
			}
		}
	}

	// GPS+LBS+Status packets require ACK
	ack := buildACK(frame.Extended, msgType, serial)

	return pos, ack, nil
}

// findStatusStart locates the status byte within a GPS+LBS+Status packet.
// This is a heuristic based on packet structure analysis.
func findStatusStart(d []byte, msgType int) int {
	// GPS data ends at offset 19 (1 type + 18 GPS)
	// LBS data follows with a length prefix
	// Status byte follows after LBS
	if len(d) < 20 {
		return -1
	}

	gpsEnd := 19 // 1(type) + 18(GPS)
	if !hasLbs(msgType) || len(d) <= gpsEnd {
		return gpsEnd
	}

	// LBS length prefix
	lbsLen := int(d[gpsEnd])
	if lbsLen == 0 {
		return gpsEnd + 1
	}
	return gpsEnd + 1 + lbsLen
}

// getDeviceSession is a stub that mocks the database lookup for device validation.
//
// Anti-Spam Strategy (from gt06n_advanced_mechanics.md):
//   - The server ACKs even unregistered devices to prevent firmware reconnect loops
//   - Subsequent location packets from unregistered devices are silently dropped
//
// In production, this queries the MySQL database or Redis cache to check
// if the IMEI belongs to a registered device.
func getDeviceSession(imei string) bool {
	// TODO: Replace with actual database/cache lookup
	// For now, accept all devices (mock returning true)
	return true
}
