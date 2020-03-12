package main

import (
	"sync"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// FS implements:
//   fs.FS
//   fs.FSStatfser
//   fs.FSInodeGenerator
//
// TODO implement fs.FSDestroyer interface
type FS struct {
	Back BackendFS // backing filesystem

	mu sync.Mutex // lock guarding nodeMap

	// Ino -> FuseNode mapping. The mapping is necessary because the FUSE
	// library uses fs.Node (FuseNode here) as a map key. Methods that return
	// fs.Node should return the same fs.Node when the result is logically
	// the same instance, otherwise unexpected behavior may happen.
	nodeMap map[uint64]*FuseNode
}

func (s *FS) Root() (fs.Node, error) {
	return s.LoadNode(1, nil), nil
}

// nolint: unparam
func (s *FS) Statfs(
	_ context.Context,
	_ *fuse.StatfsRequest, resp *fuse.StatfsResponse) error {
	// Total data blocks in filesystem
	resp.Blocks = 4096
	// Free blocks in filesystem
	resp.Bfree = 4096
	// Free blocks available to unprivileged user (non-superuser)
	resp.Bavail = 0
	// Total file nodes in filesystem
	resp.Files = 0
	// Free file nodes in filesystem
	resp.Ffree = 0
	resp.Bsize = 4096
	// Maximum length of filenames
	resp.Namelen = 255
	// Fragment size (since Linux 2.6)
	resp.Frsize = 1

	return nil
}

func (s *FS) LoadNode(ino uint64, stat *Stat) *FuseNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nodeMap == nil {
		s.nodeMap = make(map[uint64]*FuseNode)
	}
	n, ok := s.nodeMap[ino]
	if !ok {
		n = NewFuseNode(s, ino, stat)
		s.nodeMap[ino] = n
	}
	n.Update(stat)
	return n
}

func (s *FS) RemoveNode(ino uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodeMap, ino)
}
