// file: internal/updater/register.go
// version: 1.0.0
// guid: 8c9d0a1b-2c3d-4e5f-6a7b-8c9d0a1b2c3d

package updater

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// SchedulerStarterAdapter wraps *Scheduler to implement the Starter/Stopper interfaces.
// Scheduler.Start() and Stop() take no context parameter, so we adapt them here.
type SchedulerStarterAdapter struct {
	scheduler *Scheduler
}

// Start implements the serviceregistry.Starter interface.
func (a *SchedulerStarterAdapter) Start(ctx context.Context) error {
	a.scheduler.Start()
	return nil
}

// Stop implements the serviceregistry.Stopper interface.
func (a *SchedulerStarterAdapter) Stop(ctx context.Context) error {
	a.scheduler.Stop()
	return nil
}

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "updatescheduler",
		Needs: []string{},
		Build: func(c *serviceregistry.Container) (any, error) {
			// Create the Updater with the default "dev" version.
			// The actual version is set at build time via ldflags in main.go
			// and propagated to the server package's appVersion var.
			// For now, we use "dev" as the fallback; the existing server.updater
			// path (which has the real version from appVersion) remains unchanged.
			updaterInst := NewUpdater("dev")

			// Create the Scheduler with a closure that captures config.AppConfig
			scheduler := NewScheduler(updaterInst, func() SchedulerConfig {
				return SchedulerConfig{
					Enabled:     config.AppConfig.AutoUpdateEnabled,
					Channel:     config.AppConfig.AutoUpdateChannel,
					CheckMins:   config.AppConfig.AutoUpdateCheckMinutes,
					WindowStart: config.AppConfig.AutoUpdateWindowStart,
					WindowEnd:   config.AppConfig.AutoUpdateWindowEnd,
				}
			})

			// Wrap in adapter to implement Starter/Stopper interfaces
			return &SchedulerStarterAdapter{scheduler: scheduler}, nil
		},
	})
}
