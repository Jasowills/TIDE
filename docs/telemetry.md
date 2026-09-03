# Canonical telemetry (schemas/telemetry)

Fields: id, tenantId, vehicleId, deviceId, timestamp (event time),
receivedAt, processedAt, location{lat,lng,altitude?,accuracy?}, speed?,
heading?, ignition?, odometer?, fuelLevel?, batteryVoltage?, engine{},
sensors{}, raw{}, source{provider,protocol,deviceId},
metadata{correlationId, schemaVersion, sequence?, quality}.

- Raw payload never discarded. Units fixed: km/h, meters, °C, litres, WGS84, UTC.
- Three timestamps mandatory and distinct. Malformed → rejected with a
  `quality` reason, never silently dropped.
- Dedup identity: (provider, device, sequence) → fallback
  (provider, device, timestamp, payload hash).
- Schema changes: additive only; any change needs an ADR + second reviewer.
