package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pylotlight/adoMonitor/internal/config"
	"github.com/pylotlight/adoMonitor/internal/display"
	"github.com/pylotlight/adoMonitor/internal/monitor"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ado, err := monitor.NewADOMonitor(cfg.PAT, cfg.Org, cfg.Project)
	if err != nil {
		log.Fatalf("Failed to create ADO monitor: %v", err)
	}

	ado.AddPipelines(cfg.Pipelines)
	ado.SetUpdateInterval(5 * time.Second)

	tui := display.NewTUIDisplay(ado)
	ado.Register(tui)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ado.MonitorPipelines(ctx)

	if err := tui.Run(); err != nil {
		log.Printf("Error running TUI: %v", err)
	}

	// Wait for interrupt signal to gracefully shutdown the application
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	cancel()
	log.Println("Shutting down gracefully...")
	time.Sleep(time.Second) // Give some time for goroutines to finish
}

/*
- Aim: TUI to monitor azdo pipe status/progress and manage stage approvals
- Connect to ADO with PAT /w x perms
- paste 1+ pipes to get display of current progress,stages,approvals.
- Use proper function structure
*/
