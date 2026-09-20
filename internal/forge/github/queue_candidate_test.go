package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"reforge/internal/forge"
)

func TestReadQueueStateProvesFirstQueueCandidate(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		candidate  string
		parents    []string
		drift      bool
		omitCommit bool
		wantTested string
		wantError  bool
	}{
		{name: "candidate good", candidate: testCommit, parents: []string{testHead, testBase}, wantTested: testCommit},
		{name: "head masquerades as candidate", candidate: testHead, parents: []string{testHead, testBase}},
		{name: "wrong parent", candidate: testCommit, parents: []string{testHead, strings.Repeat("4", 40)}},
		{name: "queue drift", candidate: testCommit, parents: []string{testHead, testBase}, drift: true, wantError: true},
		{name: "absent head commit", omitCommit: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graphReads := 0
			p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/graphql" {
					var body struct {
						Query string `json:"query"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						return nil, err
					}
					graphReads++
					queue := map[string]any{"id": "Q1", "state": "AWAITING_CHECKS", "baseCommit": map[string]string{"oid": testBase}}
					if !testCase.omitCommit {
						queue["headCommit"] = map[string]string{"oid": testCase.candidate}
					}
					if testCase.drift && graphReads == 2 {
						queue["id"] = "Q2"
					}
					return jsonResponse(200, map[string]any{"data": map[string]any{"repository": map[string]any{"databaseId": 1, "pullRequest": map[string]any{"id": "PR1", "headRefOid": testHead, "baseRefOid": testBase, "mergeQueueEntry": queue}}}}), nil
				}
				if req.URL.Path == "/api/v3/repos/acme/repo" {
					return jsonResponse(200, map[string]any{"id": 1, "full_name": "acme/repo"}), nil
				}
				if req.URL.Path == "/api/v3/repos/acme/repo/commits/"+testCase.candidate {
					return jsonResponse(200, map[string]any{"sha": testCase.candidate, "commit": map[string]string{"message": "queue"}, "parents": []map[string]string{{"sha": testCase.parents[0]}, {"sha": testCase.parents[1]}}}), nil
				}
				return jsonResponse(404, nil), nil
			})
			state, err := p.ReadQueueState(context.Background(), forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, "7")
			if testCase.wantError {
				if err == nil || state.TestedSHA != "" {
					t.Fatalf("state=%+v err=%v", state, err)
				}
				return
			}
			if err != nil || state.TestedSHA != testCase.wantTested {
				t.Fatalf("state=%+v err=%v", state, err)
			}
			if testCase.wantTested != "" && graphReads != 2 {
				t.Fatalf("graph reads=%d", graphReads)
			}
		})
	}
}
