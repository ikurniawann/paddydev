package recruitment

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"nuhabit/backend/internal/modules/recruitment/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Request bodies. Each parser reads fields in the zod schema's order and
// then applies the schema's custom Indonesian messages with msgs; the
// handlers pick how a failure renders (first issue, "path: message", or a
// fixed message), matching the TS helper the route used.

// form reads the body like `await request.json().catch(() => null)`: a
// missing or malformed body validates as null.
func form(r *http.Request) *validate.Form {
	v, _ := validate.ReadBody(r)
	return validate.New(v, true)
}

// msgs maps an issue path ("content", "options.*.text") optionally suffixed
// with ":<code>" to the custom message the zod schema declares.
type msgs map[string]string

func (m msgs) apply(f *validate.Form) {
	issues := f.Issues()
	for i := range issues {
		key := pathKey(issues[i].Path)
		code := issues[i].Code
		if code == "invalid_type" && strings.HasPrefix(issues[i].Message, "Invalid input: expected int") {
			code = "int" // a z.number().int("…") message
		}
		if s, ok := m[key+":"+code]; ok {
			issues[i].Message = s
		} else if s, ok := m[key]; ok {
			issues[i].Message = s
		}
	}
}

func pathKey(path []any) string {
	parts := make([]string, len(path))
	for i, p := range path {
		if _, isIndex := p.(int); isIndex {
			parts[i] = "*"
		} else {
			parts[i] = fmt.Sprint(p)
		}
	}
	return strings.Join(parts, ".")
}

// firstIssue renders parseBody / parseInput: 400 with the first issue's
// message and the issue list as details.
func firstIssue(f *validate.Form) error {
	if f.Valid() {
		return nil
	}
	return httpx.BadRequest(f.Issues()[0].Message, f.Issues())
}

// pathIssue renders parseJsonBody without a fixed message:
// "<path joined by .>: <message>", no details.
func pathIssue(f *validate.Form) error {
	if f.Valid() {
		return nil
	}
	first := f.Issues()[0]
	parts := make([]string, len(first.Path))
	for i, p := range first.Path {
		parts[i] = fmt.Sprint(p)
	}
	return httpx.BadRequest(strings.Join(parts, ".") + ": " + first.Message)
}

// fixedIssue renders parseJsonBody with a fixed message.
func fixedIssue(f *validate.Form, message string) error {
	if f.Valid() {
		return nil
	}
	return httpx.BadRequest(message)
}

var (
	optional = validate.Rule{Optional: true}
	nullish  = validate.Rule{Optional: true, Nullable: true}
	nullDef  = validate.Rule{Nullable: true, HasDefault: true}
)

