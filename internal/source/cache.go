package source

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"sync"

	"github.com/reforgeapp/reforge/internal/forge"
)

type BlobCache struct {
	mu    sync.Mutex
	limit int
	size  int
	order *list.List
	items map[string]*list.Element
}

type cachedBlob struct {
	key  string
	data []byte
}

func NewBlobCache(limit int) *BlobCache {
	return &BlobCache{limit: limit, order: list.New(), items: map[string]*list.Element{}}
}

func (c *BlobCache) get(scope, format, sha string) ([]byte, bool) {
	if c == nil || scope == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[scope+"/"+format+"/"+sha]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(item)
	return item.Value.(*cachedBlob).data, true
}

func (c *BlobCache) put(scope, format, sha string, data []byte) {
	if c == nil || scope == "" || len(data) > c.limit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := scope + "/" + format + "/" + sha
	if _, ok := c.items[key]; ok {
		return
	}
	c.items[key] = c.order.PushFront(&cachedBlob{key: key, data: data})
	c.size += len(data)
	for c.size > c.limit {
		oldest := c.order.Back()
		blob := oldest.Value.(*cachedBlob)
		c.order.Remove(oldest)
		delete(c.items, blob.key)
		c.size -= len(blob.data)
	}
}

func (r Reader) Blob(ctx context.Context, repo forge.RepoRef, entry forge.SourceEntry, format, commit string) ([]byte, error) {
	if data, ok := r.Cache.get(r.Scope, format, entry.SHA); ok {
		return data, nil
	}
	f, err := r.File(ctx, repo, entry.Path, commit)
	if err != nil {
		return nil, err
	}
	if f.Path != entry.Path || f.SHA != "" && f.SHA != entry.SHA || len(f.Content) > MaxFileBytes || !VerifyBlob(format, entry.SHA, f.Content) {
		return nil, errors.New("invalid source file")
	}
	content := bytes.Clone(f.Content)
	r.Cache.put(r.Scope, format, entry.SHA, content)
	return content, nil
}
