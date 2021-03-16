package helpers

import (
	"github.com/go-git/go-billy/v5"
	"github.com/willscott/go-nfs"
	"path/filepath"
	"strings"
)

// NewBinaryHandler wraps a handler to provide a basic to/from-file handle cache.
func NewBinaryHandler(h nfs.Handler, fs billy.Filesystem) nfs.Handler {
	return &CachingHandler{
		Handler: h,
		fs: fs,
	}
}

// CachingHandler implements to/from handle via an LRU cache.
type CachingHandler struct {
	nfs.Handler
	fs billy.Filesystem
}

// ToHandle takes a file and represents it with an opaque handle to reference it.
// In stateless nfs (when it's serving a unix fs) this can be the device + inode
// but we can generalize with a stateful local cache of handed out IDs.
func (c *CachingHandler) ToHandle(f billy.Filesystem, path []string) []byte {
	joined := filepath.Join(path...)
	if len(joined) == 0 {
		return c.ToHandle(f, []string{"/"})
	}
	return []byte(filepath.Join(path...))
}

// FromHandle converts from an opaque handle to the file it represents
func (c *CachingHandler) FromHandle(fh []byte) (billy.Filesystem, []string, error) {
	path := string(fh)
	if path == "/" {
		return c.fs, []string{""}, nil
	}
	return c.fs, strings.Split(path, string(filepath.Separator)), nil
}

// HandleLimit exports how many file handles can be safely stored by this cache.
func (c *CachingHandler) HandleLimit() int {
	return 1024
}