// nullishText is z.string().trim().max(n).nullish().transform(v => v || null).
func nullishText(f *validate.Form, key string, max int, r validate.Rule) *string {
	s := f.Str(key, r, validate.StrOpts{Trim: true, Max: max})
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// optionalUUID is z.union([z.string().uuid(), z.literal("")]).nullish()
// .transform(v => v || null).
func optionalUUID(f *validate.Form, key string) *string {
	v, _, done := f.Take(key, "string", nullish)
	if done {
		return nil
	}
	s, ok := v.(string)
	if !ok || (s != "" && !validate.IsUUID(s)) {
		f.Fail(key, "invalid_union", "Invalid input")
		return nil
	}
	if s == "" {
		return nil
	}
	return &s
}

// emailPattern is zod v4's email regex minus its two lookaheads (checked in
// isEmail): no leading dot and no "..".
var emailPattern = regexp.MustCompile(`^[A-Za-z0-9_'+\-.]*[A-Za-z0-9_+-]@([A-Za-z0-9][A-Za-z0-9\-]*\.)+[A-Za-z]{2,}$`)

func isEmail(s string) bool {
	return !strings.HasPrefix(s, ".") && !strings.Contains(s, "..") && emailPattern.MatchString(s)
}

var (
	phonePattern   = regexp.MustCompile(`^[0-9+\-\s()]{8,20}$`)
	isoDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

func check(ok func(string) bool, code, msg string) func(string) (string, string, bool) {
	return func(s string) (string, string, bool) { return code, msg, ok(s) }
}

// ── candidates ──────────────────────────────────────────────────────────────

// candidateFields is the profile payload in CREATE_COLUMNS order; nil
// pointers in an update mean "not sent".
type candidateFields struct {
	FullName, Email, Phone, Domicile, Source *string
	BrandID, PositionID                      *string
	Status                                   *string
	Notes, LastExperience, LastEducation     *string
	Availability                             *string
	ExpectedSalary                           *int
	CvURL, PhotoURL                          *string
	// sent records the keys the payload carried (an update sets only those).
	sent map[string]bool
}

var candidateMsgs = msgs{
	"full_name:too_small":      "Nama minimal 2 karakter",
	"full_name:too_big":        "Nama maksimal 100 karakter",
	"email:invalid_format":     "Email tidak valid",
	"phone:invalid_format":     "Nomor telepon tidak valid",
	"domicile:too_small":       "Domisili wajib diisi",
	"domicile:too_big":         "Domisili maksimal 100 karakter",
	"cv_url:invalid_format":    "URL CV tidak valid",
	"photo_url:invalid_format": "URL foto tidak valid",
}

// parseCandidate reads candidateCreateSchema, or with update the partial
// candidateUpdateSchema (status dropped, nothing defaulted).
func parseCandidate(f *validate.Form, update bool) candidateFields {
	req := validate.Rule{}
	if update {
		req = optional
	}
	in := candidateFields{sent: map[string]bool{}}
	for key := range f.Fields() {
		in.sent[key] = true
	}
	in.FullName = f.Str("full_name", req, validate.StrOpts{Trim: true, Min: 2, Max: 100})
	in.Email = f.Str("email", req, validate.StrOpts{Trim: true, Check: check(isEmail, "invalid_format", "Invalid email address")})
	in.Phone = f.Str("phone", req, validate.StrOpts{Trim: true, Check: check(phonePattern.MatchString, "invalid_format", "Invalid string: must match pattern")})
	in.Domicile = f.Str("domicile", req, validate.StrOpts{Trim: true, Min: 1, Max: 100})
	if update {
		in.Source = f.Enum("source", optional, domain.CandidateSources)
	} else {
		src := f.StrDefault("source", "walk_in", validate.StrOpts{Check: validate.EnumCheck(domain.CandidateSources)})
		in.Source = &src
	}
	in.BrandID = optionalUUID(f, "brand_id")
	in.PositionID = optionalUUID(f, "position_id")
	if !update {
		st := f.StrDefault("status", "applied", validate.StrOpts{Check: validate.EnumCheck(domain.CreatableStatuses)})
		in.Status = &st
	}
	for _, c := range []struct {
		key string
		max int
		dst **string
	}{{"notes", 1000, &in.Notes}, {"last_experience", 300, &in.LastExperience}, {"last_education", 300, &in.LastEducation}} {
		*c.dst = nullishText(f, c.key, c.max, nullish)
	}
	in.Availability = f.Enum("availability", nullish, domain.Availabilities)
	in.ExpectedSalary = f.Int("expected_salary", nullish, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(2_000_000_000)})
	in.CvURL = f.Str("cv_url", nullish, validate.StrOpts{Check: validate.URLCheck})
	in.PhotoURL = f.Str("photo_url", nullish, validate.StrOpts{Check: validate.URLCheck})
	candidateMsgs.apply(f)
	return in
}

// columns lists the CREATE_COLUMNS the payload sets, with their values. For
// an update, status is never included and unsent fields are skipped.
func (c candidateFields) columns(update bool) ([]string, []any) {
	type col struct {
		name string
		val  any
		set  bool
	}
	nullable := func(name string, v any) col { return col{name, v, !update || c.sent[name]} }
	present := func(name string, p *string) col { return col{name, p, !update || p != nil} }
	all := []col{
		present("full_name", c.FullName),
		present("email", c.Email),
		present("phone", c.Phone),
		present("domicile", c.Domicile),
		present("source", c.Source),
		nullable("brand_id", c.BrandID),
		nullable("position_id", c.PositionID),
		{"status", c.Status, !update},
		nullable("notes", c.Notes),
		nullable("last_experience", c.LastExperience),
		nullable("last_education", c.LastEducation),
		nullable("availability", c.Availability),
		nullable("expected_salary", c.ExpectedSalary),
		nullable("cv_url", c.CvURL),
		nullable("photo_url", c.PhotoURL),
	}
	var names []string
	var vals []any
	for _, x := range all {
		if x.set {
			names = append(names, x.name)
			vals = append(vals, x.val)
		}
	}
	return names, vals
}

// candidateListQuery is candidateListQuerySchema.
type candidateListQuery struct {
	Status, BrandID, PositionID, Search, DateFrom, DateTo *string
	Sort                                                  string
	Page, Limit                                           int
	All                                                   bool
}

// parseCandidateListQuery keeps non-empty params (the last value wins, as
// in the URLSearchParams loop) and validates them.
func parseCandidateListQuery(r *http.Request) (candidateListQuery, *validate.Form) {
	raw := map[string]any{}
	for key, values := range r.URL.Query() {
		if v := values[len(values)-1]; v != "" {
			raw[key] = v
		}
	}
	f := validate.New(raw, true)
	q := candidateListQuery{}
	q.Status = f.Enum("status", optional, domain.CandidateStatuses)
	q.BrandID = f.UUID("brand_id", optional)
	q.PositionID = f.UUID("position_id", optional)
	q.Search = f.Str("search", optional, validate.StrOpts{Trim: true, Min: 1, Max: 100})
	dateCheck := check(isoDatePattern.MatchString, "invalid_format", "Tanggal tidak valid")
	q.DateFrom = f.Str("date_from", optional, validate.StrOpts{Check: dateCheck})
	q.DateTo = f.Str("date_to", optional, validate.StrOpts{Check: dateCheck})
	q.Sort = f.StrDefault("sort", "created_at", validate.StrOpts{Check: validate.EnumCheck([]string{"created_at", "updated_at"})})
	q.Page = coerceInt(f, raw, "page", 1, 1, 0)
	q.Limit = coerceInt(f, raw, "limit", 20, 1, 100)
	all := f.Enum("all", optional, []string{"true", "false"})
	q.All = all != nil && *all == "true"
	return q, f
}

// coerceInt is z.coerce.number().int().min(lo)[.max(hi)].default(def) on a
// query string value.
func coerceInt(f *validate.Form, raw map[string]any, key string, def, lo, hi int) int {
	s, ok := raw[key].(string)
	if !ok {
		return def
	}
	n := domain.JSNumber(s)
	if math.IsNaN(n) {
		f.Fail(key, "invalid_type", "Invalid input: expected number, received NaN")
		return def
	}
	o := validate.NumOpts{Integer: true, Min: validate.Bound(float64(lo))}
	if hi > 0 {
		o.Max = validate.Bound(float64(hi))
	}
	x, good := f.CheckNumber(key, json.Number(strconv.FormatFloat(n, 'g', -1, 64)), o)
	if !good {
		return def
	}
	return int(x)
}

// ── notes, screening, summaries ─────────────────────────────────────────────

func parseNote(f *validate.Form) string {
	s := f.Str("content", validate.Rule{}, validate.StrOpts{Trim: true, Min: 1, Max: 2000})
	msgs{"content": "Catatan tidak boleh kosong", "content:too_big": "Catatan maksimal 2000 karakter"}.apply(f)
	if s == nil {
		return ""
	}
	return *s
}

type screeningInput struct {
	Contacted                      bool
	Interested                     *bool
	AvailabilityNote               *string
	ConfirmedSalary                *int64
	WillingShift, WillingPlacement *bool
	Notes                          *string
	Recommendation                 *string
}

func parseScreening(f *validate.Form) screeningInput {
	in := screeningInput{}
	in.Contacted = f.BoolDefault("contacted", false)
	in.Interested = f.Bool("interested", nullDef)
	in.AvailabilityNote = f.Str("availability_note", nullDef, validate.StrOpts{Max: 200})
	if n := f.Num("confirmed_salary", nullDef, validate.NumOpts{Integer: true, Min: validate.Bound(0), Max: validate.Bound(1_000_000_000_000)}); n != nil {
		v := int64(*n)
		in.ConfirmedSalary = &v
	}
	in.WillingShift = f.Bool("willing_shift", nullDef)
	in.WillingPlacement = f.Bool("willing_placement", nullDef)
	in.Notes = f.Str("notes", nullDef, validate.StrOpts{Max: 2000})
	in.Recommendation = f.Enum("recommendation", nullDef, domain.Recommendations)
	msgs{
		"availability_note:too_big":  "Ketersediaan maksimal 200 karakter",
		"confirmed_salary:int":       "Gaji harus bilangan bulat",
		"confirmed_salary:too_small": "Gaji tidak boleh negatif",
		"confirmed_salary:too_big":   "Nominal gaji tidak wajar",
		"notes:too_big":              "Catatan maksimal 2000 karakter",
	}.apply(f)
	return in
}

type psikotesSummaryInput struct {
	Recommendation, Notes *string
}

func parsePsikotesSummary(f *validate.Form) psikotesSummaryInput {
	in := psikotesSummaryInput{
		Recommendation: f.Enum("recommendation", nullDef, domain.Recommendations),
		Notes:          f.Str("notes", nullDef, validate.StrOpts{Trim: true, Max: 2000}),
	}
	msgs{"notes:too_big": "Catatan maksimal 2000 karakter"}.apply(f)
	return in
}

// ── invitations and offers ─────────────────────────────────────────────────

type psikotesInviteInput struct {
	InstrumentIDs []string
	ExpiresDays   int
}

func parsePsikotesInvite(f *validate.Form) psikotesInviteInput {
	in := psikotesInviteInput{ExpiresDays: 3}
	before := len(f.Issues())
	items := f.List("instrument_ids", validate.Rule{}, 10, func(sub *validate.Form, i int, v any) {
		s, _ := sub.CheckString(i, v, validate.StrOpts{Check: validate.UUIDCheck})
		in.InstrumentIDs = append(in.InstrumentIDs, s)
	})
	if items != nil {
		if len(items) < 1 {
			f.Fail("instrument_ids", "too_small", "Pilih minimal satu instrumen")
		}
		if len(f.Issues()) == before && hasDuplicates(in.InstrumentIDs) {
			f.Fail("instrument_ids", "custom", "Instrumen duplikat")
		}
	}
	if n := f.Int("expires_days", validate.Rule{HasDefault: true}, validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(30)}); n != nil {
		in.ExpiresDays = *n
	}
	msgs{
		"instrument_ids.*:invalid_format": "ID instrumen tidak valid",
		"instrument_ids:too_big":          "Maksimal 10 instrumen",
		"expires_days:int":                "Masa berlaku harus bilangan bulat (hari)",
		"expires_days:too_small":          "Minimal 1 hari",
		"expires_days:too_big":            "Maksimal 30 hari",
	}.apply(f)
	return in
}

