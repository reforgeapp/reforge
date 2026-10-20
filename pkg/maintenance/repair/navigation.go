package repair

import (
	"bytes"
	"encoding/json"
	"errors"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

const navigationOutputLimit = 64 << 10
const navigationFileLimit = 4 << 20
const navigationPageLimit = 100
const navigationSearchLimit = 100

var errNavigationInput = errors.New("invalid repository navigation request")
var errNavigationFile = errors.New("repository file is unavailable or not text")

type navigationFileChunk struct {
	Path       string `json:"path"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"next_offset"`
	TotalBytes int    `json:"total_bytes"`
	Done       bool   `json:"done"`
	Content    string `json:"content"`
}

type navigationPathPage struct {
	Paths      []string `json:"paths"`
	Offset     int      `json:"offset"`
	NextOffset int      `json:"next_offset"`
	Total      int      `json:"total"`
	Done       bool     `json:"done"`
}

type navigationMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type navigationSearchPage struct {
	Matches    []navigationMatch `json:"matches"`
	Offset     int               `json:"offset"`
	NextOffset int               `json:"next_offset"`
	Done       bool              `json:"done"`
}

func readSnapshotChunk(files map[string][]byte, name string, offset, limit int) ([]byte, error) {
	if !guest.ValidPath(name) || offset < 0 || limit < 0 || limit > navigationOutputLimit {
		return nil, errNavigationInput
	}
	body, ok := files[name]
	if !ok || len(body) > navigationFileLimit || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 || offset > len(body) || offset < len(body) && !utf8.RuneStart(body[offset]) || ok && secretFile(name, body) {
		return nil, errNavigationFile
	}
	if limit == 0 {
		limit = navigationOutputLimit
	}
	end := min(offset+limit, len(body))
	for end > offset && end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	if end == offset && offset < len(body) {
		_, size := utf8.DecodeRune(body[offset:])
		end = offset + size
	}
	encode := func(next int) ([]byte, error) {
		return json.Marshal(navigationFileChunk{Path: name, Offset: offset, NextOffset: next, TotalBytes: len(body), Done: next == len(body), Content: string(body[offset:next])})
	}
	encoded, err := encode(end)
	if err != nil {
		return nil, err
	}
	if len(encoded) <= navigationOutputLimit {
		return encoded, nil
	}
	boundaries := []int{offset}
	for cursor := offset; cursor < end; {
		_, size := utf8.DecodeRune(body[cursor:end])
		cursor += size
		boundaries = append(boundaries, cursor)
	}
	low, high := 0, len(boundaries)-1
	for low < high {
		mid := low + (high-low+1)/2
		candidate, err := encode(boundaries[mid])
		if err != nil {
			return nil, err
		}
		if len(candidate) <= navigationOutputLimit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	encoded, err = encode(boundaries[low])
	if err != nil || len(encoded) > navigationOutputLimit {
		return nil, errNavigationFile
	}
	return encoded, nil
}

func listSnapshotPaths(files map[string][]byte, glob string, offset, limit int) ([]byte, error) {
	if offset < 0 || limit < 0 || limit > navigationPageLimit || len(glob) > 1024 {
		return nil, errNavigationInput
	}
	if glob != "" {
		if _, err := path.Match(glob, ""); err != nil {
			return nil, errNavigationInput
		}
	}
	paths := make([]string, 0, len(files))
	for name := range files {
		if !guest.ValidPath(name) {
			continue
		}
		if glob != "" {
			matched, err := path.Match(glob, name)
			if err != nil {
				return nil, errNavigationInput
			}
			if !matched {
				continue
			}
		}
		paths = append(paths, name)
	}
	slices.Sort(paths)
	if offset > len(paths) {
		return nil, errNavigationInput
	}
	if limit == 0 {
		limit = navigationPageLimit
	}
	page := navigationPathPage{Paths: []string{}, Offset: offset, NextOffset: offset, Total: len(paths), Done: offset == len(paths)}
	for _, name := range paths[offset:] {
		if len(page.Paths) >= limit {
			break
		}
		page.Paths = append(page.Paths, name)
		page.NextOffset = offset + len(page.Paths)
		page.Done = page.NextOffset == page.Total
		encoded, err := json.Marshal(page)
		if err != nil {
			return nil, err
		}
		if len(encoded) > navigationOutputLimit {
			page.Paths = page.Paths[:len(page.Paths)-1]
			page.NextOffset = offset + len(page.Paths)
			page.Done = false
			break
		}
	}
	return json.Marshal(page)
}

func searchSnapshotContent(files map[string][]byte, literal, glob string, offset, limit int) ([]byte, error) {
	if literal == "" || len(literal) > 256 || !utf8.ValidString(literal) || strings.ContainsAny(literal, "\x00\r\n") || offset < 0 || limit < 0 || limit > navigationSearchLimit || len(glob) > 1024 {
		return nil, errNavigationInput
	}
	if glob != "" {
		if _, err := path.Match(glob, ""); err != nil {
			return nil, errNavigationInput
		}
	}
	paths := make([]string, 0, len(files))
	for name := range files {
		if guest.ValidPath(name) {
			paths = append(paths, name)
		}
	}
	slices.Sort(paths)
	if limit == 0 {
		limit = navigationSearchLimit
	}
	page := navigationSearchPage{Matches: []navigationMatch{}, Offset: offset}
	index := 0
	more := false
	for _, name := range paths {
		if glob != "" {
			matched, err := path.Match(glob, name)
			if err != nil {
				return nil, errNavigationInput
			}
			if !matched {
				continue
			}
		}
		body := files[name]
		if len(body) > navigationFileLimit || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 || secretFile(name, body) {
			continue
		}
		for start, lineNo := 0, 1; start <= len(body); lineNo++ {
			rel := bytes.IndexByte(body[start:], '\n')
			end := len(body)
			next := len(body) + 1
			if rel >= 0 {
				end, next = start+rel, start+rel+1
			}
			line := body[start:end]
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if bytes.Contains(line, []byte(literal)) {
				if index >= offset {
					if len(page.Matches) == limit {
						more = true
						break
					}
					page.Matches = append(page.Matches, navigationMatch{Path: name, Line: lineNo, Text: searchSnippet(line, literal)})
					page.NextOffset = offset + len(page.Matches)
					encoded, err := json.Marshal(page)
					if err != nil {
						return nil, err
					}
					if len(encoded) > navigationOutputLimit {
						page.Matches = page.Matches[:len(page.Matches)-1]
						page.NextOffset = offset + len(page.Matches)
						more = true
						break
					}
				}
				index++
			}
			if rel < 0 {
				break
			}
			start = next
		}
		if more {
			break
		}
	}
	if offset > index {
		return nil, errNavigationInput
	}
	page.Done = !more
	return json.Marshal(page)
}

func searchSnippet(line []byte, literal string) string {
	const snippetLimit = 512
	if len(line) <= snippetLimit {
		return string(line)
	}
	match := bytes.Index(line, []byte(literal))
	start := max(0, match-192)
	end := min(len(line), start+snippetLimit)
	for start > 0 && !utf8.RuneStart(line[start]) {
		start--
	}
	for end < len(line) && !utf8.RuneStart(line[end]) {
		end--
	}
	text := string(line[start:end])
	if start > 0 {
		text = "…" + text
	}
	if end < len(line) {
		text += "…"
	}
	return text
}
