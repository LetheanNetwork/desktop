// SPDX-Licence-Identifier: EUPL-1.2

//go:build ios || android

package main

import core "dappco.re/go"

func mobileFirstMap(data any) map[string]any {
	switch value := data.(type) {
	case map[string]any:
		return value
	case []any:
		if len(value) > 0 {
			if payload, ok := value[0].(map[string]any); ok {
				return payload
			}
		}
	}
	return nil
}

func mobileEventBool(data any, key string, fallback bool) bool {
	if direct, ok := data.(bool); ok {
		return direct
	}
	if payload := mobileFirstMap(data); payload != nil {
		if value, ok := payload[key].(bool); ok {
			return value
		}
	}
	return fallback
}

func mobileEventString(data any, key string) string {
	if direct, ok := data.(string); ok {
		return direct
	}
	if payload := mobileFirstMap(data); payload != nil {
		if value, ok := payload[key].(string); ok {
			return value
		}
	}
	return ""
}

func mobileEventFloat(data any, key string, fallback float64) float64 {
	if direct, ok := data.(float64); ok {
		return direct
	}
	if payload := mobileFirstMap(data); payload != nil {
		if value, ok := payload[key].(float64); ok {
			return value
		}
	}
	return fallback
}

func mobilePayloadJSON(data any) string {
	if payload := mobileFirstMap(data); payload != nil {
		return core.JSONMarshalString(payload)
	}
	return "{}"
}

func mobileJSONMap(raw string) map[string]any {
	result := map[string]any{}
	if raw != "" {
		if decoded := core.JSONUnmarshalString(raw, &result); !decoded.OK {
			return map[string]any{}
		}
	}
	return result
}