func hasDuplicates(ids []string) bool {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

type interviewInviteInput struct {
	ExpiresDays, MaxQuestions int
}

func parseInterviewInvite(f *validate.Form) interviewInviteInput {
	in := interviewInviteInput{ExpiresDays: 7, MaxQuestions: 8}
	if n := f.Int("expires_days", validate.Rule{HasDefault: true}, validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(30)}); n != nil {
		in.ExpiresDays = *n
	}
	if n := f.Int("max_questions", validate.Rule{HasDefault: true}, validate.NumOpts{Min: validate.Bound(3), Max: validate.Bound(15)}); n != nil {
		in.MaxQuestions = *n
	}
	return in
}

type offerInput struct {
	BaseSalary  float64
	Benefits    []string
	StartDate   *string
	Notes       *string
	ExpiresDays int
}

func parseOffer(f *validate.Form) offerInput {
	in := offerInput{ExpiresDays: 7, Benefits: []string{}}
	if n := f.Num("base_salary", validate.Rule{}, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(1_000_000_000_000)}); n != nil {
		in.BaseSalary = *n
	}
	if b := f.Strings("benefits", validate.Rule{HasDefault: true}, 15, validate.StrOpts{Trim: true, Min: 1, Max: 120}); b != nil {
		in.Benefits = b
	}
	in.StartDate = f.Str("start_date", nullish, validate.StrOpts{Check: check(isoDatePattern.MatchString, "invalid_format", "Format tanggal harus YYYY-MM-DD")})
	in.Notes = f.Str("notes", nullish, validate.StrOpts{Trim: true, Max: 2000})
	if n := f.Int("expires_days", validate.Rule{HasDefault: true}, validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(30)}); n != nil {
		in.ExpiresDays = *n
	}
	return in
}

