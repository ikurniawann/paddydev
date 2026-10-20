package recruitment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
	"nuhabit/backend/internal/platform/whatsapp"
)

// POST /api/notifications/send: an HR action that sends a candidate a
// WhatsApp (default) or an email and logs it in notifications_log.

// notifyRoles may send candidate notifications; they also need the
// Rekrutmen menu (EPIC-057: akun Non-Rekrutmen tidak boleh mengirim).
var notifyRoles = []string{"super_admin", "admin", "hrd", "hiring_manager"}

// Sender delivers candidate notifications.
type Sender interface {
	// WhatsApp is sendWhatsAppText (provider switch and crm.wa_messages log).
	WhatsApp(ctx context.Context, q database.Querier, target, message string) (ok bool, reason string)
	// Email is sendEmail through Resend; false when not sent.
	Email(ctx context.Context, to, subject, html string) bool
}

type notifier struct {
	svc    *Service
	users  Guard
	sender Sender
}

type notifyInput struct {
	CandidateID, Channel                                  string
	Template, Message, InterviewDate, InterviewType, Mode *string
	MeetingLink, InterviewerName                          *string
}

func parseNotify(f *validate.Form) notifyInput {
	opt := validate.Rule{Optional: true}
	str := func(key string) *string { return f.Str(key, opt, validate.StrOpts{}) }
	in := notifyInput{Channel: "whatsapp"}
	if s := f.Str("candidate_id", validate.Rule{}, validate.StrOpts{Min: 1}); s != nil {
		in.CandidateID = *s
	}
	if c := f.Enum("channel", validate.Rule{HasDefault: true}, []string{"whatsapp", "email"}); c != nil {
		in.Channel = *c
	}
	in.Template = f.Enum("template", opt, []string{"interview_scheduled", "status_update"})
	in.Message = str("message")
	in.InterviewDate = str("interview_date")
	in.InterviewType = str("interview_type")
	in.Mode = f.Enum("mode", opt, []string{"offline", "online"})
	in.MeetingLink = str("meeting_link")
	in.InterviewerName = str("interviewer_name")
	return in
}

func orStr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

var nonDigit = regexp.MustCompile(`\D`)

// errNotifyBody is what request.json() throws inside validateBody on a
// missing or malformed body; apiHandler answers 500.
var errNotifyBody = errors.New("recruitment: request body is not valid JSON")

func (n *notifier) send(w http.ResponseWriter, r *http.Request) error {
	if n.users == nil {
		return httpx.Unauthorized("")
	}
	user, err := n.users.RequireMenuPrefix(r, iam.HrisRecruitment...)
	if err != nil {
		return err
	}
	if !slices.Contains(notifyRoles, user.Role) {
		return httpx.Forbidden("")
	}
	body, present := validate.ReadBody(r)
	if !present {
		return errNotifyBody
	}
	f := validate.New(body, true)
	in := parseNotify(f)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	ctx, db := r.Context(), n.svc.db
	var name, status string
	var email, phone *string
	err = db.QueryRow(ctx, `SELECT full_name, email, phone, status FROM candidates WHERE id = $1`, in.CandidateID).
		Scan(&name, &email, &phone, &status)
	if database.IsNoRows(err) {
		return httpx.NotFound("Kandidat tidak ditemukan")
	}
	if err != nil {
		return err
	}

	message := orStr(in.Message, "")
	if message == "" && in.Template != nil && *in.Template == "interview_scheduled" {
		message = interviewScheduledMessage(name, orStr(in.InterviewDate, ""), orStr(in.InterviewType, "Interview"),
			orStr(in.Mode, "offline"), orStr(in.MeetingLink, ""), orStr(in.InterviewerName, ""))
	}
	failure := ""
	if in.Channel == "whatsapp" {
		if message == "" {
			message = "Halo " + name + ", Status lamaran kamu saat ini: " + status
		}
		// The gateway takes a bare number, e.g. 6281234567890.
		if ok, reason := n.sender.WhatsApp(ctx, db, nonDigit.ReplaceAllString(orStr(phone, ""), ""), message); !ok {
			failure = reason
			if failure == "" {
				failure = "Gagal mengirim notifikasi"
			}
		}
	} else {
		subject, body := candidateStatusEmail(name, status, message)
		if !n.sender.Email(ctx, orStr(email, ""), subject, body) {
			failure = "Gagal mengirim email"
		}
	}

	logged, logStatus := message, "sent"
	if logged == "" {
		logged = "Notifikasi"
	}
	var sentAt *time.Time
	if failure != "" {
		logStatus = "failed"
	} else {
		now := n.svc.now()
		sentAt = &now
	}
	if _, err := db.Exec(ctx, `INSERT INTO notifications_log (candidate_id, channel, message, status, sent_at)
		VALUES ($1, $2, $3, $4, $5)`, in.CandidateID, in.Channel, logged, logStatus, sentAt); err != nil {
		return err
	}
	if failure != "" {
		return httpx.Status(http.StatusInternalServerError, failure)
	}
	return httpx.JSON(w, http.StatusOK, struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{true, "Notifikasi terkirim"})
}

