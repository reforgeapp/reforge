package customcmd

import (
	"path"
	"regexp"
	"strings"
	"time"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var executablePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

func ValidDigest(value string) bool { return digestPattern.MatchString(value) }

func Validate(p Profile) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 120 {
		return ErrInvalid
	}
	if !ValidDigest(p.ImageDigest) {
		return ErrInvalid
	}
	if len(p.Executable) == 0 || len(p.Executable) > 512 || !strings.HasPrefix(p.Executable, "/") || !executablePattern.MatchString(p.Executable) || strings.Contains(p.Executable, "..") || path.Clean(p.Executable) != p.Executable {
		return ErrInvalid
	}
	if len(p.Argv) > 64 {
		return ErrInvalid
	}
	for _, arg := range p.Argv {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) || strings.ContainsAny(arg, ";&|`$<>\n\r") {
			return ErrInvalid
		}
	}
	if p.ProtocolVersion != ProtocolVersion {
		return ErrInvalid
	}
	if p.MaxWallSeconds < 1 || p.MaxWallSeconds > 3600 || p.MaxOutputBytes < 1 || p.MaxOutputBytes > 16<<20 || p.MaxTurns < 1 || p.MaxTurns > 128 || p.Concurrency < 1 || p.Concurrency > 8 {
		return ErrInvalid
	}
	return nil
}

func validateSpec(p ProfileSpec) error {
	if p.ID == "" || p.Version < 1 {
		return ErrInvalid
	}
	if !ValidDigest(p.ImageDigest) {
		return ErrInvalid
	}
	if len(p.Executable) == 0 || len(p.Executable) > 512 || !strings.HasPrefix(p.Executable, "/") || !executablePattern.MatchString(p.Executable) || strings.Contains(p.Executable, "..") || path.Clean(p.Executable) != p.Executable {
		return ErrInvalid
	}
	if len(p.Argv) > 64 {
		return ErrInvalid
	}
	for _, arg := range p.Argv {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) || strings.ContainsAny(arg, ";&|`$<>\n\r") {
			return ErrInvalid
		}
	}
	if p.ProtocolVersion != ProtocolVersion {
		return ErrInvalid
	}
	if p.MaxWallSeconds < 1 || p.MaxWallSeconds > 3600 || p.MaxOutputBytes < 1 || p.MaxOutputBytes > 16<<20 || p.MaxTurns < 1 || p.MaxTurns > 128 || p.Concurrency < 1 || p.Concurrency > 8 {
		return ErrInvalid
	}
	return nil
}

func Approved(p Profile, now time.Time) bool {
	return p.RevokedAt == nil && p.ApprovedAt != nil && p.ApprovedBy != "" && !p.ApprovedAt.IsZero() && !p.ApprovedAt.After(now)
}
