// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"strings"
	"sync"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const (
	adminTokenRelativePath = "lem/admin.token"
	maxAdminTokenBytes     = 512
	minAdminTokenBytes     = 24
	adminTokenPrefix       = "lthn-mlx_"
)

// CredentialErrorCode identifies a fail-closed credential read.
type CredentialErrorCode string

const (
	// CredentialUnavailable means the registered Medium or valid LEM token is
	// unavailable.
	CredentialUnavailable CredentialErrorCode = "credential_unavailable"
)

// CredentialFailure never includes the token or provider root.
type CredentialFailure struct {
	Code    CredentialErrorCode
	Message string
	Cause   error
}

func (failure *CredentialFailure) Error() string {
	if failure == nil {
		return ""
	}
	return failure.Message
}

func (failure *CredentialFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// CredentialErrorCodeOf extracts the stable credential failure code.
func CredentialErrorCodeOf(result core.Result) CredentialErrorCode {
	if result.OK {
		return ""
	}
	var failure *CredentialFailure
	if core.As(result.Err(), &failure) && failure != nil {
		return failure.Code
	}
	return ""
}

// CredentialProvider supplies the Go-only LEM admin credential.
type CredentialProvider interface {
	Credential() core.Result
	Invalidate()
	Clear()
}

// MediumCredentialProvider reads and caches LEM's token through one registered
// application Medium.
type MediumCredentialProvider struct {
	medium coreio.Medium

	mu    sync.Mutex
	token string
}

// NewMediumCredentialProvider constructs a fail-closed token provider.
func NewMediumCredentialProvider(medium coreio.Medium) *MediumCredentialProvider {
	return &MediumCredentialProvider{medium: medium}
}

// Credential returns a bounded cached token, reading the Medium on cache miss.
func (provider *MediumCredentialProvider) Credential() core.Result {
	if provider == nil || provider.medium == nil {
		return credentialFailure(nil)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.token != "" {
		return core.Ok(provider.token)
	}
	raw, err := provider.medium.Read(adminTokenRelativePath)
	if err != nil {
		return credentialFailure(err)
	}
	token := strings.TrimSpace(raw)
	if !validAdminToken(token) {
		return credentialFailure(nil)
	}
	provider.token = token
	return core.Ok(token)
}

// Invalidate drops the cached value so one subsequent operation re-reads LEM's
// token after an unauthorised response.
func (provider *MediumCredentialProvider) Invalidate() {
	if provider == nil {
		return
	}
	provider.mu.Lock()
	provider.token = ""
	provider.mu.Unlock()
}

// Clear releases the in-memory token during Core shutdown.
func (provider *MediumCredentialProvider) Clear() {
	provider.Invalidate()
}

func validAdminToken(token string) bool {
	if len(token) < minAdminTokenBytes ||
		len(token) > maxAdminTokenBytes ||
		!strings.HasPrefix(token, adminTokenPrefix) {
		return false
	}
	for _, character := range token {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' ||
			character == '_' {
			continue
		}
		return false
	}
	return true
}

func credentialFailure(cause error) core.Result {
	return core.Fail(&CredentialFailure{
		Code:    CredentialUnavailable,
		Message: "The LEM admin credential is unavailable.",
		Cause:   cause,
	})
}
