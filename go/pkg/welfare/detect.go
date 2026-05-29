// SPDX-Licence-Identifier: EUPL-1.2

package welfare

import "dappco.re/lthn/desktop/pkg/contentshield"

// DetectResult is the welfare read for one user message.
type DetectResult struct {
	Triggered          bool    `json:"triggered"`
	SlurMatch          bool    `json:"slur_match"`
	SlurTerm           string  `json:"slur_term,omitempty"`
	AngerScore         float64 `json:"anger_score"`
	SustainedHostility float64 `json:"sustained_hostility"`
}

// Detect scores the user's latest message and the session's prior hostility,
// and reports whether the welfare-mediation trigger fires (RFC.welfare §1):
//
//	SlurMatch  OR  (AngerScore > AngerThreshold  AND  SustainedHostility > SustainedThreshold)
//
// A slur fires on a single message; anger needs a sustained pattern (so a
// one-off heated line doesn't yank a peer into mediation). Only the latest user
// message is scored — model output is never scored here, on principle.
func (s *Service) Detect(sessionID, text string) DetectResult {
	hit, term := s.matcher.Match(text)

	anger := 0.0
	if h := contentshield.Hostility(text); h != nil {
		anger = h.Score
	}

	sustained := s.recordAndScore(sessionID, anger)

	res := DetectResult{
		SlurMatch:          hit,
		SlurTerm:           term,
		AngerScore:         anger,
		SustainedHostility: sustained,
	}
	res.Triggered = hit || (anger > s.cfg.AngerThreshold && sustained > s.cfg.SustainedThreshold)
	return res
}

// recordAndScore returns SustainedHostility from the session's PRIOR turns
// (the build-up before this message), then folds the current anger into the
// rolling window. Computing sustained on prior-only means a first heated
// message has sustained 0 — anger needs a pattern to gate, not one outburst.
func (s *Service) recordAndScore(sessionID string, anger float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	prior := s.history[sessionID]
	sustained := 0.0
	if len(prior) > 0 {
		over := 0
		for _, a := range prior {
			if a >= s.cfg.AngerFloor {
				over++
			}
		}
		sustained = float64(over) / float64(len(prior))
	}

	window := append(prior, anger)
	if len(window) > s.cfg.SustainedWindow {
		window = window[len(window)-s.cfg.SustainedWindow:]
	}
	s.history[sessionID] = window
	return sustained
}

// Reset clears a session's hostility history — e.g. after the model resolves a
// mediation, or when a conversation is cleared.
func (s *Service) Reset(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.history, sessionID)
}
