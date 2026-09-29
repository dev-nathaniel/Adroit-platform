package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/deployments/migrations"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
)

// ANSI Color Codes
const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
)

// ServiceDef defines an actual microservice binary to build and supervise
type ServiceDef struct {
	Name  string
	Pkg   string
	Bin   string
	Port  int
	Color string
}

func main() {
	services := []ServiceDef{
		{
			Name:  "Auth Service",
			Pkg:   "./cmd/auth-service",
			Bin:   "auth-service",
			Port:  8081,
			Color: colorGreen,
		},
		{
			Name:  "User Service",
			Pkg:   "./cmd/user-service",
			Bin:   "user-service",
			Port:  8082,
			Color: colorYellow,
		},
		{
			Name:  "API Gateway",
			Pkg:   "./cmd/api-gateway",
			Bin:   "api-gateway",
			Port:  8080,
			Color: colorCyan,
		},
	}

	binDir := "bin"
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Printf("Failed to create %s directory: %v\n", binDir, err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("%s%s==========================================================================%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s       ADROIT PLATFORM — MICROSERVICES PROCESS SUPERVISOR               %s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s==========================================================================%s\n", colorBold, colorCyan, colorReset)
	for _, s := range services {
		fmt.Printf("   * %s%-16s%s http://localhost:%d  (%s)\n", s.Color, s.Name, colorReset, s.Port, s.Pkg)
	}
	fmt.Printf("%s==========================================================================%s\n", colorCyan, colorReset)

	// Pre-flight: Check database and run pending migrations if reachable
	dbCfg := database.LoadConfig()
	dbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	dbConn, err := database.Connect(dbCtx, dbCfg)
	cancel()
	if err == nil {
		_ = dbConn.Close()
		fmt.Print("   -> PostgreSQL detected. Checking database migrations ... ")
		if err := database.MigrateUp(dbCfg.DSN(), migrations.FS); err != nil {
			fmt.Printf("%sWARNING%s (%v)\n", colorYellow, colorReset, err)
		} else {
			v, _, _ := database.MigrateVersion(dbCfg.DSN(), migrations.FS)
			fmt.Printf("%sDONE%s (v%d)\n", colorGreen, colorReset, v)
		}
	} else {
		fmt.Printf("   -> PostgreSQL not detected; services will fallback to in-memory mode.\n")
	}

	fmt.Println("   Compiling services...")

	// 1. Build all services
	var builtBinaries []string
	for _, s := range services {
		binName := s.Bin
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(binDir, binName)

		fmt.Printf("   -> Building %s (%s) ... ", s.Name, s.Pkg)
		cmd := exec.Command("go", "build", "-o", binPath, s.Pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("%sFAILED%s\n", colorRed, colorReset)
			fmt.Printf("Build failed for %s: %v\n", s.Name, err)
			os.Exit(1)
		}
		fmt.Printf("%sDONE%s\n", colorGreen, colorReset)
		builtBinaries = append(builtBinaries, binPath)
	}

	fmt.Println()
	fmt.Println("   Starting all services...")
	fmt.Println("   Press Ctrl+C to stop all services simultaneously.")
	fmt.Println("--------------------------------------------------------------------------")

	// 2. Start each process
	var runningCmds []*exec.Cmd
	var mu sync.Mutex

	for i, s := range services {
		binPath := builtBinaries[i]
		cmd := exec.Command(binPath)

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Printf("Failed to get stdout for %s: %v\n", s.Name, err)
			continue
		}

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			fmt.Printf("Failed to get stderr for %s: %v\n", s.Name, err)
			continue
		}

		go pipeOutput(stdoutPipe, s.Name, s.Color)
		go pipeOutput(stderrPipe, s.Name, colorRed)

		if err := cmd.Start(); err != nil {
			fmt.Printf("Failed to start %s: %v\n", s.Name, err)
			continue
		}

		mu.Lock()
		runningCmds = append(runningCmds, cmd)
		mu.Unlock()
	}

	// 3. Wait for termination signal (Ctrl+C / SIGINT / SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println()
	fmt.Printf("%sShutting down all Adroit microservices...%s\n", colorYellow, colorReset)

	mu.Lock()
	for _, cmd := range runningCmds {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	mu.Unlock()

	fmt.Printf("%sAll microservices stopped successfully.%s\n", colorGreen, colorReset)
}

func pipeOutput(r io.Reader, prefix, color string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Printf("%s[%s]%s %s\n", color, prefix, colorReset, scanner.Text())
	}
}
