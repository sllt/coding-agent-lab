package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sllt/agentlab/internal/bootstrap"
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
	if args[0] == "version" {
		fmt.Printf("%s %s\npi %s\n", version.Product, version.Version, version.PiCommit)
		return nil
	}
	return errors.New("用法: agentlab serve [127.0.0.1:43117] | agentlab version")
}

func serve(addr string) error {
	rt, err := bootstrap.Build(bootstrap.Config{Addr: addr})
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
