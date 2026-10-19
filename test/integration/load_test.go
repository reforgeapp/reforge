package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/inventory"
	"github.com/reforgeapp/reforge/internal/workflow"
)

func TestControlPlaneLoadTargets(t *testing.T) {
	if os.Getenv("REFORGE_LOAD") != "1" {
		t.Skip("set REFORGE_LOAD=1 to run the control-plane load harness")
	}
	f := newInventoryFixture(t, 10000)
	ctx := context.Background()

	scanStart := time.Now()
	scan := f.sync(t)
	scanned := f.drain(t, scan.ID)
	scanDuration := time.Since(scanStart)
	if scanned.Processed != 10000 {
		t.Fatalf("scan processed %d", scanned.Processed)
	}

	importStart := time.Now()
	imported, err := f.service.StartImport(ctx, f.owner, f.org, scanned.ID, inventory.ImportInput{All: true}, scanned.Version, "load")
	if err != nil {
		t.Fatal(err)
	}
	done := f.drain(t, imported.ID)
	importDuration := time.Since(importStart)
	if done.Processed != 10000 {
		t.Fatalf("import processed %d", done.Processed)
	}

	listStart := time.Now()
	page, err := f.service.Repositories(ctx, f.owner, f.org, 200, "")
	firstPage := time.Since(listStart)
	if err != nil || len(page.Items) != 200 {
		t.Fatalf("first page: %d %v", len(page.Items), err)
	}
	filterStart := time.Now()
	filtered, err := f.service.RepositoriesFiltered(ctx, f.owner, f.org, 50, "", inventory.RepositoryFilter{Query: "REPOSITORY-9999"})
	filterDuration := time.Since(filterStart)
	if err != nil || len(filtered.Items) != 1 {
		t.Fatalf("filtered search: %d %v", len(filtered.Items), err)
	}

	cookies := make([]*http.Cookie, 0, 50)
	for i := 0; i < 50; i++ {
		_, cookie := fixtureIdentity(t, f.db)
		cookies = append(cookies, cookie)
	}
	latencies := make([]time.Duration, len(cookies))
	var wg sync.WaitGroup
	for i, cookie := range cookies {
		wg.Add(1)
		go func(index int, c *http.Cookie) {
			defer wg.Done()
			start := time.Now()
			response := identityRequest(f.server, "GET", "/api/v1/session", "", c, nil)
			latencies[index] = time.Since(start)
			if response.Code != http.StatusOK {
				t.Errorf("session %d status %d", index, response.Code)
			}
		}(i, cookie)
	}
	wg.Wait()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	wf := newWorkflowFixture(t, 100)
	claimStart := time.Now()
	for _, repo := range wf.repos {
		if _, err = wf.service.Enqueue(ctx, wf.owner, wf.org, workflow.EnqueueInput{RepositoryID: repo, Recipe: "build-repair", RecipeVersion: "1", TargetBranch: "main", IdempotencyKey: domain.NewID(), MaxAttempts: 3}, "load"); err != nil {
			t.Fatal(err)
		}
	}
	leases := 0
	for i := 0; i < 100; i++ {
		lease, err := wf.service.Claim(ctx, fmt.Sprintf("load-worker-%d", i), []string{wf.org}, time.Minute)
		if err == nil && lease.TaskID != "" {
			leases++
		} else if err != nil && !errors.Is(err, workflow.ErrNoWork) {
			t.Errorf("claim: %v", err)
		}
	}
	claimDuration := time.Since(claimStart)
	if leases != 100 {
		t.Fatalf("100 claims produced %d leases", leases)
	}

	t.Logf("load: repositories=10000 scan=%s import=%s firstPage=%s filteredSearch=%s sessions=50 sessionP50=%s sessionP95=%s sessionMax=%s claims=100 claimTotal=%s",
		scanDuration.Round(time.Millisecond), importDuration.Round(time.Millisecond), firstPage.Round(time.Millisecond),
		filterDuration.Round(time.Millisecond), latencies[24].Round(time.Millisecond), latencies[47].Round(time.Millisecond), latencies[49].Round(time.Millisecond),
		claimDuration.Round(time.Millisecond))
}