// ── psikotes bank ───────────────────────────────────────────────────────────

var mcqKeys = []string{"a", "b", "c", "d", "e", "f"}

type instrumentInput struct {
	Name     *string
	IsActive *bool
	Config   map[string]any // only the keys sent, as zod outputs them
}

func parseInstrumentUpdate(f *validate.Form) instrumentInput {
	in := instrumentInput{}
	in.Name = f.Str("name", optional, validate.StrOpts{Trim: true, Min: 1, Max: 100})
	in.IsActive = f.Bool("is_active", optional)
	if v, sent := f.Fields()["config"]; sent {
		if _, isObj := v.(map[string]any); !isObj {
			f.Fail("config", "invalid_type", "Invalid input: expected object, received "+jsTypeName(v))
		} else {
			c := f.Child("config")
			in.Config = map[string]any{}
			if n := c.Int("duration_seconds", validate.Rule{}, validate.NumOpts{Min: validate.Bound(30), Max: validate.Bound(14400)}); n != nil {
				in.Config["duration_seconds"] = *n
			}
			if _, sent := c.Fields()["question_count"]; sent {
				n := c.Int("question_count", nullish, validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(200)})
				if n != nil {
					in.Config["question_count"] = *n
				} else {
					in.Config["question_count"] = nil
				}
			}
			if b := c.Bool("shuffle", optional); b != nil {
				in.Config["shuffle"] = *b
			}
			if s := c.Str("instructions", optional, validate.StrOpts{Trim: true, Max: 2000}); s != nil {
				in.Config["instructions"] = *s
			}
		}
	}
	msgs{
		"name:too_small":                    "Nama wajib diisi",
		"name:too_big":                      "Nama maksimal 100 karakter",
		"config.duration_seconds:int":       "Durasi harus bilangan bulat (detik)",
		"config.duration_seconds:too_small": "Durasi minimal 30 detik",
		"config.duration_seconds:too_big":   "Durasi maksimal 4 jam",
		"config.question_count:int":         "Jumlah soal harus bilangan bulat",
		"config.question_count:too_small":   "Jumlah soal minimal 1",
		"config.question_count:too_big":     "Jumlah soal maksimal 200",
		"config.instructions:too_big":       "Instruksi maksimal 2000 karakter",
	}.apply(f)
	if f.Valid() && in.Name == nil && in.IsActive == nil && in.Config == nil {
		f.Fail(nil, "custom", "Tidak ada perubahan yang dikirim")
	}
	return in
}

