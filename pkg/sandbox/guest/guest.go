package guest

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
)

type File struct {
	Path       string `json:"path"`
	Content    []byte `json:"content"`
	Executable bool   `json:"executable,omitempty"`
	Delete     bool   `json:"delete,omitempty"`
}

type Request struct {
	Operation string `json:"operation"`
	Files     []File `json:"files,omitempty"`
	Path      string `json:"path,omitempty"`
	Limit     int64  `json:"limit,omitempty"`
}

func ValidPath(value string) bool {
	return value != "" && len(value) <= 1024 && value == path.Clean(value) && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\\x00") && value != ".git" && !strings.HasPrefix(value, ".git/") && value != "." && strings.Count(value, "/") < 64
}

func Run(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 1 && args[0] == "hold" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if len(args) != 0 {
		return errors.New("invalid sandbox tool arguments")
	}
	var request Request
	decoder := json.NewDecoder(io.LimitReader(input, 96<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid sandbox tool input")
	}
	root, err := os.OpenRoot("/workspace")
	if err != nil {
		return err
	}
	defer root.Close()
	switch request.Operation {
	case "apply":
		if len(request.Files) == 0 || len(request.Files) > 20000 {
			return errors.New("file count exceeded")
		}
		seen := map[string]bool{}
		size := 0
		for _, f := range request.Files {
			size += len(f.Content)
			if !ValidPath(f.Path) || seen[f.Path] || size > 64<<20 || len(f.Content) > 64<<20 {
				return errors.New("invalid or oversized file")
			}
			seen[f.Path] = true
			if info, err := root.Lstat(f.Path); err == nil && !info.Mode().IsRegular() {
				return errors.New("nonregular patch target")
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		for _, f := range request.Files {
			if f.Delete {
				if err := root.Remove(f.Path); err != nil {
					return err
				}
				continue
			}
			if err := root.MkdirAll(path.Dir(f.Path), 0700); err != nil {
				return err
			}
			mode := os.FileMode(0600)
			if f.Executable {
				mode = 0700
			}
			file, err := root.OpenFile(f.Path, os.O_WRONLY|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, mode)
			if err != nil {
				return err
			}
			opened, statErr := file.Stat()
			if statErr != nil || !opened.Mode().IsRegular() {
				file.Close()
				return errors.New("patch target changed to nonregular file")
			}
			if err = file.Truncate(0); err != nil {
				file.Close()
				return err
			}
			_, err = file.Write(f.Content)
			if err == nil {
				err = file.Chmod(mode)
			}
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		_, err = io.WriteString(output, "{}\n")
		return err
	case "read":
		if !ValidPath(request.Path) || request.Limit < 1 || request.Limit > 4<<20 {
			return errors.New("invalid artifact request")
		}
		info, err := root.Lstat(request.Path)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("artifact must be a regular file")
		}
		file, err := root.OpenFile(request.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !opened.Mode().IsRegular() {
			return errors.New("artifact changed to nonregular file")
		}
		body, err := io.ReadAll(io.LimitReader(file, request.Limit+1))
		if err != nil {
			return err
		}
		if int64(len(body)) > request.Limit {
			return errors.New("artifact limit exceeded")
		}
		return json.NewEncoder(output).Encode(map[string]string{"content": base64.StdEncoding.EncodeToString(body)})
	default:
		return fmt.Errorf("unsupported sandbox tool operation")
	}
}
