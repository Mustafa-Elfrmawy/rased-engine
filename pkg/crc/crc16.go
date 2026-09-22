package crc

// CRC16/X25 lookup table.
// Polynomial: 0x1021, Initial: 0xFFFF, Final XOR: 0xFFFF, Reflect In/Out: true
// Translated from Traccar Checksum.java CRC16_X25.
var crc16x25Table [256]uint16

func init() {
	for i := 0; i < 256; i++ {
		crc := uint16(i)
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0x8408 // reflected polynomial of 0x1021
			} else {
				crc >>= 1
			}
		}
		crc16x25Table[i] = crc
	}
}

// CRC16X25 computes CRC-16/X.25 over the given data.
// This is the exact CRC used in GT06N protocol frames.
//
// Java equivalent: Checksum.crc16(Checksum.CRC16_X25, data)
func CRC16X25(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = (crc >> 8) ^ crc16x25Table[crc&0xFF^uint16(b)]
	}
	return crc ^ 0xFFFF
}
