package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"gv-api/internal/config"
	"gv-api/internal/printers"
)

func runPrusaLogin(cfg *config.Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := printers.LoginConnect(ctx, printers.Printer{
		ConnectUser:      cfg.PrusaConnectUser,
		ConnectPassword:  cfg.PrusaConnectPassword,
		ConnectTokenFile: cfg.PrusaConnectTokenFile,
	}, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Login failed:", err)
		return 1
	}
	fmt.Println("Logged in. The running API will pick the session up on its next request.")
	return 0
}