// questionInput is a validated bank question: options and answer key as the
// JSON the TS route stores.
type questionInput struct {
	Body      string
	Options   any
	AnswerKey any // nil for forced-choice
	SortOrder int
	IsActive  bool
}

var questionBaseMsgs = msgs{
	"sort_order:int":       "Urutan harus bilangan bulat",
	"sort_order:too_small": "Urutan tidak boleh negatif",
	"sort_order:too_big":   "Urutan maksimal 10000",
}

func parseQuestionBase(f *validate.Form, in *questionInput) {
	if n := f.Int("sort_order", validate.Rule{HasDefault: true}, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(10000)}); n != nil {
		in.SortOrder = *n
	}
	in.IsActive = f.BoolDefault("is_active", true)
}

type mcqOption struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

func parseMcqQuestion(f *validate.Form) questionInput {
	in := questionInput{}
	if s := f.Str("body", validate.Rule{}, validate.StrOpts{Trim: true, Min: 1, Max: 2000}); s != nil {
		in.Body = *s
	}
	opts := []mcqOption{}
	before := len(f.Issues())
	items := f.List("options", validate.Rule{}, 6, func(sub *validate.Form, i int, v any) {
		item := sub.Item(i, v)
		key := item.Enum("key", validate.Rule{}, mcqKeys)
		text := item.Str("text", validate.Rule{}, validate.StrOpts{Trim: true, Min: 1, Max: 500})
		if key != nil && text != nil {
			opts = append(opts, mcqOption{Key: *key, Text: *text})
		}
	})
	if items != nil {
		if len(items) < 2 {
			f.Fail("options", "too_small", "Minimal 2 opsi")
		}
		if len(f.Issues()) == before {
			keys := make([]string, len(opts))
			for i, o := range opts {
				keys[i] = o.Key
			}
			if hasDuplicates(keys) {
				f.Fail("options", "custom", "Key opsi tidak boleh duplikat")
			}
		}
	}
	in.Options = opts
	ak := f.Child("answer_key")
	correct := ak.Enum("correct", validate.Rule{}, mcqKeys)
	if correct != nil {
		in.AnswerKey = map[string]string{"correct": *correct}
	}
	parseQuestionBase(f, &in)
	msgs{
		"body:too_small":           "Soal wajib diisi",
		"body:too_big":             "Soal maksimal 2000 karakter",
		"options.*.text:too_small": "Teks opsi wajib diisi",
		"options.*.text:too_big":   "Teks opsi maksimal 500 karakter",
		"options:too_big":          "Maksimal 6 opsi",
	}.apply(f)
	questionBaseMsgs.apply(f)
	if f.Valid() && !slices.ContainsFunc(opts, func(o mcqOption) bool { return o.Key == *correct }) {
		f.Fail("answer_key", "custom", "Kunci jawaban harus salah satu key opsi")
	}
	return in
}

