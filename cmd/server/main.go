package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mxcd/go-config/config"
	"github.com/mxcd/handoff/internal/server"
	"github.com/mxcd/handoff/internal/store"
	"github.com/mxcd/handoff/internal/util"
	"github.com/rs/zerolog/log"
)

func main() {
	if err := util.InitConfig(); err != nil {
		log.Panic().Err(err).Msg("error initializing config")
	}
	config.Print()

	if err := util.InitLogger(); err != nil {
		log.Panic().Err(err).Msg("error initializing logger")
	}

	sessionStore := store.NewStore()

	s, err := server.NewServer(&server.ServerOptions{
		DevMode: config.Get().Bool("DEV"),
		Port:    config.Get().Int("PORT"),
		Store:   sessionStore,
	})
	if err != nil {
		log.Panic().Err(err).Msg("error initializing server")
	}

	if err := s.RegisterRoutes(); err != nil {
		log.Panic().Err(err).Msg("error registering routes")
	}

	// Start server in a goroutine so we can listen for shutdown signals
	go func() {
		if err := s.Run(); err != nil {
			log.Panic().Err(err).Msg("error running server")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.Shutdown(ctx)
	log.Info().Msg("server shutdown complete")
}
