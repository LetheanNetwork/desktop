// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/lemma"
)

const (
	defaultLEMBaseURL  = "http://127.0.0.1:36911"
	maxResponseBytes   = 1 << 20
	defaultHTTPTimeout = 5 * time.Second
)

// ClientErrorCode classifies trusted LEM protocol failures without forwarding
// an upstream body, URL, token, or path.
type ClientErrorCode string

const (
	ClientUnavailable      ClientErrorCode = "runtime_not_ready"
	ClientUnauthorised     ClientErrorCode = "admin_unauthorised"
	ClientResponseInvalid  ClientErrorCode = "response_invalid"
	ClientResponseTooLarge ClientErrorCode = "response_too_large"
	ClientRequestTimeout   ClientErrorCode = "request_timeout"
)

// ClientFailure is retained inside trusted Go. Its message is bounded and
// contains no upstream response content.
type ClientFailure struct {
	Code    ClientErrorCode
	Message string
	Cause   error
}

func (failure *ClientFailure) Error() string {
	if failure == nil {
		return ""
	}
	return failure.Message
}

func (failure *ClientFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// ClientErrorCodeOf returns a stable code from a failed client result.
func ClientErrorCodeOf(result core.Result) ClientErrorCode {
	if result.OK {
		return ""
	}
	var failure *ClientFailure
	if core.As(result.Err(), &failure) && failure != nil {
		return failure.Code
	}
	return ""
}

// Health is the bounded LEM /v1/health response used for readiness.
type Health struct {
	Status  string   `json:"status"`
	Runtime string   `json:"runtime"`
	Models  []string `json:"models"`
	Time    int64    `json:"time"`
}

// Machine is the narrow machine-confirmation response needed by reload.
type Machine struct {
	Hash    string `json:"hash"`
	Runtime string `json:"runtime"`
}

// Status is the trusted-Go runtime status. ModelPath never crosses Wails.
type Status struct {
	ModelPath    string                  `json:"model_path"`
	Runtime      string                  `json:"runtime"`
	LoadedAtUnix int64                   `json:"loaded_at_unix"`
	Config       lemma.ServeStatusConfig `json:"config"`
}

// ReloadCommand is trusted execution data. NativePath must come from the
// Medium-backed catalogue and is never renderer-controlled.
type ReloadCommand struct {
	ConfirmMachine string
	NativePath     string
}

// Client is the narrow protocol consumed by Service.
type Client interface {
	Health(core.Context) core.Result
	Status(core.Context, string) core.Result
	Machine(core.Context, string) core.Result
	Reload(core.Context, string, ReloadCommand) core.Result
}

// HTTPClient is pinned to the loopback LEM endpoint in production.
type HTTPClient struct {
	baseURL string
	client  *http.Client
}

// NewClient returns the production LEM client.
func NewClient() *HTTPClient {
	return newHTTPClient(defaultLEMBaseURL, boundedHTTPClient(defaultHTTPTimeout))
}

func newHTTPClient(baseURL string, client *http.Client) *HTTPClient {
	if client == nil {
		client = boundedHTTPClient(defaultHTTPTimeout)
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func boundedHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Health checks readiness without requiring the admin credential.
func (client *HTTPClient) Health(ctx core.Context) core.Result {
	var health Health
	result := client.doJSON(ctx, http.MethodGet, "/v1/health", "", nil, &health)
	if !result.OK {
		return result
	}
	if health.Status == "" ||
		len(health.Status) > 32 ||
		len(health.Runtime) > 128 ||
		len(health.Models) > 512 {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM health response is invalid.",
			nil,
		)
	}
	for _, model := range health.Models {
		if invalidProtocolText(model, 4_096) {
			return clientFailure(
				ClientResponseInvalid,
				"The LEM health response is invalid.",
				nil,
			)
		}
	}
	return core.Ok(health)
}

// Status returns the trusted runtime status for snapshot reconciliation.
func (client *HTTPClient) Status(ctx core.Context, credential string) core.Result {
	var status Status
	result := client.doJSON(
		ctx,
		http.MethodGet,
		"/v1/admin/serve/status",
		credential,
		nil,
		&status,
	)
	if !result.OK {
		return result
	}
	if invalidProtocolText(status.Runtime, 128) ||
		(status.ModelPath != "" && invalidProtocolText(status.ModelPath, 4_096)) ||
		status.LoadedAtUnix < 0 ||
		status.Config.ContextLength < 0 {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM status response is invalid.",
			nil,
		)
	}
	return core.Ok(status)
}

// Machine returns the confirmation hash required for a reload.
func (client *HTTPClient) Machine(ctx core.Context, credential string) core.Result {
	var machine Machine
	result := client.doJSON(
		ctx,
		http.MethodGet,
		"/v1/admin/machine",
		credential,
		nil,
		&machine,
	)
	if !result.OK {
		return result
	}
	if invalidProtocolText(machine.Hash, 256) ||
		invalidProtocolText(machine.Runtime, 128) {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM machine response is invalid.",
			nil,
		)
	}
	return core.Ok(machine)
}

