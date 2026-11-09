package domain

import "testing"

// EPIC-088 R2/R6: kapan lampiran perlu diverifikasi HR, dan path unggah ulang.

func TestNeedsAttachmentReview(t *testing.T) {
	cases := []struct {
		url      string
		verifier bool
		want     bool
	}{
		{"leave-attachments/a/x.png", false, true},
		{"  ", false, false},
		{"", false, false},
		{"leave-attachments/a/x.png", true, false},
	}
	for _, c := range cases {
		if got := NeedsAttachmentReview(c.url, c.verifier); got != c.want {
			t.Errorf("NeedsAttachmentReview(%q, %v) = %v, want %v", c.url, c.verifier, got, c.want)
		}
	}
}

func TestLeaveAttachmentPathOK(t *testing.T) {
	cases := []struct {
		path, emp string
		want      bool
	}{
		{"leave-attachments/E1/x.png", "E1", true},
		{"leave-attachments/E2/x.png", "E1", false},
		{"leave-attachments/E1/../E2/x.png", "E1", false},
		{"leave-attachments/E1/", "E1", false},
		{"", "E1", false},
		{"leave-attachments/E1/x.png", "", false},
	}
	for _, c := range cases {
		if got := LeaveAttachmentPathOK(c.path, c.emp); got != c.want {
			t.Errorf("LeaveAttachmentPathOK(%q, %q) = %v, want %v", c.path, c.emp, got, c.want)
		}
	}
	if AttachmentAwaiting != "awaiting" || AttachmentReturned != "returned" || AttachmentVerified != "verified" {
		t.Error("konstanta attachment_review berubah")
	}
}
