package recruitment

import (
	"net/http"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
)

// Recruitment deletes (EPIC-061, Q-RA-1): "delete" on the leaf, a reason,
// an audit row.
var (
	delCandidate  = hrisdel.Spec{Menu: iam.LeafKandidat, Entity: "recruitment.candidates", Table: "recruitment.candidates", IDParam: "id"}
	delCV         = hrisdel.Spec{Menu: iam.LeafKandidat, Entity: "recruitment.candidate_cv", Table: "recruitment.candidates", IDParam: "id"}
	delQuestion   = hrisdel.Spec{Menu: iam.LeafPsikotes, Entity: "recruitment.psikotes_questions", Table: "recruitment.psikotes_questions", IDParam: "id"}
	delJobOpening = hrisdel.Spec{Menu: iam.LeafJobPortal, Entity: "recruitment.job_openings", Table: "recruitment.job_openings", IDParam: "id"}
)

// del wraps next in hrisdel.Guard on this module's database.
func (h *handler) del(spec hrisdel.Spec, next http.Handler) http.Handler {
	return hrisdel.Guard(h.guard, h.svc.db, spec, next)
}
