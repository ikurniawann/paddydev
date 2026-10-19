// Package insights is the reporting context: the recruitment dashboard
// (/api/dashboard/*), recruitment analytics (/api/analytics/*), the
// executive dashboard and the Do assistant (/api/ai/assistant).
package insights

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/extract"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/ocr"
	"nuhabit/backend/internal/platform/storage"
)

// Name is the MODULES key.
const Name = "insights"

// Service runs the reports and the assistant.
type Service struct {
	db    database.DB
	auth  *auth.Service
	ports Ports
	now   func() time.Time
	log   *slog.Logger
	llm   *openAI
	// ocr reads images and scanned PDFs sent to the assistant.
	ocr extract.OCR
	// memoryDir receives the per-user assistant markdown log.
	memoryDir string

	execMu    sync.Mutex
	execCache *executiveCached
}

type mod struct{ s *Service }

func (mod) Name() string { return Name }

// New builds the module; ports come from internal/app.
func New(deps module.Deps, ports Ports) module.Module {
	return mod{s: newService(deps, deps.DB, ports)}
}

func newService(deps module.Deps, db database.DB, ports Ports) *Service {
	return &Service{
		db: db, auth: deps.Auth, ports: ports, now: deps.Now, log: deps.Log,
		llm:       newOpenAI(os.Getenv, http.DefaultClient),
		ocr:       ocr.New(),
		memoryDir: assistantMemoryDir(os.Getenv),
	}
}

// assistantMemoryDir is <storage>/assistant-memory, the directory Next
// writes (process.cwd()/storage/assistant-memory), so both append to the
// same files.
func assistantMemoryDir(getenv func(string) string) string {
	return filepath.Join(storage.Dir(getenv), "assistant-memory")
}

// recruitmentReaders are the dashboard readers: the recruitment team and the
// recruitment Analytics menu (directors, hiring managers). Laporan HRIS alone
// does not open it (EPIC-057: akun Non-Rekrutmen).
var recruitmentReaders = append(append([]string{}, iam.HrisRecruitment...), iam.HrisAnalytics...)

func (m mod) Routes() []module.Route {
	s := m.s
	routes := []module.Route{
		{Pattern: "GET /api/dashboard/attention", Handler: httpx.Handle(s.attention)},
		{Pattern: "GET /api/dashboard/funnel", Handler: httpx.Handle(s.funnel)},
		{Pattern: "GET /api/dashboard/sources", Handler: httpx.Handle(s.sources)},
		{Pattern: "GET /api/dashboard/stats", Handler: httpx.Handle(s.stats)},
		{Pattern: "GET /api/dashboard/weekly", Handler: httpx.Handle(s.weekly)},
		{Pattern: "GET /api/analytics/brands", Handler: httpx.Handle(s.analyticsBrands)},
		{Pattern: "GET /api/analytics/overview", Handler: httpx.Handle(s.analyticsOverview)},
		{Pattern: "GET /api/analytics/sources", Handler: httpx.Handle(s.analyticsSources)},
		{Pattern: "GET /api/ai/assistant", Handler: http.HandlerFunc(s.assistantHistory)},
		{Pattern: "DELETE /api/ai/assistant", Handler: http.HandlerFunc(s.assistantDelete)},
		{Pattern: "PATCH /api/ai/assistant", Handler: http.HandlerFunc(s.assistantRename)},
		{Pattern: "POST /api/ai/assistant", Handler: http.HandlerFunc(s.assistantAsk)},
		{Pattern: "POST /api/ai/assistant/actions", Handler: http.HandlerFunc(s.assistantAction)},
		{Pattern: "POST /api/ai/assistant/attachment", Handler: http.HandlerFunc(s.assistantAttachment)},
	}
	if s.ports.Overview != nil {
		routes = append(routes, module.Route{Pattern: "GET /api/dashboard/executive", Handler: httpx.Handle(s.executive)})
	}
	return append(routes, s.salesTargetRoutes()...)
}

// parallel runs fns concurrently on a pool (the TS Promise.all) and in
// order on a transaction, which cannot run statements concurrently.
func (s *Service) parallel(fns ...func()) {
	if _, pool := s.db.(*pgxpool.Pool); !pool {
		for _, fn := range fns {
			fn()
		}
		return
	}
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Go(fn)
	}
	wg.Wait()
}
