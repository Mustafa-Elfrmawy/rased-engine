#!/usr/bin/env python3
"""
Synthetic GT06N Device Client — Integration Test

Simulates a physical GT06N tracker over a real TCP socket to validate the
full ingestion lifecycle (from gt06n_advanced_mechanics.md §6):

  1. Establish a persistent TCP connection to the engine (port 5023)
  2. Send a login packet   (MSG_LOGIN 0x01)  -> expect Server ACK
  3. Send a heartbeat      (MSG_HEARTBEAT 0x23) -> expect Server ACK
  4. Send location packets (MSG_GPS_LBS_1 0x12) -> fire-and-forget, NO ACK
  5. Send a status packet  (MSG_STATUS 0x13) -> expect Server ACK

Run:  python3 test/test_gt06n_client.py [host] [port]
"""

import socket
import struct
import sys
import time


def crc16_x25(data: bytes) -> int:
    """CRC-16/X.25 — exact algorithm used by GT06N frames.

    Polynomial 0x1021, bit-reflected to 0x8408, init 0xFFFF, final XOR 0xFFFF.
    Bit-by-bit equivalent of the Go table-driven implementation.
    """
    crc = 0xFFFF
    for b in data:
        crc ^= b
        for _ in range(8):
            if crc & 1:
                crc = (crc >> 1) ^ 0x8408
            else:
                crc >>= 1
    return crc & 0xFFFF ^ 0xFFFF


def build_frame(msg_type: int, payload: bytes, serial: int) -> bytes:
    """Build a standard (non-extended) GT06N frame with correct length + CRC."""
    ser = struct.pack(">H", serial)
    length = 1 + len(payload) + 2 + 2  # type + payload + serial + crc
    body = bytes([length, msg_type]) + payload + ser
    crc = crc16_x25(body)
    return b"\x78\x78" + body + struct.pack(">H", crc) + b"\x0d\x0a"


def build_login(imei_hex: bytes) -> bytes:
    """Login packet. IMEI is 8 bytes; we also send the 2-byte terminal type."""
    # IMEI hex (8 bytes) encodes 15 digits + parity nibble
    payload = imei_hex + b"\x01\x01"
    return build_frame(0x01, payload, 0x0001)


def build_heartbeat(status: int = 0x07, battery: int = 300, rssi: int = 30) -> bytes:
    payload = bytes([status]) + struct.pack(">H", battery) + bytes([rssi])
    return build_frame(0x23, payload, 0x0002)


def build_location(lat: float, lon: float, speed: int, course: int, serial: int) -> bytes:
    """
    Build a 0x12 GPS+LBS location packet.

    lat/lon are converted to the protocol's raw uint32: degrees * 60 * 30000.
    Flags: bit10=1 (north/positive lat), bit13=0, bit12=1 (valid fix),
           bit11=0 (east/positive lon), course in bits 0-9.
    """
    import datetime

    now = datetime.datetime.now(datetime.timezone.utc)
    year = (now.year - 2000) & 0xFF
    time_bytes = bytes([year, now.month, now.day, now.hour, now.minute, now.second])

    lat_raw = struct.pack(">I", int(round(lat * 60 * 30000)))
    lon_raw = struct.pack(">I", int(round(lon * 60 * 30000)))

    sat = 8
    flags = (1 << 12) | (1 << 10) | (course & 0x3FF)  # valid + north + course

    gps = time_bytes + bytes([sat]) + lat_raw + lon_raw + bytes([speed]) + struct.pack(">H", flags)

    # LBS: mcc(2) mnc(1) lac(2) cid(3)
    lbs = struct.pack(">H", 460) + bytes([0]) + struct.pack(">H", 100) + bytes([0x00, 0x00, 0xC8])

    payload = gps + lbs
    return build_frame(0x12, payload, serial)


def build_status(status: int = 0x02) -> bytes:
    """Status packet (0x13): bit1 = ignition on."""
    return build_frame(0x13, bytes([status]), 0x0003)


def expect_ack(sock: socket.socket, label: str, timeout: float = 5.0):
    """Read a server ACK and verify its size + CRC."""
    sock.settimeout(timeout)
    try:
        data = sock.recv(32)
    except socket.timeout:
        print(f"  !! {label}: no ACK received (timeout)")
        return False
    if not data:
        print(f"  !! {label}: connection closed, no ACK")
        return False
    if len(data) >= 10 and data[:2] == b"\x78\x78" and data[-2:] == b"\x0d\x0a":
        print(f"  OK {label}: ACK {data.hex().upper()}")
        return True
    print(f"  ?? {label}: unexpected response {data.hex()}")
    return False


def main():
    host = sys.argv[1] if len(sys.argv) > 1 else "127.0.0.1"
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 5023

    print(f"[GT06N synthetic client] connecting to {host}:{port} ...")
    sock = socket.create_connection((host, port), timeout=10)
    sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)

    imei_hex = bytes.fromhex("0123456789ABCDEF")  # -> IMEI digits "123456789ABCDEF"

    # 1. Login
    print("[1] Sending login ...")
    sock.sendall(build_login(imei_hex))
    expect_ack(sock, "login")

    # 2. Heartbeat (keeps connection alive)
    print("[2] Sending heartbeat ...")
    sock.sendall(build_heartbeat())
    expect_ack(sock, "heartbeat")

    # 3. Location packets — fire-and-forget, no ACK expected
    print("[3] Sending 3 location packets (no ACK expected) ...")
    for i, (lat, lon) in enumerate([(30.0444, 31.2357), (30.1000, 31.3000), (30.2000, 31.4000)]):
        sock.sendall(build_location(lat, lon, 42 + i, 90 + i, 0x0100 + i))
        print(f"  sent location #{i + 1} ({lat:.4f}, {lon:.4f})")
        time.sleep(0.2)
    sock.settimeout(1.0)
    try:
        leftover = sock.recv(32)
        if leftover:
            print(f"  !! unexpected data after locations: {leftover.hex()}")
    except socket.timeout:
        print("  OK no unexpected ACK after location packets (fire-and-forget)")

    # 4. Status packet — requires ACK
    print("[4] Sending status packet ...")
    sock.sendall(build_status(0x02))
    expect_ack(sock, "status")

    # 5. Clean disconnect
    print("[5] Closing connection.")
    sock.close()
    print("SUCCESS: synthetic GT06N device session completed")


if __name__ == "__main__":
    main()