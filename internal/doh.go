package internal

import (
	"context"
	"doh/internal/cache"
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
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", *startup.Port),
	}

	cache.Cache = cache.NewDNSCache()

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
	log.Printf("DoH proxy started on http://localhost:%d, upstream server: %s, cache ttl: %d\n", *startup.Port, *startup.Upstream, *startup.Ttl)

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
