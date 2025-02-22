package main

import (
	"all-unifi-monitor/internal/config"
	"all-unifi-monitor/internal/store"
	"all-unifi-monitor/pkg/logger"
)

func main() {
	logger.Info().Msg("Initializing...")

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load configuration")
	}

	unifiStore := store.New(cfg)
	go unifiStore.Start()

	// Keep the main thread alive
	select {}
}
