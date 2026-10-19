package domain

// Columns of the Data Karyawan file (Spec 2026-10-19 §3.2): export and
// import share them so an exported file can be edited and imported back.

type EmployeeColumn struct {
	Key        string
	Header     string
	Required   bool
	Kind       string
	Width      float64
	ExportOnly bool
}

var EmployeeColumns = []EmployeeColumn{
	{Key: "nip", Header: "NIP", Kind: "text", Width: 16},
	{Key: "full_name", Header: "Nama Lengkap", Required: true, Kind: "text", Width: 28},
	{Key: "email", Header: "Email", Required: true, Kind: "text", Width: 28},
	{Key: "phone", Header: "No. HP", Kind: "text", Width: 16},
	{Key: "join_date", Header: "Tanggal Masuk", Required: true, Kind: "date", Width: 14},
	{Key: "employment_status", Header: "Status Kepegawaian", Required: true, Kind: "list", Width: 20},
	{Key: "schedule_type", Header: "Tipe Jadwal", Required: true, Kind: "list", Width: 12},
	{Key: "branch", Header: "Cabang Utama", Required: true, Kind: "list", Width: 22},
	{Key: "department", Header: "Departemen", Kind: "list", Width: 22},
	{Key: "position", Header: "Jabatan", Kind: "list", Width: 24},
	{Key: "manager_nip", Header: "NIP Atasan", Kind: "text", Width: 16},
	{Key: "ktp", Header: "NIK (KTP)", Kind: "text", Width: 20},
	{Key: "npwp", Header: "NPWP", Kind: "text", Width: 20},
	{Key: "birth_date", Header: "Tanggal Lahir", Kind: "date", Width: 14},
	{Key: "gender", Header: "Jenis Kelamin", Kind: "list", Width: 14},
	{Key: "marital_status", Header: "Status Pernikahan", Kind: "list", Width: 18},
	{Key: "address", Header: "Alamat", Kind: "text", Width: 36},
	{Key: "city", Header: "Kota", Kind: "text", Width: 16},
	{Key: "province", Header: "Provinsi", Kind: "text", Width: 16},
	{Key: "postal_code", Header: "Kode Pos", Kind: "text", Width: 10},
	{Key: "bank_name", Header: "Nama Bank", Kind: "text", Width: 14},
	{Key: "bank_account", Header: "No. Rekening", Kind: "text", Width: 18},
	{Key: "bpjs_tk", Header: "No. BPJS Ketenagakerjaan", Kind: "text", Width: 22},
	{Key: "bpjs_kesehatan", Header: "No. BPJS Kesehatan", Kind: "text", Width: 20},
	{Key: "emergency_contact_name", Header: "Kontak Darurat (Nama)", Kind: "text", Width: 22},
	{Key: "emergency_contact_phone", Header: "Kontak Darurat (No. HP)", Kind: "text", Width: 18},
	{Key: "emergency_contact_relationship", Header: "Kontak Darurat (Hubungan)", Kind: "text", Width: 18},
	{Key: "manager_name", Header: "Nama Atasan", Kind: "text", Width: 24, ExportOnly: true},
	{Key: "active", Header: "Status Aktif", Kind: "text", Width: 12, ExportOnly: true},
	{Key: "end_date", Header: "Tanggal Keluar", Kind: "date", Width: 14, ExportOnly: true},
	{Key: "updated_at", Header: "Terakhir Diubah", Kind: "date", Width: 14, ExportOnly: true},
}

func EmployeeHeaderKey(header string) (string, bool) {
	h := NormalizeHeader(header)
	for _, c := range EmployeeColumns {
		if NormalizeHeader(c.Header) == h {
			return c.Key, true
		}
	}
	return "", false
}