type papiStatement struct {
	Text  string `json:"text"`
	Scale string `json:"scale"`
}

func parsePapiQuestion(f *validate.Form) questionInput {
	in := questionInput{}
	in.Body = f.StrDefault("body", "", validate.StrOpts{Trim: true, Max: 500})
	opts := f.Child("options")
	var bForm *validate.Form
	statement := func(key string) papiStatement {
		s := opts.Child(key)
		if key == "b" {
			bForm = s
		}
		out := papiStatement{}
		if t := s.Str("text", validate.Rule{}, validate.StrOpts{Trim: true, Min: 1, Max: 500}); t != nil {
			out.Text = *t
		}
		if sc := s.Enum("scale", validate.Rule{}, domain.PapiScaleCodes); sc != nil {
			out.Scale = *sc
		}
		return out
	}
	a, b := statement("a"), statement("b")
	in.Options = map[string]papiStatement{"a": a, "b": b}
	parseQuestionBase(f, &in)
	msgs{
		"body:too_big":             "Keterangan maksimal 500 karakter",
		"options.a.text:too_small": "Pernyataan wajib diisi",
		"options.a.text:too_big":   "Pernyataan maksimal 500 karakter",
		"options.b.text:too_small": "Pernyataan wajib diisi",
		"options.b.text:too_big":   "Pernyataan maksimal 500 karakter",
	}.apply(f)
	questionBaseMsgs.apply(f)
	if f.Valid() && a.Scale == b.Scale {
		bForm.Fail("scale", "custom", "Skala pernyataan A dan B tidak boleh sama")
	}
	return in
}

func parseReview(f *validate.Form) string {
	s := f.Str("review_notes", validate.Rule{}, validate.StrOpts{Trim: true, Min: 1, Max: 2000})
	msgs{"review_notes:too_small": "Kesimpulan review wajib diisi", "review_notes:too_big": "Kesimpulan maksimal 2000 karakter"}.apply(f)
	if s == nil {
		return ""
	}
	return *s
}

// ── candidate portal ────────────────────────────────────────────────────────

// parseAnswers is sessionAnswersSchema: a record of question uuid → choice
// (trimmed, 1..4 chars), at most 500 entries. ok is false on any failure.
func parseAnswers(f *validate.Form) (map[string]string, bool) {
	raw, isObj := f.Fields()["answers"].(map[string]any)
	if f.Fields() == nil || !isObj || len(raw) > 500 {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		s, isStr := v.(string)
		if !validate.IsUUID(k) || !isStr {
			return nil, false
		}
		s = validate.JSTrim(s)
		if n := validate.UTF16Len(s); n < 1 || n > 4 {
			return nil, false
		}
		out[k] = s
	}
	return out, true
}

var framePattern = regexp.MustCompile(`^data:image/jpeg;base64,[A-Za-z0-9+/=]+$`)

const maxFrameChars = 400_000

