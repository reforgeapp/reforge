package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/reforgeapp/reforge/internal/deployment"
	"github.com/reforgeapp/reforge/internal/gitops"
	"io"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: evidence keygen|sign --key-file PATH [--kind provenance|native-health|gitops-health --document PATH]")
	}
	flags := flag.NewFlagSet("evidence", flag.ContinueOnError)
	keyPath := flags.String("key-file", "", "Private signing key file")
	kind := flags.String("kind", "", "Evidence document kind")
	input := flags.String("document", "", "JSON document file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *keyPath == "" {
		return errors.New("explicit key file required")
	}
	switch args[0] {
	case "keygen":
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(*keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return errors.New("cannot create new private key file")
		}
		_, err = file.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		_, err = fmt.Fprintln(out, base64.StdEncoding.EncodeToString(pub))
		return err
	case "sign":
		if *input == "" {
			return errors.New("document file required")
		}
		info, err := os.Lstat(*keyPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 256 {
			return errors.New("private key must be a regular file readable only by its owner")
		}
		encoded, err := os.ReadFile(*keyPath)
		if err != nil {
			return errors.New("private key unavailable")
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return errors.New("private key must be base64 Ed25519 private key")
		}
		canonical := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
		if !bytes.Equal(canonical, key) {
			return errors.New("private key public component is inconsistent")
		}
		file, err := os.Open(*input)
		if err != nil {
			return errors.New("document unavailable")
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 {
			return errors.New("document must be at most 1 MiB")
		}
		document, err := documentFor(*kind)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(document); err != nil {
			return err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return errors.New("exactly one JSON document required")
		}
		body, err := json.Marshal(document)
		if err != nil {
			return err
		}
		signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, body))
		return json.NewEncoder(out).Encode(struct {
			Document  any    `json:"document"`
			Signature string `json:"signature"`
		}{document, signature})
	default:
		return errors.New("unsupported evidence command")
	}
}
func documentFor(kind string) (any, error) {
	switch kind {
	case "provenance":
		return &deployment.Provenance{}, nil
	case "native-health":
		return &deployment.HealthReport{}, nil
	case "gitops-health":
		return &gitops.HealthReport{}, nil
	}
	return nil, errors.New("kind must be provenance, native-health or gitops-health")
}
