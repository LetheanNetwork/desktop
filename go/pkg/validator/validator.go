// SPDX-Licence-Identifier: EUPL-1.2

// Package validator pings remote inference endpoints to confirm
// they're reachable + speak the expected protocol. Used by the
// setup wizard to validate a `routes.X.base_url` before persisting
// it to config.
//
// Today: GET <base>/models (OpenAI-compat) and accept any 2xx as
// success. Returns the resolved status code + body excerpt so the
// CLI can surface "yes that's an OpenAI-compat endpoint serving
// these models" feedback.
//
// Usage example:
//
//	r := validator.Endpoint("http://localhost:11434/v1")
//	if r.OK { info := r.Value.(validator.EndpointInfo); _ = info }
package validator

import (
	core "dappco.re/go"
)

// EndpointInfo is the validation result for a probed endpoint.
type EndpointInfo struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	OK     bool   `json:"ok"`
	Body   string `json:"body,omitempty"`
}

// Endpoint hits <baseURL>/models and reports the status. Treats
// 2xx as OK; everything else as not-OK without returning a Result
// failure (the caller still wants the diagnostic info).
//
// Usage example:
//
//	r := validator.Endpoint("https://api.openai.com/v1")
//	if r.OK { _ = r.Value.(EndpointInfo) }
func Endpoint(baseURL string) core.Result {
	if baseURL == "" {
		return core.Fail(core.E("validator.Endpoint", "base url is required", nil))
	}
	url := core.Concat(core.TrimRight(baseURL, "/"), "/models")
	resp := core.HTTPGet(url)
	if !resp.OK {
		return core.Ok(EndpointInfo{URL: url, Status: 0, OK: false})
	}
	r := resp.Value.(*core.Response)
	defer r.Body.Close()

	br := core.ReadAll(core.LimitReader(r.Body, 4096))
	if !br.OK {
		return core.Fail(core.E("validator.Endpoint", "read body failed", br.Value.(error)))
	}
	return core.Ok(EndpointInfo{
		URL:    url,
		Status: r.StatusCode,
		OK:     r.StatusCode >= 200 && r.StatusCode < 300,
		Body:   br.Value.(string),
	})
}
