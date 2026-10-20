package recruitment

import (
	"net/http"
	"strconv"

	"nuhabit/backend/internal/modules/recruitment/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
)

// Guard is the slice of platform/auth the handlers use.
type Guard interface {
	RequireMenuPrefix(r *http.Request, prefixes ...string) (*auth.User, error)
	RequireMenuAction(r *http.Request, action string, prefixes ...string) (*auth.User, error)
}

type handler struct {
	svc   *Service
	guard Guard
}

// Routes lists the JSON routes; the file, PDF and AI routes are in files.go.
func (h *handler) Routes() []module.Route {
	rec := iam.HrisRecruitment
	return []module.Route{
		{Pattern: "GET /api/candidates", Handler: h.staff(rec, h.listCandidates)},
		{Pattern: "POST /api/candidates", Handler: h.staff(rec, h.createCandidate)},
		{Pattern: "GET /api/candidates/{id}", Handler: h.staff(rec, h.getCandidate)},
		{Pattern: "PUT /api/candidates/{id}", Handler: h.staff(rec, h.updateCandidate)},
		{Pattern: "GET /api/candidates/{id}/notes", Handler: h.staff(rec, h.listNotes)},
		{Pattern: "POST /api/candidates/{id}/notes", Handler: h.staff(rec, h.addNote)},
		{Pattern: "GET /api/candidates/{id}/activities", Handler: h.staff(rec, h.listActivities)},
		{Pattern: "POST /api/candidates/{id}/activities", Handler: h.staff(rec, h.logTemplate)},
		{Pattern: "POST /api/candidates/{id}/stage", Handler: h.staff(rec, h.moveStage)},
		{Pattern: "GET /api/candidates/{id}/screening", Handler: h.staff(rec, h.getScreening)},
		{Pattern: "PUT /api/candidates/{id}/screening", Handler: h.staff(rec, h.saveScreening)},
		{Pattern: "GET /api/candidates/{id}/psikotes", Handler: h.staff(rec, h.psikotesPanel)},
		{Pattern: "POST /api/candidates/{id}/psikotes/sessions", Handler: h.staff(rec, h.invitePsikotes)},
		{Pattern: "PUT /api/candidates/{id}/psikotes/summary", Handler: h.staff(rec, h.savePsikotesSummary)},
		{Pattern: "GET /api/candidates/{id}/interview", Handler: h.staff(rec, h.interviewPanel)},
		{Pattern: "POST /api/candidates/{id}/interview/sessions", Handler: h.staff(rec, h.inviteInterview)},
		{Pattern: "GET /api/candidates/{id}/offers", Handler: h.staff(rec, h.offerPanel)},
		{Pattern: "POST /api/candidates/{id}/offers", Handler: h.staff(rec, h.createOffer)},

		{Pattern: "GET /api/psikotes/instruments", Handler: h.staff(rec, h.listInstruments)},
		{Pattern: "PUT /api/psikotes/instruments/{id}", Handler: h.staff(rec, h.updateInstrument)},
		{Pattern: "GET /api/psikotes/instruments/{id}/questions", Handler: h.staff(rec, h.listQuestions)},
		{Pattern: "POST /api/psikotes/instruments/{id}/questions", Handler: h.staff(rec, h.createQuestion)},
		{Pattern: "PUT /api/psikotes/questions/{id}", Handler: h.staff(rec, h.updateQuestion)},
		{Pattern: "DELETE /api/psikotes/questions/{id}", Handler: h.del(delQuestion, h.staff(rec, h.deleteQuestion))},
		{Pattern: "GET /api/psikotes/session-tests/{id}/answers", Handler: h.staff(rec, h.answerDetail)},
		{Pattern: "PUT /api/psikotes/session-tests/{id}/review", Handler: h.staff(rec, h.reviewTest)},
		{Pattern: "GET /api/psikotes/sessions/{id}/proctor-events", Handler: h.staff(rec, h.proctorEvents("psikotes"))},
		{Pattern: "GET /api/interview/sessions/{id}/proctor-events", Handler: h.staff(rec, h.proctorEvents("interview"))},

		{Pattern: "GET /api/psikotes/session/{token}", Handler: httpx.Handle(h.psikotesSession)},
		{Pattern: "POST /api/psikotes/session/{token}/start", Handler: httpx.Handle(h.startPsikotes)},
		{Pattern: "POST /api/psikotes/session/{token}/finish", Handler: httpx.Handle(h.finishPsikotes)},
		{Pattern: "POST /api/psikotes/session/{token}/tests/{testId}/start", Handler: httpx.Handle(h.startTest)},
		{Pattern: "PUT /api/psikotes/session/{token}/tests/{testId}/answers", Handler: httpx.Handle(h.saveAnswers)},
		{Pattern: "POST /api/psikotes/session/{token}/tests/{testId}/finish", Handler: httpx.Handle(h.finishTest)},
		{Pattern: "GET /api/psikotes/session/{token}/chat", Handler: httpx.Handle(h.portalChat(psikotesPortal))},
		{Pattern: "POST /api/psikotes/session/{token}/chat", Handler: httpx.Handle(h.portalPostChat(psikotesPortal))},
		{Pattern: "POST /api/psikotes/session/{token}/live-frame", Handler: httpx.Handle(h.portalFrame(psikotesPortal))},
		{Pattern: "GET /api/psikotes/session/{token}/webrtc", Handler: httpx.Handle(h.portalOffers(psikotesPortal))},
		{Pattern: "POST /api/psikotes/session/{token}/webrtc", Handler: httpx.Handle(h.portalAnswer(psikotesPortal))},
		{Pattern: "GET /api/interview/session/{token}/chat", Handler: httpx.Handle(h.portalChat(interviewPortal))},
		{Pattern: "POST /api/interview/session/{token}/chat", Handler: httpx.Handle(h.portalPostChat(interviewPortal))},
		{Pattern: "POST /api/interview/session/{token}/live-frame", Handler: httpx.Handle(h.portalFrame(interviewPortal))},
		{Pattern: "GET /api/interview/session/{token}/webrtc", Handler: httpx.Handle(h.portalOffers(interviewPortal))},
		{Pattern: "POST /api/interview/session/{token}/webrtc", Handler: httpx.Handle(h.portalAnswer(interviewPortal))},

		{Pattern: "GET /api/offer/session/{token}", Handler: httpx.Handle(h.offerSession)},
		{Pattern: "POST /api/offer/session/{token}/respond", Handler: httpx.Handle(h.respondOffer)},
		{Pattern: "PUT /api/offers/{id}/response", Handler: http.HandlerFunc(h.manualOfferResponse)},

		{Pattern: "GET /api/recruitment/live-monitoring", Handler: h.staff(rec, h.liveSessions)},
		{Pattern: "GET /api/recruitment/live-monitoring/{type}/{id}/chat", Handler: h.staff(rec, h.liveChat)},
		{Pattern: "POST /api/recruitment/live-monitoring/{type}/{id}/chat", Handler: h.staff(rec, h.livePostChat)},
		{Pattern: "GET /api/recruitment/live-monitoring/{type}/{id}/frame", Handler: h.staff(rec, h.liveFrame)},
		{Pattern: "POST /api/recruitment/live-monitoring/{type}/{id}/webrtc", Handler: h.staff(rec, h.livePutOffer)},
		{Pattern: "GET /api/recruitment/live-monitoring/{type}/{id}/webrtc", Handler: h.staff(rec, h.liveAnswer)},

		{Pattern: "GET /api/hris/job-openings", Handler: h.staff(rec, h.listJobOpenings)},
		{Pattern: "POST /api/hris/job-openings", Handler: h.staff(rec, h.createJobOpening)},
		{Pattern: "PATCH /api/hris/job-openings/{id}", Handler: h.staff(rec, h.updateJobOpening)},
		{Pattern: "DELETE /api/hris/job-openings/{id}", Handler: h.del(delJobOpening, h.staff(rec, h.deleteJobOpening))},
		{Pattern: "POST /api/hris/promote", Handler: h.staff(rec, h.promote)},
		{Pattern: "GET /api/job-openings/public", Handler: http.HandlerFunc(h.publicJobOpenings)},
		{Pattern: "GET /api/positions", Handler: h.staff(iam.Hris, h.listPositions)},
		{Pattern: "POST /api/positions", Handler: h.staff(iam.HrisMaster, h.createPosition)},
	}
}

