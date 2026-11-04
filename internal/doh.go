package internal

import (
	"context"
	"doh/internal/handler"
	"doh/internal/startup"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Run() {
	// Create a new HTTP server
	// ponytail: bind to localhost by default and set timeouts to avoid
	// Slowloris / connection-exhaustion DoS and accidental public exposure.
	server := &http.Server{
		Addr:            fmt.Sprintf("%s:%d", *startup.BindAddress, *startup.Port),
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Handle incoming HTTP requests
	http.HandleFunc("/", handler.HandleRequest)

	c := make(chan os.Signal, 1)
	defer close(c)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// Start the HTTP server
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	log.Printf("DoH proxy started on http://localhost:%d, upstream server: %s, cache ttl: %d, tcp: %t\n", *startup.Port, *startup.Upstream, *startup.Ttl, *startup.Tcp)

	<-c
	log.Print("Graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Print("Server Exited")
	os.Exit(0)
}
