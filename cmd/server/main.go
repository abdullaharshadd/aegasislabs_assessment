package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	client "migrated-app/internal"
)

func main() {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: client.BuildRouter(),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	log.Info().Msg("server started on :8080")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() {
			for {
				resp, err := http.Get("http://localhost:8080/health")
				if err == nil {
					resp.Body.Close()
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			close(ch)
		}()
		return ch
	}():
		<-quit
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}

	_ = os.Stderr
}