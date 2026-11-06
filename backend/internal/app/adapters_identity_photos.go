package app

import (
	"context"

	"nuhabit/backend/internal/platform/database"
)

// identityEmployeePhotos copies an account's profile photo (EPIC-087 R4) to
// the linked employee, only over an empty photo_url or the previous profile
// photo, so a photo HR entered stays.
type identityEmployeePhotos struct{ db database.Querier }

func (a identityEmployeePhotos) SetForUser(ctx context.Context, userID string, newURL, oldURL *string) error {
	_, err := a.db.Exec(ctx, `UPDATE hris.employees SET photo_url = $2, updated_at = now()
		WHERE user_id = $1 AND (photo_url IS NULL OR photo_url = '' OR photo_url = $3)`, userID, newURL, oldURL)
	return err
}
