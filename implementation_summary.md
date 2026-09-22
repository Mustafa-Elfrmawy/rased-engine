# GT06N Go Ingestion Engine — Implementation Summary

This document describes exactly what was built inside `go-tracker-service/`, how
the Traccar Java sources were translated, how the protocol is wired together, and
which values are placeholders.

---

## 1. Translated Java → Go (bitwise operations)

### 1.1 CRC-16/X.25 (`pkg/crc/crc16.go`)
`Traccar Checksum.java CRC16_X25` → `crc.CRC16X25(data []byte) uint16`.

- Polynomial `0x1021`, init `0xFFFF`, reflection `true`, final XOR `0xFFFF`.
- Implemented as a 256-entry byte-wise lookup table built from the **reflected**
  polynomial `0x8408` (identical math to Java).
- Verified against the standard check value: `CRC16X25("123456789") == 0x906E`.

### 1.2 Frame decoding (`internal/protocols/gt06n/frame_decoder.go`)
`Gt06FrameDecoder.java decode()` → `DecodeFrame()`, `ValidateCRC()`, and the
custom `GT06NSplitFunc`.

Key translation notes:
- **Start bits**: `0x78 0x78` (1-byte length) vs `0x79 0x79` (2-byte length).
- **Length semantics**: the GT06N length field counts everything **from the byte
  after itself through the CRC** (`type + payload + serial + CRC`). Therefore
  the *physical* frame size is `length + 5` (standard) or `length + 6`
  (extended), matching Java's `2 + 2 + 1 + length` calculation exactly.
- **Stop bits**: `0x0D 0x0A` checked at the exact computed position.
- **CRC coverage**: bytes `[2 : frameSize-4]`, i.e. from the length field through
  the serial number — mirrors Java `crc16(nioBuffer(2, writerIndex - 2))`.

### 1.3 Data parsing (`internal/protocols/gt06n/decoder.go`)
`Gt06ProtocolDecoder.java decode()` → `Decode()` and its subroutines.

| Java | Go | Notes |
|------|----|-------|
| `MSG_LOGIN` case | `decodeLogin` | IMEI = `hexDump(readSlice(8)).substring(1)` → `fmt.Sprintf("%016X", 8 bytes)[1:]` (drops parity nibble) |
| `MSG_HEARTBEAT` case | `decodeHeartbeat` | bit0=armed, bit1=ignition, bit2=charging; battery `uint16/100`; rssi `uint8` |
| `decodeGps()` | `decodeGPSData` | time BCD(6) | sat(count nibble) | lat u32/60/30000 | lon u32/60/30000 | speed | flags (course=bits0-9, N/S=bit10, E/W=bit11, valid=bit12) |
| `decodeLbs()` | `decodeLBSData` | mcc u16 (`& 0x7FFF`) | mnc u8 | lac u16 | cid u24 (3 bytes) |
| `decodeStatus()` | `decodeStatusAlarm` | bits 3-6 → alarm label (vibration/power-cut/low-battery/SOS/geofence/removing) |
| `getDeviceSession()` | `getDeviceSession` | **mocked to always return `true`** |

The N/S and E/W handling follows Traccar exactly:
`if !check(flags,10) { lat = -lat }` and `if check(flags,11) { lon = -lon }`.

### 1.4 Server ACK (`internal/protocols/gt06n/encoder.go`)
`Gt06ProtocolEncoder.java sendResponse()` → `buildACK()`.

- Standard ACK: `[78 78][05][type][serial:2][CRC16 over [len..serial]][0D 0A]`.
- Extended ACK: `[79 79][0005][type][serial:2][CRC][0D 0A]`.
- Serial is **echoed** from the device packet (read from `d[len(d)-2:]`).
- Verified against the synthetic client: login ACK = `78 78 05 01 00 01 D9 DC 0D 0A`.

### 1.5 Commands (`internal/protocols/gt06n/encoder.go`)
`Gt06ProtocolEncoder.java encodeContent()` → `EncodeCommand()`.

- `EncodeEngineCutOff()` → `"Relay,1#"` (default), `EncodeEngineResume()` → `"Relay,0#"`.
- Frame: `[78 78][len][0x80][cmdLen][0x00000000][ASCII command][serial][CRC][0D 0A]`,
  length = `type + cmdLen + content + serial + CRC` (matches Java `1+1+4+content+2+2`).
