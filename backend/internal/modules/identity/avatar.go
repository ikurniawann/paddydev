package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/identity/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// Foto profil akun sendiri (EPIC-087 R2/R4).

// avatarBucket is a staff bucket: any signed-in dashboard account reads it.
const avatarBucket = "profile-photos"

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
	old, found, err := s.repo.SetAvatar(ctx, user.ID, url)
	if err != nil {
		return err
	}
	if !found {
		return httpx.Unauthorized("Not authenticated")
	}
	if err := s.repo.AuditAvatar(ctx, user, old, url); err != nil {
		return err
	}
	if s.photos != nil {
		if err := s.photos.SetForUser(ctx, user.ID, url, old); err != nil {
			s.warn(ctx, "salin foto profil ke karyawan gagal", err)
		}
	}
	if old != nil && strings.HasPrefix(*old, "/api/files/"+avatarBucket+"/") {
		if err := s.files.Delete(avatarBucket, *old); err != nil {
			s.warn(ctx, "hapus foto profil lama gagal", err)
		}
	}
	return nil
}

// ── storage ──────────────────────────────────────────────────────────────

// SetAvatar writes avatar_url and returns the previous value; found is false
// without a configuration.users row.
func (p *Postgres) SetAvatar(ctx context.Context, userID string, url *string) (*string, bool, error) {
	var old *string
	err := p.db.QueryRow(ctx, `UPDATE configuration.users u SET avatar_url = $2, updated_at = now()
		FROM (SELECT avatar_url FROM configuration.users WHERE id = $1 FOR UPDATE) prev
		WHERE u.id = $1 RETURNING prev.avatar_url`, userID, url).Scan(&old)
	if database.IsNoRows(err) {
		return nil, false, nil
	}
	return old, err == nil, err
}

// AuditAvatar writes one account.avatar row (before/after URL).
func (p *Postgres) AuditAvatar(ctx context.Context, user *auth.User, before, after *string) error {
	name := user.FullName
	if name == "" {
		name = user.Email
	}
	id := user.ID
	return audit.Write(ctx, p.db, audit.Entry{ActorID: user.ID, ActorName: &name, Action: "account.avatar",
		Entity: "configuration.users", EntityID: &id, EntityLabel: &name,
		Before: map[string]any{"avatar_url": before}, After: map[string]any{"avatar_url": after}})
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
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
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