// parseFrame is liveFrameSchema.
func parseFrame(f *validate.Form) (string, bool) {
	s, ok := f.Fields()["frame"].(string)
	if !ok || validate.UTF16Len(s) > maxFrameChars || !framePattern.MatchString(s) {
		return "", false
	}
	return s, true
}

// parseChatMessage is z.object({ message: z.string() }).
func parseChatMessage(f *validate.Form) (string, bool) {
	s, ok := f.Fields()["message"].(string)
	return s, ok
}

// parseSignal is {offer_id: string (matching re when set), sdp: 1..100000 chars}.
func parseSignal(f *validate.Form, offerID func(string) bool) (string, string, bool) {
	id, okID := f.Fields()["offer_id"].(string)
	sdp, okSDP := f.Fields()["sdp"].(string)
	if !okID || !okSDP || (offerID != nil && !offerID(id)) {
		return "", "", false
	}
	if n := validate.UTF16Len(sdp); n < 1 || n > domain.MaxSDPChars {
		return "", "", false
	}
	return id, sdp, true
}

type offerResponseInput struct {
	Action string
	Note   *string
}

// parseOfferRespond is offerRespondSchema.
func parseOfferRespond(f *validate.Form) offerResponseInput {
	in := offerResponseInput{}
	if a := f.Enum("action", validate.Rule{}, []string{"accept", "negotiate", "decline"}); a != nil {
		in.Action = *a
	}
	in.Note = f.Str("note", optional, validate.StrOpts{Trim: true, Max: 2000})
	return in
}

type manualResponseInput struct {
	Status string
	Note   *string
}

// parseManualResponse is offerManualResponseSchema.
func parseManualResponse(f *validate.Form) manualResponseInput {
	in := manualResponseInput{}
	if s := f.Enum("status", validate.Rule{}, []string{"negotiating", "accepted", "declined"}); s != nil {
		in.Status = *s
	}
	in.Note = f.Str("note", nullish, validate.StrOpts{Trim: true, Max: 2000})
	return in
}

// ── hris ────────────────────────────────────────────────────────────────────

type promotionInput struct {
	CandidateID      string
	JoinDate         *string
	EmploymentStatus *string
	DepartmentID     *string
	ReportingTo      *string
	// NIP is typed by HR (EPIC-081); the service refuses a blank one.
	NIP *string
}

const uuidRegexMessage = "Invalid string: must match pattern /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i"

// parsePromotion is promotionSchema.
func parsePromotion(f *validate.Form) promotionInput {
	in := promotionInput{}
	if s := f.Str("candidate_id", validate.Rule{}, validate.StrOpts{Min: 1}); s != nil {
		in.CandidateID = *s
	}
	in.JoinDate = f.Str("join_date", nullish, validate.StrOpts{})
	in.EmploymentStatus = f.Enum("employment_status", nullish, domain.EmploymentStatuses)
	uuidRe := check(domain.IsUUID, "invalid_format", uuidRegexMessage)
	in.DepartmentID = f.Str("department_id", nullish, validate.StrOpts{Check: uuidRe})
	in.ReportingTo = f.Str("reporting_to", nullish, validate.StrOpts{Check: uuidRe})
	in.NIP = f.Str("nip", nullish, validate.StrOpts{})
	msgs{"candidate_id": "candidate_id wajib diisi"}.apply(f)
	return in
}

type positionInput struct {
	Title      string
	Department *string
	Level      *string
	IsActive   *bool
	BrandID    *string
}

// parsePosition is positionSchema (lib/hris/master-data.ts).
func parsePosition(f *validate.Form) positionInput {
	in := positionInput{}
	if s := f.Str("title", validate.Rule{}, validate.StrOpts{Trim: true, Min: 1}); s != nil {
		in.Title = *s
	}
	text := func(key string) *string {
		s := f.Str(key, nullish, validate.StrOpts{})
		if s == nil || *s == "" {
			return nil
		}
		return s
	}
	in.Department = text("department")
	in.Level = text("level")
	in.IsActive = f.Bool("is_active", optional)
	in.BrandID = optionalUUID(f, "brand_id")
	msgs{"title": "Nama jabatan wajib diisi"}.apply(f)
	return in
}

func jsTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "number"
}
