package auth

import (
	"strings"

	"github.com/hashicorp/boundary/internal/auth/oidc"
	"github.com/hashicorp/boundary/internal/auth/password"
)

type SubType int

const (
	UnknownSubtype SubType = iota
	PasswordSubtype
	OidcSubtype
)

func (t SubType) String() string {
	switch t {
	case PasswordSubtype:
		return "password"
	case OidcSubtype:
		return "oidc"
	}
	return "unknown"
}

// SubtypeFromType converts a string to a SubType.
// returns UnknownSubtype if no SubType with that name is found.
func SubtypeFromType(t string) SubType {
	switch {
	case strings.EqualFold(strings.TrimSpace(t), PasswordSubtype.String()):
		return PasswordSubtype
	case strings.EqualFold(strings.TrimSpace(t), OidcSubtype.String()):
		return OidcSubtype
	}
	return UnknownSubtype
}

// SubtypeFromId takes any public id in the auth subsystem and uses the prefix to determine
// what subtype the id is for.
// Returns UnknownSubtype if no SubType with this id's prefix is found.
func SubtypeFromId(id string) SubType {
	switch {
	case strings.HasPrefix(strings.TrimSpace(id), password.AuthMethodPrefix),
		strings.HasPrefix(strings.TrimSpace(id), password.AccountPrefix):
		return PasswordSubtype
	case strings.HasPrefix(strings.TrimSpace(id), oidc.AuthMethodPrefix),
		strings.HasPrefix(strings.TrimSpace(id), oidc.AccountPrefix):
		return OidcSubtype
	}
	return UnknownSubtype
}