var (
	jakarta     = mustJakarta()
	monthsLong  = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	weekdays    = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	dateOnlyRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	localLayout = []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"}
)

func mustJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}

// parseDateLike is lib/format toDate: a calendar date is noon WIB, other
// strings parse as new Date() would (no offset = server local time).
func parseDateLike(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if dateOnlyRe.MatchString(s) {
		t, err := time.ParseInLocation("2006-01-02 15:04", s+" 12:00", jakarta)
		return t, err == nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	for _, layout := range localLayout {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// interviewScheduledMessage is buildInterviewScheduledMessage: date and
// time in WIB ("-" when the date does not parse).
func interviewScheduledMessage(name, date, typ, mode, link, interviewer string) string {
	day, clock := "-", "-"
	if t, ok := parseDateLike(date); ok {
		t = t.In(jakarta)
		day = weekdays[t.Weekday()] + ", " + strconv.Itoa(t.Day()) + " " + monthsLong[t.Month()-1] + " " + strconv.Itoa(t.Year())
		clock = t.Format("15.04")
	}
	modeLabel := "Offline (Tatap Muka)"
	if mode == "online" {
		modeLabel = "Online (Zoom/Google Meet)"
	}
	lines := []string{
		"Halo " + name + ", jadwal interview telah ditentukan:", "",
		"📅 *" + typ + "*",
		"🗓️ Tanggal: " + day + ", " + clock + " WIB",
		"📍 Mode: " + modeLabel,
	}
	if mode == "online" && link != "" {
		lines = append(lines, "🔗 Link: "+link)
	}
	if interviewer != "" {
		lines = append(lines, "👤 Interviewer: "+interviewer)
	}
	lines = append(lines, "", "Mohon konfirmasi kehadiran. Terima kasih!")
	return strings.Join(lines, "\n")
}

var statusLabels = map[string]string{
	"applied": "Applied", "screening": "Screening", "psikotes": "Psikotes", "interview": "Interview",
	"offer": "Offer", "talent_pool": "Talent Pool", "hired": "Diterima", "rejected": "Tidak Diterima",
}

// escapeHTML is lib/security/escape-html.
func escapeHTML(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "&#34;", "&quot;")
}

// candidateStatusEmail is candidateStatusEmail (lib/resend).
func candidateStatusEmail(name, status, notes string) (subject, body string) {
	label := statusLabels[status]
	if label == "" {
		label = status
	}
	notesHTML := ""
	if notes != "" {
		notesHTML = "<p>Catatan: " + escapeHTML(notes) + "</p>"
	}
	return "Update Status Lamaran - " + name, `
      <h2>Hi ` + escapeHTML(name) + `,</h2>
      <p>Status lamaran kamu saat ini: <strong>` + escapeHTML(label) + `</strong></p>
      ` + notesHTML + `
      <p>Terima kasih sudah melamar di Aapex Technology.</p>
    `
}

// defaultSender sends WhatsApp through platform/whatsapp and email through
// the Resend REST API (RESEND_API_KEY; without it nothing is sent).
type defaultSender struct {
	wa     *whatsapp.Client
	getenv func(string) string
}

func (d defaultSender) WhatsApp(ctx context.Context, q database.Querier, target, message string) (bool, string) {
	res := d.wa.SendText(ctx, q, target, message, "", "")
	return res.Success, res.Reason
}

func (d defaultSender) Email(ctx context.Context, to, subject, body string) bool {
	key := strings.TrimSpace(d.getenv("RESEND_API_KEY"))
	if key == "" {
		return false
	}
	raw, _ := json.Marshal(map[string]string{"from": "Talent Pool <onboarding@resend.dev>", "to": to, "subject": subject, "html": body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(raw))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 300
}
