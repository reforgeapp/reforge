package forge

import (
	"context"
	"encoding/hex"
	"strings"
)

type RefreshBranchRequest struct {
	Repository        RepoRef
	ChangeID          string
	HeadBranch        string
	TargetBranch      string
	ExpectedHeadSHA   string
	ExpectedTargetSHA string
	OperationID       string
}

type ForgeBranchRefresher interface {
	RefreshAppBranch(context.Context, RefreshBranchRequest) error
}

func (r RefreshBranchRequest) Valid() bool {
	validSHA := func(value string) bool {
		if len(value) != 40 {
			return false
		}
		_, err := hex.DecodeString(value)
		return err == nil
	}
	return r.Repository.NativeID != "" && len(r.Repository.NativeID) <= 256 && r.Repository.FullName != "" && len(r.Repository.FullName) <= 1024 && r.ChangeID != "" && len(r.ChangeID) <= 32 && strings.HasPrefix(r.HeadBranch, "reforge/repair/") && len(r.HeadBranch) > len("reforge/repair/") && len(r.HeadBranch) <= 255 && r.TargetBranch != "" && len(r.TargetBranch) <= 255 && r.TargetBranch != r.HeadBranch && validSHA(r.ExpectedHeadSHA) && validSHA(r.ExpectedTargetSHA)
}
