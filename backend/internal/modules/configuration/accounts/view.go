package accounts

import (
	"encoding/json"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/httpx"
)

// employeeView is mapEmployeeUserRow (lib/users/user-mapper.ts).
type employeeView struct {
	ID                           string          `json:"id"`
	UserID                       *string         `json:"userId"`
	FullName                     string          `json:"fullName"`
	NIP                          string          `json:"nip"`
	Email                        string          `json:"email"`
	Phone                        string          `json:"phone"`
	JoinDate                     httpx.JSTime    `json:"joinDate"`
	EndDate                      *httpx.JSTime   `json:"endDate"`
	EmploymentStatus             string          `json:"employmentStatus"`
	IsActive                     bool            `json:"isActive"`
	IsAccessApp                  bool            `json:"isAccessApp"`
	ScheduleType                 string          `json:"scheduleType"`
	DepartmentID                 *string         `json:"departmentId"`
	SectionID                    *string         `json:"sectionId"`
	JobTitleID                   *string         `json:"jobTitleId"`
	ReportingTo                  *string         `json:"reportingTo"`
	KTP                          *string         `json:"ktp"`
	NPWP                         *string         `json:"npwp"`
	BirthDate                    *httpx.JSTime   `json:"birthDate"`
	Gender                       *string         `json:"gender"`
	MaritalStatus                *string         `json:"maritalStatus"`
	Address                      *string         `json:"address"`
	City                         *string         `json:"city"`
	Province                     *string         `json:"province"`
	PostalCode                   *string         `json:"postalCode"`
	BankName                     *string         `json:"bankName"`
	BankAccount                  *string         `json:"bankAccount"`
	BPJSTK                       *string         `json:"bpjsTk"`
	BPJSKesehatan                *string         `json:"bpjsKesehatan"`
	EmergencyContactName         *string         `json:"emergencyContactName"`
	EmergencyContactPhone        *string         `json:"emergencyContactPhone"`
	EmergencyContactRelationship *string         `json:"emergencyContactRelationship"`
	PhotoURL                     *string         `json:"photoUrl"`
	Notes                        *string         `json:"notes"`
	CreatedAt                    httpx.JSTime    `json:"createdAt"`
	UpdatedAt                    *httpx.JSTime   `json:"updatedAt"`
	Department                   json.RawMessage `json:"department"`
	Section                      json.RawMessage `json:"section"`
	JobTitle                     json.RawMessage `json:"jobTitle"`
	Manager                      json.RawMessage `json:"manager"`
	AppAccount                   *appAccountView `json:"appAccount"`
}

type appAccountView struct {
	ID                  string          `json:"id"`
	Role                string          `json:"role"`
	Status              string          `json:"status"`
	BrandID             *string         `json:"brandId"`
	BrandName           *string         `json:"brandName"`
	BusinessScope       *string         `json:"businessScope"`
	HoldingID           *string         `json:"holdingId"`
	CompanyID           *string         `json:"companyId"`
	BranchID            *string         `json:"branchId"`
	HoldingName         *string         `json:"holdingName"`
	CompanyName         *string         `json:"companyName"`
	BranchName          *string         `json:"branchName"`
	LastSignInAt        *httpx.JSTime   `json:"lastSignInAt"`
	CanSwitchStall      bool            `json:"canSwitchStall"`
	CanCentralCheckout  bool            `json:"canCentralCheckout"`
	DefaultWarehouseID  *string         `json:"defaultWarehouseId"`
	ApprovalPermissions json.RawMessage `json:"approvalPermissions"`
	Warehouses          []warehouseView `json:"warehouses"`
}

type warehouseView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	BranchID string `json:"branchId"`
}

// embed renders a row_to_json value as JSON.parse + JSON.stringify would.
func embed(raw json.RawMessage) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	return kit.NormalizeJSON(raw)
}

// mapEmployee is mapEmployeeUserRow; app is nil for app_user null.
func mapEmployee(e Employee, app *appUser) (employeeView, error) {
	v := employeeView{
		ID: e.ID, UserID: e.UserID, FullName: e.FullName, NIP: e.NIP, Email: e.Email, Phone: e.Phone,
		JoinDate: httpx.JSTime(e.JoinDate), EndDate: httpx.NewJSTime(e.EndDate),
		EmploymentStatus: e.EmploymentStatus, IsActive: e.IsActive, IsAccessApp: e.IsAccessApp, ScheduleType: e.ScheduleType,
		DepartmentID: e.DepartmentID, SectionID: e.SectionID, JobTitleID: e.JobTitleID, ReportingTo: e.ReportingTo,
		KTP: e.KTP, NPWP: e.NPWP, BirthDate: httpx.NewJSTime(e.BirthDate), Gender: e.Gender,
		MaritalStatus: e.MaritalStatus, Address: e.Address, City: e.City, Province: e.Province,
		PostalCode: e.PostalCode, BankName: e.BankName, BankAccount: e.BankAccount, BPJSTK: e.BPJSTK,
		BPJSKesehatan: e.BPJSKesehatan, EmergencyContactName: e.EmergencyName,
		EmergencyContactPhone: e.EmergencyPhone, EmergencyContactRelationship: e.EmergencyRelation,
		PhotoURL: e.PhotoURL, Notes: e.Notes, CreatedAt: httpx.JSTime(e.CreatedAt), UpdatedAt: httpx.NewJSTime(e.UpdatedAt),
	}
	var err error
	for _, p := range []struct {
		dst *json.RawMessage
		raw json.RawMessage
	}{{&v.Department, e.Department}, {&v.Section, e.Section}, {&v.JobTitle, e.JobTitle}, {&v.Manager, e.Manager}} {
		if *p.dst, err = embed(p.raw); err != nil {
			return v, err
		}
	}
	if app == nil {
		return v, nil
	}
	perms := app.Permissions
	if perms == nil {
		perms = json.RawMessage(`[]`)
	}
	warehouses := make([]warehouseView, len(app.Warehouses))
	for i, w := range app.Warehouses {
		warehouses[i] = warehouseView{ID: w.WarehouseID, Name: w.Name, Code: w.Code, BranchID: w.BranchID}
	}
	v.AppAccount = &appAccountView{
		ID: app.ID, Role: app.Role, Status: app.Status, BrandID: app.BrandID,
		BusinessScope: app.BusinessScope, HoldingID: app.HoldingID, CompanyID: app.CompanyID, BranchID: app.BranchID,
		HoldingName: app.HoldingName, CompanyName: app.CompanyName, BranchName: app.BranchName,
		LastSignInAt: httpx.NewJSTime(app.LastSignInAt), CanSwitchStall: app.CanSwitchStall,
		CanCentralCheckout: app.CanCentralCheckout, DefaultWarehouseID: app.DefaultWarehouseID,
		ApprovalPermissions: perms, Warehouses: warehouses,
	}
	return v, nil
}