- `EncodeAddressResponse()` implements the extended `MSG_ADDRESS_RESPONSE`
  (0x79 0x79 header, content `[lenByte][0][0][0][0]"NA&&NA&&0##"`).

---

## 2. TCP Listener & Buffer Scanner (`internal/core/server.go`)

- **Port**: `5023` (hardcoded default, overridable via `TCP_PORT` env).
- **Connection-based routing (Layer 1)**: the listener blindly assumes any data
  on the port is GT06N.
- One lightweight **goroutine per accepted connection** (`go s.handleConnection(conn)`).
- **Idle management**: `conn.SetReadDeadline(now + 2m)` before every scan; reset on
  every successfully-read frame. On `os.ErrDeadlineExceeded` the connection is
  closed cleanly (dead-vehicle/tunnel case).
- **Keep-alive**: OS-level `SetKeepAlive(true)` + 2-minute period as a secondary net.
- **Stream fragmentation**: `bufio.Scanner` with `scanner.Split(GT06NSplitFunc)`
  and a 4KB initial / 2048 max buffer. The split function scans for the start bits
  (`0x78 0x78` / `0x79 0x79`), reads the length field, waits for the full frame,
  validates the `0x0D 0x0A` stop bits, and hands the complete frame to the URL-scanner.
  Garbage bytes before a valid header are skipped, so mid-stream resync works.
- **Selective ACKs**: after each `HandleFrame`, the returned ACK bytes are written
  back with a 5s write deadline. Login, heartbeat, and status/alarm packets ACK;
  standard location packets do **not** (fire-and-forget).

---

## 3. Middleware Pipeline (Handlers) (`internal/pipeline/`)

Mirrors Traccar's `ProcessingHandler` chain:

```
FilterHandler -> GeofenceHandler -> PublishHandler
```

- `PositionHandler` interface + `Pipeline.Process()` (recursive `run()` chaining).
- **`FilterHandler`**: validates coordinates (drops zero/out-of-range fixes).
- **`GeofenceHandler`**: empty pass-through with the comment
  `// TODO: Implement Tile38 integration later`.
- **`PublishHandler`**: final stage — marshals the enriched `Position` to JSON,
  logs it, and pushes it to Redis List `tracker:locations` via
  `redis.Client.PublishLocation()` (Laravel Queue worker consumes it). Designed to
  be trivially swapped for RabbitMQ.

The protocol handler (`gt06n_tcp/handler.go`) routes each frame type
(login/heartbeat/status/location), stores the per-connection `session.IMEI`, and
feeds decoded positions into the pipeline.

---

## 4. Assumptions, Hardcoded Values & Placeholders

| Item | Where | Status |
|------|-------|--------|
| Port `5023` | `internal/config/config.go` | Hardcoded default |
| Idle timeout `2m`, write timeout `5s` | `internal/core/server.go` | Hardcoded constant |
| `getDeviceSession()` always returns `true` | `decoder.go` | **Mock — DB validation is a TODO** |
| Tile38 geofence checks | `internal/pipeline/geofence.go` | **Empty pass-through** |
| Redis List vs RabbitMQ | `internal/redis/client.go` | Redis implemented, RabbitMQ is a swap target |
| Laravel queue worker | external project | Consumer of `tracker:locations`; not part of this codebase |
| Unsupported/unknown message types | `decoder.go` default case | ACK sent, no position produced |
| `models/payload.go` (old ESP JSON schema) | leftover | Retained for backwards-compat with the old handlers' consumers; the GT06N path uses `models/position.go` |
| `load_test/*.ts` (ports 8070/8071) | external test scripts | Target the old ESP/UDP devices; out of GT06N scope |

## 5. Verification performed

- `go build ./...`, `go vet ./...`, `go test ./...` — **all pass** in `golang:1.22`.
- Unit tests: CRC check-vector, frame decode/validate, login ACK byte layout,
  heartbeat flags/battery, GPS+LBS coordinates + network, command encoder layout,
  address-response encoder, split-func fragmentation/coalescing/garbage-recovery,
  pipeline chaining order and short-circuit.
- **End-to-end**: ran the server, connected the Python synthetic GT06N client
  (`test/test_gt06n_client.py`), and verified:
  - login → ACK `78 78 05 01 00 01 …`, heartbeat → ACK `…23 00 02 …`,
    status → ACK `…13 00 03 …`;
  - 3 location packets (lat 30.0444 / lon 31.2357 etc.) decoded and published
    through Filter → Geofence → Publish with **no** ACK (fire-and-forget).