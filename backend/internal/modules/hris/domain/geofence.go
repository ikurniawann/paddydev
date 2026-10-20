package domain

import "math"

// Geofence absensi (EPIC-062, MOM AB-01/AB-03, K-14): the distance from a
// clock-in/out position to the chosen store, judged against the store
// radius (100 m or a per-store override) with a capped GPS accuracy
// tolerance.

const (
	// GeofenceRadiusDefaultM is the MOM radius when the store has no override.
	GeofenceRadiusDefaultM = 100
	// GeofenceAccuracyCapM caps the GPS accuracy tolerance (EPIC-062 R3).
	GeofenceAccuracyCapM = 100.0
	// GeofenceNoteMinChars is the shortest out-of-area note (AB-03).
	GeofenceNoteMinChars = 5
)

// Statuses stored in hris.attendance.clock_in_geofence / clock_out_geofence.
const (
	GeofenceValid     = "valid"
	GeofenceInvalid   = "invalid"
	GeofenceUnchecked = "unchecked"
	GeofenceDinas     = "dinas"
)

// GeoPoint is a device position; Accuracy is the reported radius in meters.
type GeoPoint struct {
	Lat, Lng float64
	Accuracy *float64
}

// GeofenceInput is one clock event: the store coordinates and radius
// override, the device position, and whether an approved business trip
// covers the date.
type GeofenceInput struct {
	BranchLat, BranchLng *float64
	RadiusM              *int
	Point                *GeoPoint
	OnBusinessTrip       bool
}

// GeofenceResult is the verdict; DistanceM is nil when it cannot be measured.
type GeofenceResult struct {
	Status    string
	DistanceM *int
	RadiusM   int
}

// DistanceMeters is the haversine distance in meters (the same formula as
// frontend/src/lib/hris/placements.ts distanceMeters).
func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	rad := func(v float64) float64 { return v * math.Pi / 180 }
	dLat, dLng := rad(lat2-lat1), rad(lng2-lng1)
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Pow(math.Sin(dLng/2), 2)
	// Rounding can push h a hair above 1 for near-antipodal points; clamp so
	// Asin never returns NaN.
	return 2 * 6_371_000 * math.Asin(math.Sqrt(math.Min(1, h)))
}

// usableCoord rejects NaN, out-of-range and 0,0 positions (a GPS fix that
// failed, not a place in Indonesia).
func usableCoord(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lng) || math.Abs(lat) > 90 || math.Abs(lng) > 180 {
		return false
	}
	return lat != 0 || lng != 0
}

func accuracyTolerance(acc *float64) float64 {
	if acc == nil || math.IsNaN(*acc) || *acc <= 0 {
		return 0
	}
	return math.Min(*acc, GeofenceAccuracyCapM)
}

// Geofence judges one clock event (EPIC-062 R2/R3): dinas on an approved
// business trip; unchecked without usable store or device coordinates;
// invalid only when the rounded distance minus the capped accuracy exceeds
// the radius; valid otherwise.
func Geofence(in GeofenceInput) GeofenceResult {
	radius := GeofenceRadiusDefaultM
	if in.RadiusM != nil && *in.RadiusM > 0 {
		radius = *in.RadiusM
	}
	out := GeofenceResult{Status: GeofenceUnchecked, RadiusM: radius}
	if in.BranchLat != nil && in.BranchLng != nil && usableCoord(*in.BranchLat, *in.BranchLng) &&
		in.Point != nil && usableCoord(in.Point.Lat, in.Point.Lng) {
		d := int(math.Round(DistanceMeters(*in.BranchLat, *in.BranchLng, in.Point.Lat, in.Point.Lng)))
		out.DistanceM = &d
	}
	switch {
	case in.OnBusinessTrip:
		out.Status = GeofenceDinas
	case out.DistanceM == nil:
		out.Status = GeofenceUnchecked
	case float64(*out.DistanceM)-accuracyTolerance(in.Point.Accuracy) > float64(radius):
		out.Status = GeofenceInvalid
	default:
		out.Status = GeofenceValid
	}
	return out
}

// GeofenceNoteMissing: an out-of-area event needs a note of at least
// GeofenceNoteMinChars characters (AB-03); other statuses need none.
func GeofenceNoteMissing(status string, note *string) bool {
	if status != GeofenceInvalid {
		return false
	}
	return note == nil || len([]rune(JSTrim(*note))) < GeofenceNoteMinChars
}
