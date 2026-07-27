// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	core "dappco.re/go"
)

func (service *Service) ensureSampler() {
	if service == nil || service.options.DisableSampler {
		return
	}
	service.mu.Lock()
	if service.shuttingDown || service.samplerCancel != nil {
		service.mu.Unlock()
		return
	}
	ctx, cancel := core.WithCancel(core.Background())
	service.samplerCancel = cancel
	interval := service.options.SampleInterval
	service.work.Add(1)
	service.mu.Unlock()

	go func() {
		defer service.work.Done()
		for {
			select {
			case <-service.options.After(interval):
				result := service.Snapshot()
				if !result.OK {
					continue
				}
				snapshot := result.Value.(Snapshot)
				if !snapshot.Desired ||
					snapshot.State == StateStopped ||
					snapshot.State == StateUnavailable {
					service.stopSampler()
					return
				}
				service.fireEvent("sample", snapshot.State)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (service *Service) stopSampler() {
	if service == nil {
		return
	}
	service.mu.Lock()
	if service.samplerCancel != nil {
		service.samplerCancel()
		service.samplerCancel = nil
	}
	service.mu.Unlock()
}