// Reload requests one Medium-resolved local model path.
func (client *HTTPClient) Reload(
	ctx core.Context,
	credential string,
	command ReloadCommand,
) core.Result {
	if invalidProtocolText(command.ConfirmMachine, 256) ||
		invalidProtocolText(command.NativePath, 4_096) {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM reload request is invalid.",
			nil,
		)
	}
	return client.doJSON(
		ctx,
		http.MethodPost,
		"/v1/admin/serve/reload",
		credential,
		lemma.ReloadRequest{
			ConfirmMachine: command.ConfirmMachine,
			ModelPath:      command.NativePath,
		},
		nil,
	)
}

func (client *HTTPClient) doJSON(
	ctx core.Context,
	method string,
	requestPath string,
	credential string,
	body any,
	target any,
) core.Result {
	if client == nil || client.client == nil || client.baseURL == "" {
		return clientFailure(
			ClientUnavailable,
			"The LEM runtime is unavailable.",
			nil,
		)
	}
	if ctx == nil {
		ctx = core.Background()
	}
	var requestBody io.Reader
	if body != nil {
		encoded := core.JSONMarshal(body)
		if !encoded.OK {
			return clientFailure(
				ClientResponseInvalid,
				"The LEM request could not be encoded.",
				encoded.Err(),
			)
		}
		requestBody = bytes.NewReader(encoded.Bytes())
	}
	request, err := http.NewRequestWithContext(
		ctx,
		method,
		client.baseURL+requestPath,
		requestBody,
	)
	if err != nil {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM request is invalid.",
			err,
		)
	}
	request.Header.Set("Accept", "application/json")
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.client.Do(request)
	if err != nil {
		return clientTransportFailure(err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return clientFailure(
			ClientUnauthorised,
			"LEM rejected the local admin credential.",
			nil,
		)
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return clientFailure(
			ClientResponseInvalid,
			"LEM returned an unexpected response.",
			nil,
		)
	}
	if target == nil {
		return core.Ok(nil)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") {
		return clientFailure(
			ClientResponseInvalid,
			"LEM returned a non-JSON response.",
			nil,
		)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM response could not be read.",
			err,
		)
	}
	if len(responseBody) > maxResponseBytes {
		return clientFailure(
			ClientResponseTooLarge,
			"The LEM response exceeded the allowed size.",
			nil,
		)
	}
	decoded := core.JSONUnmarshal(responseBody, target)
	if !decoded.OK {
		return clientFailure(
			ClientResponseInvalid,
			"The LEM response is invalid.",
			decoded.Err(),
		)
	}
	return core.Ok(target)
}

func clientTransportFailure(err error) core.Result {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) ||
		(core.As(err, &networkError) && networkError.Timeout()) {
		return clientFailure(
			ClientRequestTimeout,
			"The LEM request timed out.",
			err,
		)
	}
	return clientFailure(
		ClientUnavailable,
		"The LEM runtime is unavailable.",
		err,
	)
}

func invalidProtocolText(value string, maximum int) bool {
	if value == "" || len(value) > maximum {
		return true
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func clientFailure(
	code ClientErrorCode,
	message string,
	cause error,
) core.Result {
	return core.Fail(&ClientFailure{
		Code:    code,
		Message: message,
		Cause:   cause,
	})
}
