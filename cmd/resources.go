package caddycmd

import (
	"log/slog"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

func setResourceLimits(logger *zap.Logger) func() {
	// 2. Configure the maximum number of CPUs to use to match the Linux container quota (if any)
	// See https://pkg.go.dev/runtime#GOMAXPROCS
	undo, err := maxprocs.Set(maxprocs.Logger(logger.Sugar().Infof))
	if err != nil {
		logger.Warn("failed to set GOMAXPROCS", zap.Error(err))
	}

	// 3. Configure the maximum memory to use to match the Linux container quota (if any) or system memory
	// See https://pkg.go.dev/runtime/debug#SetMemoryLimit
	_, _ = memlimit.SetGoMemLimitWithOpts(
		memlimit.WithLogger(
			slog.New(zapslog.NewHandler(logger.Core())),
		),
		memlimit.WithProvider(
			memlimit.ApplyFallback(
				memlimit.FromCgroup,
				memlimit.FromSystem,
			),
		),
	)

	// Adaptive Memory Management: "Black / Grey / White"
	//
	// Goals:
	// - White Mode (High Load): Maximize throughput (Standard GC).
	// - Grey Mode (Moderate Load): Maximize efficiency (Aggressive GC).
	// - Black Mode (Idle): Minimize RSS (Force Scavenge).
	//
	// Strategy: Monitor allocation rate (TotalAlloc delta) every interval.

	const (
		ModeBlack = "black" // Idle: Scavenge aggressively
		ModeGrey  = "grey"  // Normal: Efficient GC
		ModeWhite = "white" // Busy: Performance GC
	)

	debug.SetGCPercent(50) // Default to Grey (Efficiency)
	currentMode := ModeGrey
	logger.Info("adaptive memory management started", zap.String("initial_mode", currentMode))

	stopMonitor := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		var lastStats runtime.MemStats
		runtime.ReadMemStats(&lastStats)

		for {
			select {
			case <-ticker.C:
				var currentStats runtime.MemStats
				runtime.ReadMemStats(&currentStats)

				// Calculate allocation rate (bytes per second approx)
				// We actually care about "activity", so TotalAlloc delta is a good proxy.
				allocDelta := currentStats.TotalAlloc - lastStats.TotalAlloc
				// Average allocs per second over the last minute
				allocRate := allocDelta / 60

				var newMode string
				// Thresholds (Heuristic - tune as needed)
				// > 10 MB/sec -> White (High Load)
				// < 100 KB/sec -> Black (Idle)
				// Else -> Grey (Moderate)
				if allocRate > 10*1024*1024 {
					newMode = ModeWhite
				} else if allocRate < 100*1024 {
					newMode = ModeBlack
				} else {
					newMode = ModeGrey
				}

				if newMode != currentMode {
					logger.Info("switching memory mode",
						zap.String("from", currentMode),
						zap.String("to", newMode),
						zap.Uint64("alloc_rate_bytes_sec", allocRate),
					)
					currentMode = newMode

					switch newMode {
					case ModeWhite:
						debug.SetGCPercent(100) // Standard Go default
					case ModeGrey:
						debug.SetGCPercent(50) // More aggressive
					case ModeBlack:
						debug.SetGCPercent(50) // Keep aggressive GC
						debug.FreeOSMemory()   // AND Force release to OS
					}
				} else if currentMode == ModeBlack {
					// In Black mode, keep scavenging if still idle
					debug.FreeOSMemory()
				}

				lastStats = currentStats

			case <-stopMonitor:
				return
			}
		}
	}()

	return func() {
		close(stopMonitor)
		if undo != nil {
			undo()
		}
	}
}
