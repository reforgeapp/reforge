package mergecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/store"
)

func TestCompanionProofAcceptsVerifiedDescendantAndKeepsCandidateFrozen(t *testing.T) {
	candidate, observed, merged := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	item := Companion{TaskID: "task", ChangeID: "7", HeadSHA: candidate, Branch: "reforge/repair/task", TargetBranch: "main", State: "published"}
	fresh := &forge.Change{ID: "7", Repository: forge.RepoRef{NativeID: "repo", FullName: "org/repo"}, HeadRepository: forge.RepoRef{NativeID: "repo", FullName: "org/repo"}, TargetRepository: forge.RepoRef{NativeID: "repo", FullName: "org/repo"}, HeadSHA: observed, HeadBranch: item.Branch, TargetBranch: item.TargetBranch, State: "merged", MergeSHA: merged}
	calls := [][2]string{}
	proof, err := companionProof(item, fresh, fresh.Repository, item.TargetBranch, func(base, head string) (int, error) {
		calls = append(calls, [2]string{base, head})
		if base == candidate && head == observed {
			return 0, nil
		}
		if base == observed && head == candidate {
			return 2, nil
		}
		return 0, errors.New("unexpected compare")
	}, nil)
	if err != nil || proof.State != "merged" || proof.HeadSHA != candidate || proof.ObservedHeadSHA != observed || proof.MergeSHA != merged || len(calls) != 2 {
		t.Fatalf("proof=%+v calls=%v err=%v", proof, calls, err)
	}
	if !companionsCurrent([]Companion{item}, []Companion{proof}) {
		t.Fatal("valid refreshed merge proof did not match frozen DB candidate")
	}
	current := item
	current.HeadSHA = observed
	if companionsCurrent([]Companion{current}, []Companion{proof}) {
		t.Fatal("changed DB candidate was accepted")
	}
}

func TestCompanionProofRejectsDivergenceAndForeignIdentity(t *testing.T) {
	candidate, observed := strings.Repeat("a", 40), strings.Repeat("b", 40)
	item := Companion{TaskID: "task", ChangeID: "7", HeadSHA: candidate, Branch: "reforge/repair/task", TargetBranch: "main", State: "published"}
	repo := forge.RepoRef{NativeID: "repo", FullName: "org/repo"}
	fresh := &forge.Change{ID: "7", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadSHA: observed, HeadBranch: item.Branch, TargetBranch: item.TargetBranch, State: "merged", MergeSHA: strings.Repeat("c", 40)}
	proof, err := companionProof(item, fresh, repo, item.TargetBranch, func(string, string) (int, error) { return 1, nil }, nil)
	if err != nil || proof.State != "merge_pending" {
		t.Fatalf("diverged proof=%+v err=%v", proof, err)
	}
	foreign := *fresh
	foreign.HeadBranch = "reforge/repair/other"
	if _, err = companionProof(item, &foreign, repo, item.TargetBranch, nil, nil); err == nil {
		t.Fatal("foreign branch accepted")
	}
	foreign = *fresh
	foreign.Repository.NativeID = "other"
	if _, err = companionProof(item, &foreign, repo, item.TargetBranch, nil, nil); err == nil {
		t.Fatal("foreign repository accepted")
	}
}

func TestCompanionProofWaitsForMergeAndUsesValidatedMergeFallback(t *testing.T) {
	candidate, observed := strings.Repeat("a", 40), strings.Repeat("b", 40)
	item := Companion{TaskID: "task", ChangeID: "7", HeadSHA: candidate, Branch: "reforge/repair/task", TargetBranch: "main", State: "published"}
	repo := forge.RepoRef{NativeID: "repo", FullName: "org/repo"}
	fresh := &forge.Change{ID: "7", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadSHA: observed, HeadBranch: item.Branch, TargetBranch: item.TargetBranch, State: "open", MergeSHA: strings.Repeat("c", 40)}
	compares, fallbacks := 0, 0
	proof, err := companionProof(item, fresh, repo, item.TargetBranch, func(string, string) (int, error) { compares++; return 0, nil }, func() (bool, error) { fallbacks++; return true, nil })
	if err != nil || proof.State != "merge_pending" || compares != 0 || fallbacks != 0 {
		t.Fatalf("nonmerged proof=%+v compares=%d fallback=%d err=%v", proof, compares, fallbacks, err)
	}
	fresh.State = "merged"
	fresh.MergeSHA = strings.Repeat("c", 40)
	proof, err = companionProof(item, fresh, repo, item.TargetBranch, func(string, string) (int, error) { return 1, nil }, func() (bool, error) { fallbacks++; return true, nil })
	if err != nil || proof.State != "merged" || fallbacks != 1 {
		t.Fatalf("validated merge proof=%+v fallback=%d err=%v", proof, fallbacks, err)
	}
	_, err = companionProof(item, fresh, repo, item.TargetBranch, func(string, string) (int, error) { return 0, errors.New("compare failed") }, func() (bool, error) { return false, nil })
	if err == nil {
		t.Fatal("unproven merge accepted after compare failure")
	}
}

func TestValidatedMergeHeadRecordRequiresExactSuccessfulGate(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	org, repo, user, gateID, operationID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	changeID, branch, targetBranch := "77", "reforge/repair/task", "main"
	head, merge := strings.Repeat("a", 40), strings.Repeat("b", 40)
	repository := forge.RepoRef{NativeID: "native-repo", FullName: "org/repo"}
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'companion-proof-test',$2,'Owner',$2||'@example.test')`, user, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	gateDoc, _ := json.Marshal(map[string]any{
		"binding":  map[string]any{"head": head},
		"decision": map[string]any{"outcome": "allow"},
		"snapshot": map[string]any{"change": map[string]any{
			"id": changeID, "head_sha": head, "head_branch": branch, "target_branch": targetBranch,
			"repository": repository, "head_repository": repository, "target_repository": repository,
		}},
	})
	nativeResult, _ := json.Marshal(forge.MergeResult{State: "merged", HeadSHA: head, MergeSHA: merge})
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Companion proof')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,$4)`, org, repo, repository.NativeID, repository.FullName); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,1,$5)`, org, gateID, repo, changeID, gateDoc); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state,native_result) VALUES($1,$2,$3,$4,$4,$5,$6,$8,$7,'merged',$9)`, org, operationID, repo, gateID, changeID, targetBranch, user, domain.NewID(), nativeResult)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db}
	item := Companion{ChangeID: changeID, Branch: branch, TargetBranch: targetBranch}
	fresh := forge.Change{Repository: repository, HeadSHA: head, MergeSHA: merge}
	ok, err := service.hasValidatedMergeHead(ctx, org, repo, item, fresh)
	if err != nil || !ok {
		t.Fatalf("validated head proof=%t err=%v", ok, err)
	}
	for name, changed := range map[string]forge.Change{
		"different head":  {Repository: repository, HeadSHA: strings.Repeat("c", 40), MergeSHA: merge},
		"different merge": {Repository: repository, HeadSHA: head, MergeSHA: strings.Repeat("c", 40)},
	} {
		if accepted, checkErr := service.hasValidatedMergeHead(ctx, org, repo, item, changed); checkErr != nil || accepted {
			t.Fatalf("%s accepted=%t err=%v", name, accepted, checkErr)
		}
	}
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE merge_gates SET document=jsonb_set(document,'{decision,outcome}','"deny"') WHERE org_id=$1 AND id=$2`, org, gateID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ok, err = service.hasValidatedMergeHead(ctx, org, repo, item, fresh)
	if err != nil || ok {
		t.Fatalf("denied gate proof=%t err=%v", ok, err)
	}
}
