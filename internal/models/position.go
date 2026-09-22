package models

import "time"

// Position represents a decoded GT06N telemetry record.
// Fields are mapped from the binary protocol as decoded by Gt06ProtocolDecoder.
type Position struct {
	DeviceID    string    `json:"device_id"`
	DeviceType  string    `json:"device_type"`
	Protocol    string    `json:"protocol"`
	Valid       bool      `json:"valid"`
	Timestamp   time.Time `json:"timestamp"`

	// GPS coordinates (decimal degrees)
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude,omitempty"`

	// Motion
	Speed  float64 `json:"speed"`  // km/h (converted from knots in protocol)
	Course float64 `json:"course"` // degrees

	// Device state
	Satellites int     `json:"satellites,omitempty"`
	Battery    float64 `json:"battery,omitempty"`    // voltage (e.g., 4.2V)
	BatteryPct int     `json:"battery_pct,omitempty"` // percentage 0-100
	RSSI       int     `json:"rssi,omitempty"`        // signal strength

	// Boolean flags
	Ignition bool `json:"ignition,omitempty"`
	Charging bool `json:"charging,omitempty"`
	Armed    bool `json:"armed,omitempty"`
	Blocked  bool `json:"blocked,omitempty"`

	// Alarms (GT06N alarm codes mapped to string labels)
	Alarms []string `json:"alarms,omitempty"`

	// LBS / Network info
	Network *NetworkInfo `json:"network,omitempty"`

	// Metadata
	Odometer    float64 `json:"odometer,omitempty"`     // km
	Event       int     `json:"event,omitempty"`         // event code
	StatusCode  int     `json:"status_code,omitempty"`   // raw status byte
	Power       float64 `json:"power,omitempty"`         // external power voltage
	RawType     int     `json:"raw_type,omitempty"`      // original message type byte
	RawPayload  string  `json:"raw_payload,omitempty"`   // hex dump of the frame
}

// NetworkInfo holds LBS / cellular tower data from the packet.
type NetworkInfo struct {
	MCC int `json:"mcc"` // Mobile Country Code
	MNC int `json:"mnc"` // Mobile Network Code
	LAC int `json:"lac"` // Location Area Code
	CID int `json:"cid"` // Cell ID
}

// Alarm type constants — human-readable labels for GT06N alarm codes.
const (
	AlarmSOS          = "sos"
	AlarmPowerCut     = "power_cut"
	AlarmVibration    = "vibration"
	AlarmLowBattery   = "low_battery"
	AlarmPowerOff     = "power_off"
	AlarmOverspeed    = "overspeed"
	AlarmGeofenceIn   = "geofence_enter"
	AlarmGeofenceOut  = "geofence_exit"
	AlarmRemoving     = "removing"
	AlarmTampering    = "tampering"
	AlarmDoor         = "door"
	AlarmAccident     = "accident"
	AlarmAcceleration = "acceleration"
	AlarmBraking      = "braking"
	AlarmCornering    = "cornering"
	AlarmFallDown     = "fall_down"
	AlarmJamming      = "jamming"
	AlarmGeneral      = "general"
	AlarmLock         = "lock"
	AlarmUnlock       = "unlock"
	AlarmFuelLeak     = "fuel_leak"
	AlarmTemperature  = "temperature"
	AlarmIdle         = "idle"
	AlarmTow          = "tow"
	AlarmLowPower     = "low_power"
	AlarmGeofence     = "geofence"
)
