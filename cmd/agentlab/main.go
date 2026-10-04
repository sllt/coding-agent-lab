package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"path/filepath"

	"github.com/sllt/agentlab/internal/agent/fixture"
	"github.com/sllt/agentlab/internal/app"
	"github.com/sllt/agentlab/internal/bootstrap"
	"github.com/sllt/agentlab/internal/doctor"
	"github.com/sllt/agentlab/internal/httpapi"
	"github.com/sllt/agentlab/internal/runner"
	"github.com/sllt/agentlab/internal/store/sqlite"
	"github.com/sllt/agentlab/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "serve" {
		addr := "127.0.0.1:43117"
		if len(args) >= 2 && args[0] == "serve" {
			addr = args[1]
		}
		return serve(addr)
	}
	switch args[0] {
	case "version":
		fmt.Printf("%s %s\npi %s\n", version.Product, version.Version, version.PiCommit)
		return nil
	case "fake-agent":
		os.Exit(fixture.Run())
		return nil
	case "runner":
		return runner.Serve(context.Background(), os.Stdin, os.Stdout)
	case "doctor":
		adapter := "fixture"
		if len(args) > 1 {
			adapter = args[1]
		}
		report := doctor.Static(context.Background(), adapter, "", "", false)
		fmt.Printf("%s static=%t verified=%t model_call=%s network=%s blockers=%v\n", report.Adapter, report.StaticPassed, report.Verified, report.ModelCall, report.Network, report.Blockers)
		if !report.StaticPassed {
			return errors.New("doctor 未通过")
		}
		return nil
	default:
		return errors.New("用法: agentlab serve [127.0.0.1:43117] | runner | doctor [adapter] | version")
	}
}

func serve(addr string) error {
	data := os.Getenv("AGENTLAB_DATA")
	if data == "" {
		data = ".agentlab"
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	store, err := sqlite.Open(filepath.Join(data, "lab.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	svc := &app.Service{Store: store, DataDir: data, GlobalLimit: 1}
	api := httpapi.New(svc)
	rt, err := bootstrap.Build(bootstrap.Config{
		Addr:     addr,
		Register: api.Register,
		Wrap:     api.Wrap,
		Worker:   svc.Loop,
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rt.Start(ctx); err != nil {
		return err
	}
	fmt.Printf("agentlab 正在监听 http://%s\n", rt.Addr)
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10_000_000_000)
	defer cancel()
	return rt.Stop(shutdown)
}
