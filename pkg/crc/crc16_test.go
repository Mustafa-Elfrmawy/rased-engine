package crc

import "testing"

// TestCRC16X25KnownVector verifies the CRC-16/X.25 against the standard
// check value 0x906E for the ASCII string "123456789".
func TestCRC16X25KnownVector(t *testing.T) {
	crc := CRC16X25([]byte("123456789"))
	if crc != 0x906E {
		t.Errorf("CRC16X25(\"123456789\") = 0x%04X, want 0x906E", crc)
	}
}

func TestCRC16X25Empty(t *testing.T) {
	// CRC-16/X.25 of empty input = 0xFFFF reflected... final xor gives 0x0000
	crc := CRC16X25(nil)
	if crc != 0x0000 {
		t.Errorf("CRC16X25(nil) = 0x%04X, want 0x0000", crc)
	}
}