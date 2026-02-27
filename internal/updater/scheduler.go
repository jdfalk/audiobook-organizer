// file: internal/updater/scheduler.go
// version: 1.0.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

package updater

import (
	"log"
	"time"
)

// SchedulerConfig holds the runtime config for the update scheduler.
type SchedulerConfig struct {
	Enabled     bool
	Channel     string
	CheckMins   int
	WindowStart int // hour 0-23
	WindowEnd   int // hour 0-23
}

// Scheduler periodically checks for updates and applies them within a window.
type Scheduler struct {
	updater *Updater
	ticker  *time.Ticker
	stopCh  chan struct{}
	config  func() SchedulerConfig
}

// NewScheduler creates a scheduler that reads config dynamically via the getter.
func NewScheduler(u *Updater, configGetter func() SchedulerConfig) *Scheduler {
	return &Scheduler{
		updater: u,
		stopCh:  make(chan struct{}),
		config:  configGetter,
	}
}

// Start begins the periodic check loop in a goroutine.
func (s *Scheduler) Start() {
	cfg := s.config()
	if !cfg.Enabled {
		log.Printf("[INFO] Auto-update scheduler disabled")
		return
	}

	interval := time.Duration(cfg.CheckMins) * time.Minute
	if interval < time.Minute {
		interval = time.Minute
	}
	s.ticker = time.NewTicker(interval)

	log.Printf("[INFO] Auto-update scheduler started: checking every %d minutes on %q channel, window %02d:00-%02d:00",
		cfg.CheckMins, cfg.Channel, cfg.WindowStart, cfg.WindowEnd)

	go s.loop()
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
	log.Printf("[INFO] Auto-update scheduler stopped")
}

func (s *Scheduler) loop() {
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	cfg := s.config()
	if !cfg.Enabled {
		return
	}

	info, err := s.updater.CheckForUpdate(cfg.Channel)
	if err != nil {
		log.Printf("[WARN] Auto-update check failed: %v", err)
		return
	}

	if !info.UpdateAvailable {
		log.Printf("[DEBUG] Auto-update check: no update available (current=%s, latest=%s)",
			info.CurrentVersion, info.LatestVersion)
		return
	}

	log.Printf("[INFO] Update available: %s -> %s (%s)", info.CurrentVersion, info.LatestVersion, info.Channel)

	// Check if current hour is within the update window
	hour := time.Now().Hour()
	if !inWindow(hour, cfg.WindowStart, cfg.WindowEnd) {
		log.Printf("[INFO] Update available but outside update window (%02d:00-%02d:00, current hour: %02d)",
			cfg.WindowStart, cfg.WindowEnd, hour)
		return
	}

	log.Printf("[INFO] Applying update within window...")
	if err := s.updater.DownloadAndReplace(info); err != nil {
		log.Printf("[ERROR] Failed to apply update: %v", err)
		return
	}

	// This will exit the process; systemd restarts with new binary
	s.updater.RestartSelf()
}

// inWindow checks if hour is within [start, end). Handles wrap-around (e.g. 23-4).
func inWindow(hour, start, end int) bool {
	if start <= end {
		return hour >= start && hour < end
	}
	// Wraps midnight: e.g. start=23, end=4 means 23,0,1,2,3
	return hour >= start || hour < end
}
