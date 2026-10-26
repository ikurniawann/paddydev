package hris

import (
	"context"
	"fmt"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
)

// Notifikasi harian ringkasan absen terlewat untuk HR (EPIC-067 R6, R7, R8).

const (
	// GapDigestEvery is how often the background loop looks for a date to
	// summarize.
	GapDigestEvery = 5 * time.Minute
	// gapDigestHour is the WIB hour from which yesterday's digest is sent.
	gapDigestHour = 9
	gapDigestHref = "/dashboard/hris/attendance"
)

// AttendanceGapDigestRepo is the digest marker and its in-app notification.
type AttendanceGapDigestRepo interface {
	// ClaimGapDigest inserts the marker of date; false when it already exists.
	ClaimGapDigest(ctx context.Context, date string) (bool, error)
	// NotifyHRDirect inserts one notification per HR account (the recipients
	// of public.notify_hrd) and returns how many were inserted.
	NotifyHRDirect(ctx context.Context, kind, title, message, link string) (int, error)
	// FinishGapDigest records the counts, the recipients and, when sent, the
	// send time on the marker of date.
	FinishGapDigest(ctx context.Context, date string, missing, noClockOut, unscheduled, recipients int, sentAt *time.Time) error
}

// GapDigestResult is one digest run that processed a date.
type GapDigestResult struct {
	Date       string
	Sent       bool
	Recipients int
	Summary    *GapSummary
}

// RunGapDigest summarizes yesterday's (WIB) missed attendance for the HR once
// per date. It does nothing (nil) before 09.00 WIB or when the date was
// already processed. Everything runs in one transaction: the marker, the
// summary, the notifications and the final counts commit together.
func (s *Service) RunGapDigest(ctx context.Context, now time.Time) (*GapDigestResult, error) {
	if domain.NowWIB(now).Hour() < gapDigestHour {
		return nil, nil
	}
	date := domain.AddDaysISO(domain.TodayWIB(now), -1)
	var result *GapDigestResult
	err := s.repo.InTx(ctx, func(r Repository) error {
		result = nil
		claimed, err := r.ClaimGapDigest(ctx, date)
		if err != nil || !claimed {
			return err
		}
		summary, err := s.AttendanceGapSummary(ctx, r, date)
		if err != nil {
			return err
		}
		res := &GapDigestResult{Date: date, Summary: summary}
		var sentAt *time.Time
		if summary.Missing+summary.NoClockOut > 0 {
			title, message := domain.GapDigestNotice(date, summary.Missing, summary.NoClockOut, summary.Unscheduled)
			if res.Recipients, err = r.NotifyHRDirect(ctx, "reminder", title, message, gapDigestHref); err != nil {
				return err
			}
			res.Sent = true
			at := now.UTC()
			sentAt = &at
		}
		if err := r.FinishGapDigest(ctx, date, summary.Missing, summary.NoClockOut, summary.Unscheduled, res.Recipients, sentAt); err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result != nil && result.Sent && result.Recipients == 0 {
		// The marker keeps sent_at (the date is done); nobody was told.
		s.log.Warn("hris gap digest has gaps but no HR recipients", "date", result.Date,
			"missing", result.Summary.Missing, "no_clock_out", result.Summary.NoClockOut)
	}
	return result, nil
}

// GapDigestLoop runs RunGapDigest once at start and then every interval until
// ctx ends. Errors and panics are logged; the loop never stops because of them.
func (s *Service) GapDigestLoop(ctx context.Context, every time.Duration) {
	s.gapDigestTick(ctx)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.gapDigestTick(ctx)
		}
	}
}

func (s *Service) gapDigestTick(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			s.log.Error("hris gap digest panicked", "panic", fmt.Sprint(p))
		}
	}()
	res, err := s.RunGapDigest(ctx, s.now())
	if err != nil {
		s.log.Error("hris gap digest failed", "error", err)
		return
	}
	if res != nil {
		s.log.Info("hris gap digest", "date", res.Date, "sent", res.Sent, "recipients", res.Recipients)
	}
}

func (s *store) ClaimGapDigest(ctx context.Context, date string) (bool, error) {
	tag, err := s.db.Exec(ctx, `INSERT INTO hris.attendance_gap_digests (date) VALUES ($1::date)
		ON CONFLICT (date) DO NOTHING`, date)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *store) NotifyHRDirect(ctx context.Context, kind, title, message, link string) (int, error) {
	tag, err := s.db.Exec(ctx, `INSERT INTO public.notifications (user_id, title, message, type, link)
		SELECT id, $1, $2, $3, $4 FROM configuration.users WHERE role = 'hrd'`, title, message, kind, link)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *store) FinishGapDigest(ctx context.Context, date string, missing, noClockOut, unscheduled, recipients int, sentAt *time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.attendance_gap_digests
		SET missing = $2, no_clock_out = $3, unscheduled = $4, recipients = $5, sent_at = $6
		WHERE date = $1::date`, date, missing, noClockOut, unscheduled, recipients, sentAt)
	return err
}
