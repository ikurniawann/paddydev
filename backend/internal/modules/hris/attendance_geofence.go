package hris

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
)

// Geofence absensi (EPIC-062): the status and distance of every clock-in
// and clock-out against the chosen store. Only an employee clocking
// themselves is judged; HR recording for someone else leaves the columns
// NULL (R6).

// clockGeofence is the verdict plus the store name for the 422 message.
type clockGeofence struct {
	domain.GeofenceResult
	BranchName string
}

// numPtr is a nullable numeric column (numeric arrives as text); nil for
// NULL or an unparsable value.
func numPtr(r *Row, key string) *float64 {
	if r == nil || r.Get(key) == nil {
		return nil
	}
	v := r.Num(key)
	if math.IsNaN(v) {
		return nil
	}
	return &v
}

// clockPoint is the device position of a clock location object; nil when
// the device sent none.
func clockPoint(loc *Row) *domain.GeoPoint {
	if loc == nil || !loc.Has("latitude") || !loc.Has("longitude") {
		return nil
	}
	return &domain.GeoPoint{Lat: loc.Num("latitude"), Lng: loc.Num("longitude"), Accuracy: numPtr(loc, "accuracy")}
}

// geofenceFor judges one clock event of employeeID on date at branchID;
// nil when the actor records for another employee. A non-HR employee is
// always judged on the server's today, never on a date they sent (P5): the
// date also decides the Perjalanan Dinas exemption.
func (s *Service) geofenceFor(ctx context.Context, a *Actor, employeeID, date string, branchID *string, loc *Row) (*clockGeofence, error) {
	if a.EmployeeID == nil || *a.EmployeeID != employeeID {
		return nil, nil
	}
	if !a.IsHR {
		date = domain.TodayWIB(s.now())
	}
	in := domain.GeofenceInput{Point: clockPoint(loc)}
	name := ""
	if branchID != nil && *branchID != "" {
		b, err := s.repo.BranchGeofence(ctx, *branchID)
		if err != nil {
			return nil, err
		}
		if b != nil {
			in.BranchLat, in.BranchLng = numPtr(b, "lat"), numPtr(b, "lng")
			if r := numPtr(b, "geofence_radius_m"); r != nil {
				v := int(*r)
				in.RadiusM = &v
			}
			name = b.Str("name")
		}
	}
	return &clockGeofence{GeofenceResult: domain.Geofence(in), BranchName: name}, nil
}

// requireGeofenceNote is the AB-03 popup contract: an out-of-area event
// without a note answers 422 geofence_note_required before anything is
// stored, so the client resends the same payload with notes.
func requireGeofenceNote(g *clockGeofence, note *string) error {
	if g == nil || !domain.GeofenceNoteMissing(g.Status, note) {
		return nil
	}
	place := g.BranchName
	var branchName any
	if place == "" {
		place = "toko"
	} else {
		branchName = place
	}
	distance := *g.DistanceM // invalid always has a distance
	return &httpx.Error{
		Status: http.StatusUnprocessableEntity,
		Message: fmt.Sprintf("Anda di luar area %s (±%d m, batas %d m). Isi keterangan minimal %d karakter untuk tetap absen.",
			place, distance, g.RadiusM, domain.GeofenceNoteMinChars),
		Details: obj("code", "geofence_note_required", "distance_m", distance, "radius_m", g.RadiusM, "branch_name", branchName),
	}
}

// status and distance are the column values; NULL when not judged.
func (g *clockGeofence) status() any {
	if g == nil {
		return nil
	}
	return g.Status
}

func (g *clockGeofence) distance() any {
	if g == nil || g.DistanceM == nil {
		return nil
	}
	return *g.DistanceM
}