type staffFunc func(w http.ResponseWriter, r *http.Request, actor Actor) error

// staff guards a route by IAM menu prefix (requireIamMenuPrefix).
func (h *handler) staff(prefixes []string, fn staffFunc) http.Handler {
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.guard.RequireMenuPrefix(r, prefixes...)
		if err != nil {
			return err
		}
		return fn(w, r, Actor{ID: u.ID, FullName: u.FullName, Role: u.Role})
	})
}

// reply writes an ordered JSON object built from key, value pairs (the
// recruitment routes answer `{ data, message }` without "success").
func reply(w http.ResponseWriter, status int, kv ...any) error {
	return httpx.JSON(w, status, object(kv...))
}

// rateHeaders sets getRateLimitHeaders for key: limit and remaining always
// against the default 100, reset in epoch milliseconds.
func (h *handler) rateHeaders(w http.ResponseWriter, r *http.Request, key string) error {
	win, err := h.svc.limiter.Peek(r.Context(), key)
	if err != nil {
		return err
	}
	remaining, reset := domain.DefaultRateLimit, h.svc.now().Add(domain.RateWindow)
	if win != nil {
		remaining, reset = max(0, domain.DefaultRateLimit-win.Count), win.ResetAt
	}
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(domain.DefaultRateLimit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.UnixMilli(), 10))
	return nil
}
