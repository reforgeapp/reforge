package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/inventory"
)

func TestInventoryTenThousandRepositoriesAcrossTenOrganisations(t *testing.T) {
	if testing.Short() {
		t.Skip("scale acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	const organisations = 10
	const repositories = 1000
	fixtures := make([]*inventoryFixture, organisations)
	scans := make([]inventory.Job, organisations)
	imports := make([]inventory.Job, organisations)
	for i := range fixtures {
		fixtures[i] = newInventoryFixture(t, repositories)
	}
	started := time.Now()
	for i, f := range fixtures {
		scans[i] = f.sync(t)
	}
	for phase := 0; phase < 2; phase++ {
		if phase == 1 {
			for i, f := range fixtures {
				var err error
				imports[i], err = f.service.StartImport(ctx, f.owner, f.org, scans[i].ID, inventory.ImportInput{All: true}, scans[i].Version, "scale-fixture")
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		complete := make([]bool, organisations)
		remaining := organisations
		for round := 0; remaining > 0; round++ {
			if round > repositories/100+2 {
				t.Fatal("scale jobs exceeded bounded page rounds")
			}
			for i, f := range fixtures {
				if complete[i] {
					continue
				}
				jobID := scans[i].ID
				if phase == 1 {
					jobID = imports[i].ID
				}
				job, err := f.service.Job(ctx, f.owner, f.org, jobID)
				if err != nil {
					t.Fatal(err)
				}
				if job.State == "complete" {
					if phase == 0 {
						scans[i] = job
					}
					if job.Processed != repositories || job.Pages != repositories/100 {
						t.Fatalf("organisation %d job incomplete: %+v", i, job)
					}
					complete[i] = true
					remaining--
					continue
				}
				if job.State == "failed" || job.State == "stale" {
					t.Fatalf("organisation %d job state %s", i, job.State)
				}
				lease, err := f.service.Claim(ctx, f.org, fmt.Sprintf("scale-org-%02d", i))
				if err != nil || lease == nil {
					t.Fatalf("organisation %d claim: %v", i, err)
				}
				if err := f.service.Step(ctx, *lease); err != nil {
					t.Fatalf("organisation %d step: %v", i, err)
				}
			}
		}
	}
	ingestDuration := time.Since(started)
	readStart := time.Now()
	listSamples := make([]time.Duration, 0, organisations*5)
	searchSamples := make([]time.Duration, 0, organisations*5)
	var pageReads int
	for org, f := range fixtures {
		cursor := ""
		seen := 0
		for {
			before := time.Now()
			page, err := f.service.Repositories(ctx, f.owner, f.org, 200, cursor)
			listSamples = append(listSamples, time.Since(before))
			if err != nil {
				t.Fatal(err)
			}
			pageReads++
			seen += len(page.Items)
			if page.Complete {
				break
			}
			cursor = page.NextCursor
		}
		if seen != repositories {
			t.Fatalf("organisation %d listed %d repositories", org, seen)
		}
		for i := 0; i < 5; i++ {
			before := time.Now()
			result, err := f.service.RepositoriesFiltered(ctx, f.owner, f.org, 20, "", inventory.RepositoryFilter{Query: fmt.Sprintf("repository-%04d", i*100), Provider: "github"})
			searchSamples = append(searchSamples, time.Since(before))
			if err != nil || len(result.Items) == 0 {
				t.Fatalf("organisation %d search: %v", org, err)
			}
		}
	}
	for i, f := range fixtures {
		other := fixtures[(i+1)%organisations]
		row, err := f.service.Repositories(ctx, f.owner, f.org, 1, "")
		if err != nil || len(row.Items) != 1 {
			t.Fatalf("organisation %d missing fixture row: %v", i, err)
		}
		if _, err = other.service.Repository(ctx, other.owner, other.org, row.Items[0].ID); err == nil {
			t.Fatalf("organisation %d read organisation %d repository", (i+1)%organisations, i)
		}
	}
	t.Logf("repositories=%d organisations=%d scan_and_import_duration=%s list_pages=%d list_p50=%s list_p95=%s search_samples=%d search_p50=%s search_p95=%s read_phase=%s fixture_provider=true concurrent_executors=0", organisations*repositories, organisations, ingestDuration, pageReads, percentile(listSamples, .50), percentile(listSamples, .95), len(searchSamples), percentile(searchSamples, .50), percentile(searchSamples, .95), time.Since(readStart))
}

func percentile(samples []time.Duration, fraction float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	index := int(float64(len(sorted)-1) * fraction)
	return sorted[index]
}
