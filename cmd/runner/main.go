package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
)

func main() {
	if err := run(); err != nil {
		slog.Error("runner stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: reforge-runner enroll|connector --endpoint URL --credentials FILE [--token-file FILE]")
	}
	mode := os.Args[1]
	if mode != "enroll" && mode != "connector" {
		return errors.New("unknown runner command")
	}
	flags := flag.NewFlagSet("reforge-runner", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "controller HTTPS origin")
	credentials := flags.String("credentials", "", "private runner credential file")
	name := flags.String("name", "reforge-runner", "runner name")
	tokenFile := flags.String("token-file", "", "one-use enrollment token file")
	caFile := flags.String("ca-file", "", "controller CA certificate PEM")
	development := flags.Bool("development", false, "allow explicit loopback development endpoints")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *credentials == "" {
		return errors.New("controller endpoint and credential file are required")
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
