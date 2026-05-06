// hack/perf/ddgr-loadtest/main.go
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Config struct {
	Kubeconfig      string
	Namespace       string
	Count           int
	ChurnPercent    int
	ChurnInterval   time.Duration
	Duration        time.Duration
	Seed            int64
	Mode            string
	NamePrefix      string
	FillConcurrency int
}

func parseFlags() Config {
	cfg := Config{}
	flag.StringVar(&cfg.Kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)")
	flag.StringVar(&cfg.Namespace, "namespace", "ddgr-loadtest", "namespace for DDGRs")
	flag.IntVar(&cfg.Count, "count", 500, "steady-state monitor count")
	flag.IntVar(&cfg.ChurnPercent, "churn-percent", 10, "percent of monitors mutated per churn tick")
	flag.DurationVar(&cfg.ChurnInterval, "churn-interval", 2*time.Minute, "time between churn ticks")
	flag.DurationVar(&cfg.Duration, "duration", 2*time.Hour, "total test duration after fill")
	flag.Int64Var(&cfg.Seed, "seed", 1, "RNG seed (deterministic churn selection)")
	flag.StringVar(&cfg.Mode, "mode", "run", "run | cleanup")
	flag.StringVar(&cfg.NamePrefix, "name-prefix", "loadtest", "prefix for generated DDGR names")
	flag.IntVar(&cfg.FillConcurrency, "fill-concurrency", 10, "bounded create concurrency for fill phase")
	flag.Parse()
	return cfg
}

func main() {
	cfg := parseFlags()
	log.Printf("config=%+v", cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	r, err := NewRunner(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	switch cfg.Mode {
	case "run":
		err = r.Run(ctx)
	case "cleanup":
		err = r.Cleanup(ctx)
	default:
		log.Fatalf("unknown --mode=%q (must be run|cleanup)", cfg.Mode)
	}
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
