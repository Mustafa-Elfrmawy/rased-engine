package main

import (
	"encoding/binary"
	"fmt"
	"github.com/Mustafa-Elfrmawy/rased-engine/pkg/crc"
)

func main() {
	frame := make([]byte, 0, 21)
	frame = append(frame, 0x78, 0x78)
	frame = append(frame, 0x0F)
	frame = append(frame, 0x01)
	imei := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	frame = append(frame, imei...)
	frame = append(frame, 0x00, 0x01)
	serial := []byte{0x2B, 0x76}
	frame = append(frame, serial...)
	crcVal := crc.CRC16X25(frame[2:])
	crcBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(crcBytes, crcVal)
	frame = append(frame, crcBytes...)
	frame = append(frame, 0x0D, 0x0A)
	for _, b := range frame {
		fmt.Printf("%02X ", b)
	}
	fmt.Println()
}
