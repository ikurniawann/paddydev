package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/identity/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// Foto profil akun sendiri (EPIC-087 R2/R4).

// avatarBucket is a staff bucket: any signed-in dashboard account reads it.
const avatarBucket = "profile-photos"

// maxAvatarBody is the JSON body limit: a 5 MB picture as base64 plus slack.
const maxAvatarBody = 8 << 20

// EmployeePhotos mirrors the account photo into the linked employee's
// photo_url, never over a photo HR set (the adapter lives in internal/app).
type EmployeePhotos interface {
	SetForUser(ctx context.Context, userID string, newURL, oldURL *string) error
}

// AvatarResult is the body of both avatar routes.
type AvatarResult struct {
	AvatarURL *string `json:"avatar_url"`
}

// SetAvatar stores the processed picture and makes it the account's photo;
// the previous file is removed once the row points at the new one.
func (s *Service) SetAvatar(ctx context.Context, user *auth.User, dataURL string) (*AvatarResult, error) {
	raw, err := domain.DecodeDataURL(dataURL)
	if err == nil {
		raw, err = domain.SquareAvatar(raw)
	}
	if errors.Is(err, domain.ErrAvatarFormat) || errors.Is(err, domain.ErrAvatarTooLarge) {
		return nil, httpx.BadRequest(err.Error())
	}
	if err != nil {
		return nil, err
	}
	url, err := s.files.Upload(avatarBucket, user.ID, raw, "image/jpeg", "avatar.jpg")
	if err != nil {
		return nil, err
	}
	if err := s.switchAvatar(ctx, user, &url); err != nil {
		_ = s.files.Delete(avatarBucket, url)
		return nil, err
	}
	return &AvatarResult{AvatarURL: &url}, nil
}

// ClearAvatar goes back to the initial avatar.
func (s *Service) ClearAvatar(ctx context.Context, user *auth.User) (*AvatarResult, error) {
	if err := s.switchAvatar(ctx, user, nil); err != nil {
		return nil, err
	}
	return &AvatarResult{}, nil
}

func (s *Service) switchAvatar(ctx context.Context, user *auth.User, url *string) error {
	// Baris + audit dalam satu transaksi: gagal → tidak ada yang berubah.
	old, found, err := s.repo.SetAvatar(ctx, user, url)
	if err != nil {
		return err
	}
	if !found {
		return httpx.Unauthorized("Not authenticated")
	}
	if old == nil && url == nil {
		return nil // tidak ada foto yang dihapus
	}
	// Sesudah commit: jangan ikut batal bila klien menutup permintaan.
	after := context.WithoutCancel(ctx)
	if s.photos != nil {
		if err := s.photos.SetForUser(after, user.ID, url, old); err != nil {
			// File lama mungkin masih dirujuk hris.employees.photo_url: biarkan.
			s.warn(after, "salin foto profil ke karyawan gagal", err)
			return nil
		}
	}
	if old != nil && strings.HasPrefix(*old, "/api/files/"+avatarBucket+"/") {
		if err := s.files.Delete(avatarBucket, *old); err != nil {
			s.warn(after, "hapus foto profil lama gagal", err)
		}
	}
	return nil
}

// ── storage ──────────────────────────────────────────────────────────────

// SetAvatar writes avatar_url and its account.avatar audit row in one
// transaction and returns the previous value; found is false without a
// configuration.users row. Removing an absent photo writes no audit row.
func (p *Postgres) SetAvatar(ctx context.Context, user *auth.User, url *string) (*string, bool, error) {
	var old *string
	found := false
	run := func(q database.Querier) error {
		err := q.QueryRow(ctx, `UPDATE configuration.users u SET avatar_url = $2, updated_at = now()
			FROM (SELECT avatar_url FROM configuration.users WHERE id = $1 FOR UPDATE) prev
			WHERE u.id = $1 RETURNING prev.avatar_url`, user.ID, url).Scan(&old)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		if old == nil && url == nil {
			return nil
		}
		name := user.FullName
		if name == "" {
			name = user.Email
		}
		id := user.ID
		return audit.Write(ctx, q, audit.Entry{ActorID: user.ID, ActorName: &name, Action: "account.avatar",
			Entity: "configuration.users", EntityID: &id, EntityLabel: &name,
			Before: map[string]any{"avatar_url": old}, After: map[string]any{"avatar_url": url}})
	}
	if tb, ok := p.db.(database.TxBeginner); ok {
		err := database.WithTx(ctx, tb, func(tx pgx.Tx) error { return run(tx) })
		return old, found, err
	}
	return old, found, run(p.db) // sudah di dalam transaksi pemanggil
}

// ── HTTP ─────────────────────────────────────────────────────────────────

func (h handlers) setAvatar(w http.ResponseWriter, r *http.Request) error {
	user, err := h.svc.sessions.RequireUser(r)
	if err != nil {
		return httpx.Unauthorized("Not authenticated")
	}
	var body struct {
		Image string `json:"image"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, maxAvatarBody+1))
	if len(raw) > maxAvatarBody {
		return httpx.BadRequest(domain.ErrAvatarTooLarge.Error())
	}
	_ = json.Unmarshal(raw, &body)
	res, err := h.svc.SetAvatar(r.Context(), user, body.Image)
	if err != nil {
		return err
	}
	return httpx.Data(w, http.StatusOK, res)
}

func (h handlers) clearAvatar(w http.ResponseWriter, r *http.Request) error {
	user, err := h.svc.sessions.RequireUser(r)
	if err != nil {
		return httpx.Unauthorized("Not authenticated")
	}
	res, err := h.svc.ClearAvatar(r.Context(), user)
	if err != nil {
		return err
	}
	return httpx.Data(w, http.StatusOK, res)
}
