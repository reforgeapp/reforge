package integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/release"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

const liveCheckoutApplication = `const http = require('node:http')
const fs = require('node:fs')
const version = 'one'
const checkout = basket => basket.length > 0 ? 'order confirmed' : 'empty basket'
exports.checkout = checkout
if (require.main === module) {
  const server = http.createServer((request, response) => {
    if (version === 'two' && fs.existsSync('outage')) { response.writeHead(503); response.end('checkout unavailable'); return }
    const count = Number(new URL(request.url, 'http://localhost').searchParams.get('items'))
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify({outcome: checkout(Array.from({length: count})), version}))
  })
  server.listen(0, '127.0.0.1', () => process.stdout.write(String(server.address().port) + '\n'))
}
`
const liveCheckoutTest = `const test = require('node:test')
const assert = require('node:assert/strict')
const {checkout} = require('./app.cjs')
test('single item checkout confirms order', () => assert.equal(checkout(['item']), 'order confirmed'))
test('several items checkout confirms order', () => assert.equal(checkout(['one', 'two']), 'order confirmed'))
test('empty basket does not place order', () => assert.equal(checkout([]), 'empty basket'))
`
const liveCheckoutWorkflow = "name: Checkout checks\non: [push, pull_request]\njobs:\n  checks:\n    runs-on: local-qualified\n    steps:\n      - run: node --check app.cjs\n      - run: node --test --test-reporter=tap --test-isolation=none app.test.cjs\n"

type liveCheckoutProcess struct {
	root string
	url  string
	stop func()
}

func startLiveCheckout(t *testing.T, artifact []byte) *liveCheckoutProcess {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.cjs"), artifact, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "app.cjs"))
	command.Dir = root
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err = command.Start(); err != nil {
		t.Fatal("start actual checkout application", err)
	}
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); _ = command.Wait() }) }
	t.Cleanup(stop)
	port := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			port <- scanner.Text()
		}
	}()
	select {
	case value := <-port:
		number, err := strconv.Atoi(value)
		if err != nil || number < 1 || number > 65535 {
			t.Fatal("application did not report a valid local port")
		}
		return &liveCheckoutProcess{root: root, url: "http://127.0.0.1:" + value, stop: stop}
	case <-time.After(5 * time.Second):
		stop()
		t.Fatal("actual checkout application did not start")
	}
	return nil
}

func (p *liveCheckoutProcess) smoke() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	for _, count := range []string{"1", "3"} {
		response, err := client.Get(p.url + "/checkout?items=" + count)
		if err != nil {
			return false
		}
		var body struct{ Outcome string }
		err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != 200 || body.Outcome != "order confirmed" {
			return false
		}
	}
	return true
}

func buildLiveCheckout(t *testing.T, artifact []byte, sha string) (string, *deployment.BuildValidation) {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string][]byte{"app.cjs": artifact, "app.test.cjs": []byte(liveCheckoutTest)} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) sandbox.CommandResult {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "node", args...)
		command.Dir = root
		started := time.Now()
		output, err := command.CombinedOutput()
		if err != nil || len(output) > 1<<20 {
			t.Fatalf("actual repository checks failed: %v %s", err, output)
		}
		return sandbox.CommandResult{ExitCode: 0, Output: output, Duration: time.Since(started)}
	}
	build := run("--check", "app.cjs")
	tests := run("--test", "--test-reporter=tap", "--test-isolation=none", "app.test.cjs")
	validation, err := release.CollectValidation(sha, "tap", build, tests)
	if err != nil {
		t.Fatal("actual CI evidence collection", err)
	}
	sum := sha256.Sum256(artifact)
	return "sha256:" + hex.EncodeToString(sum[:]), validation
}
