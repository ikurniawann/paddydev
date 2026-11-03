package main

import (
	"bytes"
	"strings"
	"testing"
)

// Gerbang sebelum menyentuh database (EPIC-086 R4).
func TestRunRefusesBadArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-mode", "apply", "-file", "x.xlsx"}, "-sha wajib untuk apply"},
		{[]string{"-mode", "preview"}, "-file wajib"},
		{[]string{"-mode", "hapus"}, "mode harus audit, template, preview, atau apply"},
		{[]string{"-mode", "template"}, "-out wajib"},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		if code := run(c.args, &out, &errOut); code == 0 || !strings.Contains(errOut.String(), c.want) {
			t.Errorf("%v: exit %d, stderr %q, want %q", c.args, code, errOut.String(), c.want)
		}
	}
}
