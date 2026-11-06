package domain

import "strings"

// Verifikasi lampiran cuti/izin oleh HR (EPIC-088, ES-11). Selama menunggu HR
// pengajuan tetap pending; tahapnya di hris.leaves.attachment_review.
const (
	AttachmentAwaiting = "awaiting" // atasan sudah setuju, menunggu HR
	AttachmentReturned = "returned" // HR mengembalikan, karyawan unggah ulang
	AttachmentVerified = "verified" // HR memverifikasi (pengajuan approved)
)

// NeedsAttachmentReview: a leave with an attachment approved by someone who
// is not an attachment verifier waits for HR (R2).
func NeedsAttachmentReview(attachmentURL string, approverVerifies bool) bool {
	return strings.TrimSpace(attachmentURL) != "" && !approverVerifies
}

// LeaveAttachmentPathOK: a re-uploaded attachment must be a file in the
// requester's own leave-attachments folder (R6).
func LeaveAttachmentPathOK(path, employeeID string) bool {
	if employeeID == "" || strings.Contains(path, "..") {
		return false
	}
	name, ok := strings.CutPrefix(path, "leave-attachments/"+employeeID+"/")
	return ok && name != "" && !strings.Contains(name, "/")
}
