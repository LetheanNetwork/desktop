// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"sync"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type countingMedium struct {
	coreio.Medium
	mu    sync.Mutex
	reads int
}

func (medium *countingMedium) Read(path string) (string, error) {
	medium.mu.Lock()
	medium.reads++
	medium.mu.Unlock()
	return medium.Medium.Read(path)
}

func (medium *countingMedium) readCount() int {
	medium.mu.Lock()
	defer medium.mu.Unlock()
	return medium.reads
}

func TestMediumCredentialProvider_GoodCachesAndInvalidates(t *core.T) {
	base := coreio.NewMemoryMedium()
	const token = "lthn-mlx_abcdefghijklmnopqrstuvwx"
	core.RequireNoError(t, base.Write(adminTokenRelativePath, token+"\n"))
	medium := &countingMedium{Medium: base}
	provider := NewMediumCredentialProvider(medium)

	first := provider.Credential()
	second := provider.Credential()

	core.RequireTrue(t, first.OK, first.Error())
	core.RequireTrue(t, second.OK, second.Error())
	core.AssertEqual(t, token, first.String())
	core.AssertEqual(t, token, second.String())
	core.AssertEqual(t, 1, medium.readCount())

	provider.Invalidate()
	third := provider.Credential()
	core.RequireTrue(t, third.OK, third.Error())
	core.AssertEqual(t, token, third.String())
	core.AssertEqual(t, 2, medium.readCount())

	provider.Clear()
	fourth := provider.Credential()
	core.RequireTrue(t, fourth.OK, fourth.Error())
	core.AssertEqual(t, token, fourth.String())
	core.AssertEqual(t, 3, medium.readCount())
}

func TestMediumCredentialProvider_BadRejectsMalformedTokens(t *core.T) {
	tests := []string{
		"",
		"plain-token",
		"lthn-mlx_short",
		"lthn-mlx_has whitespace",
		"lthn-mlx_has\nnewline",
		"lthn-mlx_" + string([]byte{0x7f}) + "control",
	}
	for index, token := range tests {
		t.Run(core.Sprintf("case-%d", index), func(t *core.T) {
			medium := coreio.NewMemoryMedium()
			core.RequireNoError(t, medium.Write(adminTokenRelativePath, token))
			result := NewMediumCredentialProvider(medium).Credential()
			core.AssertFalse(t, result.OK)
			core.AssertEqual(t, CredentialUnavailable, CredentialErrorCodeOf(result))
		})
	}

	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(
		adminTokenRelativePath,
		"lthn-mlx_"+string(make([]byte, maxAdminTokenBytes)),
	))
	oversized := NewMediumCredentialProvider(medium).Credential()
	core.AssertFalse(t, oversized.OK)
	core.AssertEqual(t, CredentialUnavailable, CredentialErrorCodeOf(oversized))
}

func TestMediumCredentialProvider_UglyFailsClosedWithoutMediumOrFile(t *core.T) {
	var missing *MediumCredentialProvider
	nilProvider := missing.Credential()
	core.AssertFalse(t, nilProvider.OK)
	core.AssertEqual(t, CredentialUnavailable, CredentialErrorCodeOf(nilProvider))

	noFile := NewMediumCredentialProvider(coreio.NewMemoryMedium()).Credential()
	core.AssertFalse(t, noFile.OK)
	core.AssertEqual(t, CredentialUnavailable, CredentialErrorCodeOf(noFile))
}
