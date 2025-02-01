package models

import "strings"

type SCIMUserRequest struct {
	Schemas  []string    `json:"schemas"`
	UserName string      `json:"userName"`
	Name     SCIMName    `json:"name"`
	Emails   []SCIMEmail `json:"emails"`
}

type SCIMUserResponse struct {
	Schemas  []string    `json:"schemas"`
	ID       string      `json:"id"`
	UserName string      `json:"userName"`
	Name     SCIMName    `json:"name"`
	Emails   []SCIMEmail `json:"emails"`
	Meta     SCIMMeta    `json:"meta"`
}

// SCIMPatchRequest represents a SCIM PATCH request
type SCIMPatchRequest struct {
	Schemas    []string      `json:"schemas"`
	Operations []SCIMPatchOp `json:"Operations"`
}

// SCIMPatchOp represents a single PATCH operation
type SCIMPatchOp struct {
	Op    string      `json:"op"`    // "add", "remove", or "replace"
	Path  string      `json:"path"`  // e.g. "emails", "name.givenName"
	Value interface{} `json:"value"` // New value
}

type SCIMName struct {
	GivenName  string `json:"givenName"`
	FamilyName string `json:"familyName"`
}

// Convert full name (DB) to SCIM Name struct
func (u *User) ToSCIMName() SCIMName {
	parts := strings.Fields(u.Name)
	if len(parts) > 1 {
		return SCIMName{
			GivenName:  parts[0],
			FamilyName: strings.Join(parts[1:], " "),
		}
	}
	return SCIMName{
		GivenName:  u.Name,
		FamilyName: "",
	}
}

// Convert SCIM Name struct to full name (DB format)
func (u *User) SetSCIMName(name SCIMName) {
	u.Name = strings.TrimSpace(name.GivenName + " " + name.FamilyName)
}

type SCIMEmail struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

type SCIMMeta struct {
	ResourceType string `json:"resourceType"`
	Location     string `json:"location"`
}

type SCIMErrorResponse struct {
	Schemas []string `json:"schemas"`
	Detail  string   `json:"detail"`
	Status  int      `json:"status"`
}

// SCIMGroupRequest represents a SCIM-compliant request for creating/updating groups
type SCIMGroupRequest struct {
	Schemas     []string     `json:"schemas"`
	DisplayName string       `json:"displayName"`
	Members     []SCIMMember `json:"members,omitempty"` // Optional member list
}

// SCIMGroupResponse represents the SCIM-compliant response for groups
type SCIMGroupResponse struct {
	Schemas     []string     `json:"schemas"`
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	Members     []SCIMMember `json:"members,omitempty"`
	Meta        SCIMMeta     `json:"meta"`
}

// SCIMMember represents a user inside a group
type SCIMMember struct {
	Value string `json:"value"` // User ID
	Ref   string `json:"$ref"`  // Reference URL
}

// SCIMGroupPatchRequest handles PATCH operations
type SCIMGroupPatchRequest struct {
	Schemas    []string      `json:"schemas"`
	Operations []SCIMPatchOp `json:"Operations"`
}
