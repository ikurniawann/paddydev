package recruitment

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/extract"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/ocr"
	"nuhabit/backend/internal/platform/storage"
)

// The routes that read or write the shared storage directory, build the
// pipeline report PDF, or call the AI providers: CV upload, extraction and
// analysis, the candidate portals' recordings, answers, snapshots and
// drawings, HR's private file viewer and the public career form.

type fileHandler struct {
	*handler
	store *storage.Store
	ai    aiClient
	ocr   extract.OCR
}

func newFileHandler(h *handler) *fileHandler {
	return &fileHandler{
		handler: h,
		store:   storage.FromEnv(),
		ai:      aiClient{settings: h.svc.ports.Settings, http: &http.Client{}, getenv: os.Getenv, retryDelay: time.Second},
		ocr:     ocr.New(),
	}
}

func (f *fileHandler) db() database.DB { return f.svc.db }

func (f *fileHandler) routes() []module.Route {
	rec := iam.HrisRecruitment
	return []module.Route{
		{Pattern: "DELETE /api/candidates/{id}", Handler: f.del(delCandidate, f.staff(rec, f.deleteCandidate))},
		{Pattern: "POST /api/candidates/{id}/cv-upload", Handler: f.staff(rec, f.uploadCV)},
		{Pattern: "DELETE /api/candidates/{id}/cv-upload", Handler: f.del(delCV, f.staff(rec, f.removeCV))},
		{Pattern: "GET /api/candidates/{id}/report", Handler: f.staff(rec, f.pipelineReport)},
		{Pattern: "POST /api/candidates/cv-extract", Handler: f.staff(rec, f.cvExtract)},
		{Pattern: "GET /api/candidates/{id}/ai-analysis", Handler: f.staff(rec, f.getAnalysis)},
		{Pattern: "POST /api/candidates/{id}/ai-analysis", Handler: f.staff(rec, f.analyzeCV)},

		{Pattern: "GET /api/interview/files/{path...}", Handler: f.staff(rec, f.privateFile("interview"))},
		{Pattern: "GET /api/psikotes/files/{path...}", Handler: f.staff(rec, f.privateFile("psikotes"))},
		{Pattern: "GET /api/interview/sessions/{id}/recordings", Handler: f.staff(rec, f.recordings)},
		{Pattern: "POST /api/psikotes/session-tests/{id}/ai-insight", Handler: f.staff(rec, f.drawingInsight)},

		{Pattern: "GET /api/interview/session/{token}", Handler: httpx.Handle(f.interviewSession)},
		{Pattern: "POST /api/interview/session/{token}/start", Handler: httpx.Handle(f.startInterview)},
		{Pattern: "POST /api/interview/session/{token}/answer", Handler: httpx.Handle(f.answerInterview)},
		{Pattern: "POST /api/interview/session/{token}/recording-chunk", Handler: httpx.Handle(f.recordingChunk)},
		{Pattern: "POST /api/interview/session/{token}/proctor-event", Handler: httpx.Handle(f.interviewProctorEvent)},
		{Pattern: "POST /api/psikotes/session/{token}/proctor-event", Handler: httpx.Handle(f.psikotesProctorEvent)},
		{Pattern: "POST /api/psikotes/session/{token}/tests/{testId}/upload", Handler: httpx.Handle(f.uploadDrawing)},

		{Pattern: "POST /api/portal/submit", Handler: httpx.Handle(f.portalSubmit)},
	}
}

// privateFile is privateFileResponse: HR reads storage/private/<folder>/…;
// segments are decoded like Next's catch-all params, then checked by
// safeSegmentsUnder. Interview recordings are served as video/webm.
func (f *fileHandler) privateFile(folder string) staffFunc {
	prefix := "/api/" + folder + "/files/"
	return func(w http.ResponseWriter, r *http.Request, _ Actor) error {
		notFound := httpx.NotFound("File tidak ditemukan")
		var segments []string
		for _, raw := range strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), prefix), "/") {
			seg, err := storage.DecodeURIComponent(raw)
			if err != nil {
				return notFound
			}
			segments = append(segments, seg)
		}
		safe := storage.SafeSegmentsUnder(segments, folder, 2)
		if safe == nil {
			return notFound
		}
		rel := strings.Join(safe, "/")
		data, mime, err := f.store.ReadPrivate(rel)
		if err != nil {
			return notFound
		}
		if folder == "interview" && strings.Contains(rel, "/recording/") {
			mime = "video/webm"
		}
		storage.WritePrivateFile(w, data, mime, "private, max-age=300", "")
		return nil
	}
}

// readForm is request.formData() under limit: a body over the limit is 413
// with tooLarge, any other failure 400 with invalid.
func readForm(r *http.Request, limit int64, invalid, tooLarge string) (*storage.Form, error) {
	form, err := storage.ReadForm(r, limit)
	switch {
	case errors.Is(err, storage.ErrBodyTooLarge):
		return nil, httpx.Status(http.StatusRequestEntityTooLarge, tooLarge)
	case err != nil:
		return nil, httpx.BadRequest(invalid)
	}
	return form, nil
}

// aiError maps a missing API key to 400 with its message; anything else
// is logged and answered with fallback (500).
func (f *fileHandler) aiError(err error, context, fallback string) error {
	var nc notConfigured
	if errors.As(err, &nc) {
		return httpx.BadRequest(nc.Error())
	}
	f.svc.log.Error("["+context+"] gagal", "error", err)
	return httpx.Status(http.StatusInternalServerError, fallback)
}
