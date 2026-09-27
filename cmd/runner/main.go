package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"reforge/internal/auth"
	"reforge/internal/privateconnector"
	"reforge/internal/runnerclient"
	"reforge/internal/sandbox"
)

func main() {
	if err := run(); err != nil {
		slog.Error("runner stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: reforge-runner enroll|connector|run|builtin|builtin-init [flags]")
	}
	mode := os.Args[1]
	if mode == "image-digest" {
		return imageDigest(os.Args[2:])
	}
	if mode == "verify-runtime" {
		return verifyRuntime(os.Args[2:])
	}
	if mode == "builtin-init" {
		return builtinInit(os.Args[2:])
	}
	if mode == "builtin" {
		return runBuiltin(os.Args[2:])
	}
	if mode != "enroll" && mode != "connector" && mode != "run" {
		return errors.New("unknown runner command")
	}
	flags := flag.NewFlagSet("reforge-runner", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "controller HTTPS origin")
	credentials := flags.String("credentials", "", "private runner credential file")
	name := flags.String("name", "reforge-runner", "runner name")
	tokenFile := flags.String("token-file", "", "one-use enrollment token file")
	caFile := flags.String("ca-file", "", "controller CA certificate PEM")
	development := flags.Bool("development", false, "allow explicit loopback development endpoints")
	runtimeConfig := flags.String("runtime-config", "", "strict sandbox runtime configuration JSON")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *credentials == "" {
		return errors.New("controller endpoint and credential file are required")
	}
	var runtime sandbox.RuntimeConfig
	if mode == "run" {
		if *runtimeConfig == "" {
			return errors.New("run requires --runtime-config")
		}
		loaded, err := loadRuntimeConfig(*runtimeConfig)
		if err != nil {
			return err
		}
		runtime = loaded
		if runtime.Development && !*development {
			return errors.New("runtime development mode requires --development")
		}
	}
	absolute, err := filepath.Abs(*credentials)
	if err != nil {
		return err
	}
	var ca []byte
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if *caFile != "" {
		ca, err = os.ReadFile(*caFile)
		if err != nil {
			return errors.New("controller CA file unavailable")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(ca) {
			return errors.New("invalid controller CA certificate")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	defer transport.CloseIdleConnections()
	client, err := runnerclient.New(runnerclient.Config{Endpoint: strings.TrimRight(*endpoint, "/"), Name: *name, Development: *development, CredentialFile: absolute, Client: &http.Client{Transport: transport}})
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if mode == "enroll" {
		if *tokenFile == "" {
			return errors.New("enrollment requires --token-file with a one-use token")
		}
		file, err := os.OpenFile(*tokenFile, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return errors.New("enrollment token file unavailable")
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("enrollment token requires a private regular file")
		}
		raw, err := io.ReadAll(io.LimitReader(file, 1025))
		if err != nil || len(raw) > 1024 {
			return errors.New("invalid enrollment token file")
		}
		defer clear(raw)
		if err = client.Enroll(ctx, strings.TrimSpace(string(raw))); err != nil {
			return err
		}
		identity, _ := client.Supervisor()
		fmt.Printf("Enrolled runner %s; credential saved to %s\n", identity.ID, absolute)
		return nil
	}
	if err = client.Load(); err != nil {
		return err
	}
	if mode == "run" {
		return runRepair(ctx, client, strings.TrimRight(*endpoint, "/"), ca, *development, runtime)
	}
	var private *privateconnector.Client
	var activeToken string
	defer func() {
		if private != nil {
			private.Close()
		}
	}()
	for ctx.Err() == nil {
		identity, token := client.Supervisor()
		if time.Until(identity.CredentialExpiresAt) < time.Hour {
			if err = client.Rotate(ctx); err != nil {
				return err
			}
			identity, token = client.Supervisor()
		}
		if token != activeToken {
			if private != nil {
				private.Close()
			}
			private, err = privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: strings.TrimRight(*endpoint, "/"), Credential: token, Target: privateconnector.Target{OrgID: identity.OrgID, RunnerID: identity.ID}, Development: *development, CAPEM: ca})
			if err != nil {
				return err
			}
			activeToken = token
		}
		err = private.RunOnce(ctx)
		if errors.Is(err, auth.ErrUnauthenticated) {
			_, latest := client.Supervisor()
			if latest != token {
				continue
			}
			return errors.New("runner credential revoked or expired; enroll again")
		}
		if err != nil && ctx.Err() == nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	return nil
}

func imageDigest(args []string) error {
	flags := flag.NewFlagSet("image-digest", flag.ContinueOnError)
	imagePath := flags.String("path", "", "toolchain image directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*imagePath) {
		return errors.New("image-digest requires an absolute --path")
	}
	info, err := os.Lstat(*imagePath)
	if err != nil || !info.IsDir() || info.Mode()&0022 != 0 {
		return errors.New("image path must be a private regular directory")
	}
	digest, err := sandbox.ImageDigest(*imagePath)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, digest)
	return err
}

func verifyRuntime(args []string) error {
	flags := flag.NewFlagSet("verify-runtime", flag.ContinueOnError)
	path := flags.String("runtime-config", "", "strict sandbox runtime configuration JSON")
	development := flags.Bool("development", false, "allow explicit development isolation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected runtime verification argument")
	}
	config, err := loadRuntimeConfig(*path)
	if err != nil {
		return err
	}
	if config.Development && !*development {
		return errors.New("runtime development mode requires --development")
	}
	if config.Backend == "kubernetes" {
		return runnerclient.ValidateRuntimeConfig(config)
	}
	config.Fetch = func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return sandbox.Snapshot{}, sandbox.ErrBoundary
	}
	runtime, err := sandbox.NewRuntime(config)
	if err != nil {
		return err
	}
	return runtime.Close()
}
func loadRuntimeConfig(name string) (sandbox.RuntimeConfig, error) {
	if !filepath.IsAbs(name) {
		return sandbox.RuntimeConfig{}, errors.New("runtime config path must be absolute")
	}
	file, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return sandbox.RuntimeConfig{}, errors.New("runtime config unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return sandbox.RuntimeConfig{}, errors.New("runtime config must be a private regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return sandbox.RuntimeConfig{}, errors.New("runtime config invalid")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var config sandbox.RuntimeConfig
	if err := decoder.Decode(&config); err != nil || decoder.Decode(new(any)) != io.EOF {
		return sandbox.RuntimeConfig{}, errors.New("runtime config invalid")
	}
	config.Fetch = nil
	if err := runnerclient.ValidateRuntimeConfig(config); err != nil {
		return sandbox.RuntimeConfig{}, errors.New("runtime config invalid")
	}
	return config, nil
}

func runRepair(ctx context.Context, client *runnerclient.Client, endpoint string, ca []byte, development bool, config sandbox.RuntimeConfig) error {
	runctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 2)
	processor, closeProcessor := runnerclient.RepairProcessorWithCloser(config)
	defer closeProcessor()
	go func() { errs <- client.Run(runctx, processor) }()
	go func() { errs <- runPrivate(runctx, client, endpoint, ca, development) }()
	first := <-errs
	cancel()
	second := <-errs
	if ctx.Err() != nil {
		return nil
	}
	if errors.Is(first, context.Canceled) {
		first = nil
	}
	if errors.Is(second, context.Canceled) {
		second = nil
	}
	return errors.Join(first, second)
}

func runPrivate(ctx context.Context, client *runnerclient.Client, endpoint string, ca []byte, development bool) error {
	var private *privateconnector.Client
	var active string
	defer func() {
		if private != nil {
			private.Close()
		}
	}()
	for ctx.Err() == nil {
		identity, token := client.Supervisor()
		if token != active {
			if private != nil {
				private.Close()
			}
			var err error
			private, err = privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: endpoint, Credential: token, Target: privateconnector.Target{OrgID: identity.OrgID, RunnerID: identity.ID}, Development: development, CAPEM: ca})
			if err != nil {
				return err
			}
			active = token
		}
		err := private.RunOnce(ctx)
		if errors.Is(err, auth.ErrUnauthenticated) {
			_, latest := client.Supervisor()
			if latest != token {
				continue
			}
			return errors.New("runner credential revoked or expired; enroll again")
		}
		if err != nil && ctx.Err() == nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	return ctx.Err()
}
