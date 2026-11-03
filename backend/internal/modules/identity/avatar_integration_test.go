package identity_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"nuhabit/backend/internal/modules/identity"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-087 R2/R4: unggah dan hapus foto profil akun sendiri.

type photoCall struct{ user, newURL, oldURL string }

type recordingPhotos struct {
	mu    sync.Mutex
	calls []photoCall
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (p *recordingPhotos) SetForUser(_ context.Context, userID string, newURL, oldURL *string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, photoCall{userID, deref(newURL), deref(oldURL)})
	return nil
}

func pngDataURL(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{200, 100, 50, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestAvatarUploadAndClear(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STORAGE_DIR", dir)
	deps := testutil.Deps(t, nil)
	photos := &recordingPhotos{}
	mux := testutil.Mux(identity.New(deps, noStalls{}, photos))
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", FullName: "Paddy HR"})
	t.Cleanup(func() {
		_, _ = deps.DB.Exec(context.Background(), `DELETE FROM audit.audit_log WHERE action = 'account.avatar' AND entity_id = $1`, staff.UserID)
	})
	post := func(body any) (int, map[string]any) {
		rec, out := testutil.Do(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/auth/me/avatar", body), staff))
		return rec.Code, out
	}
	onDisk := func(url string) bool {
		_, err := os.Stat(filepath.Join(dir, "uploads", strings.TrimPrefix(url, "/api/files/")))
		return err == nil
	}

	rec, _ := testutil.Do(t, mux, testutil.Request("POST", "/api/auth/me/avatar", map[string]any{"image": pngDataURL(t, 10, 10)}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tanpa sesi = %d", rec.Code)
	}

	var g bytes.Buffer
	_ = gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil)
	code, out := post(map[string]any{"image": "data:image/gif;base64," + base64.StdEncoding.EncodeToString(g.Bytes())})
	if code != 400 || out["error"] != "Foto harus berupa gambar JPG, PNG, atau WebP" {
		t.Fatalf("gif = %d %v", code, out)
	}

	code, out = post(map[string]any{"image": pngDataURL(t, 800, 600)})
	first, _ := out["data"].(map[string]any)["avatar_url"].(string)
	if code != 200 || !strings.HasPrefix(first, "/api/files/profile-photos/"+staff.UserID+"/") || !onDisk(first) {
		t.Fatalf("unggah = %d %v", code, out)
	}
	_, me := testutil.Do(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/auth/me", nil), staff))
	if me["data"].(map[string]any)["avatar_url"] != first {
		t.Fatalf("/me = %v", me)
	}

	// Review Focus 4: ganti foto → file lama hilang.
	_, out = post(map[string]any{"image": pngDataURL(t, 300, 300)})
	second := out["data"].(map[string]any)["avatar_url"].(string)
	if second == first || onDisk(first) || !onDisk(second) {
		t.Fatalf("ganti: first on disk=%v second=%s", onDisk(first), second)
	}

	rec, out = testutil.Do(t, mux, testutil.AsStaff(testutil.Request("DELETE", "/api/auth/me/avatar", nil), staff))
	if rec.Code != 200 || out["data"].(map[string]any)["avatar_url"] != nil || onDisk(second) {
		t.Fatalf("hapus = %d %v", rec.Code, out)
	}
	_, me = testutil.Do(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/auth/me", nil), staff))
	if me["data"].(map[string]any)["avatar_url"] != nil {
		t.Fatalf("/me setelah hapus = %v", me)
	}

	if n := count(t, deps, `SELECT count(*)::int FROM audit.audit_log WHERE action = 'account.avatar' AND entity_id = $1`, staff.UserID); n != 3 {
		t.Fatalf("audit = %d, want 3", n)
	}
	want := []photoCall{{staff.UserID, first, ""}, {staff.UserID, second, first}, {staff.UserID, "", second}}
	if len(photos.calls) != 3 || photos.calls[0] != want[0] || photos.calls[1] != want[1] || photos.calls[2] != want[2] {
		t.Fatalf("salinan karyawan = %+v", photos.calls)
	}
}
