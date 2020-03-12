package main

import (
	"log"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// FuseNode implements:
//   fuse.Node
//   fuse.NodeGetattrer
//   fuse.NodeStringLookuper
//   fuse.NodeOpener
//   fuse.NodeCreater
//   fuse.NodeMkdirer
//   fuse.NodeRemover
//   fuse.NodeRenamer
//   fuse.NodeLinker
//   fuse.NodeSetattrer
//   fuse.NodeForgetter
//   fuse.NodeFsyncer
type FuseNode struct {
	fs   *FS
	ino  uint64
	attr *Stat
}

func NewFuseNode(fs *FS, ino uint64, attr *Stat) *FuseNode {
	return &FuseNode{
		fs:   fs,
		ino:  ino,
		attr: attr,
	}
}

// This method should be called with lock being held
func (fn *FuseNode) Update(stat *Stat) {
	fn.attr = stat
}

// This method will be called by the FUSE library when replying the
// following requests:
//   LOOKUP, MKDIR, CREATE, MKNOD, SYNLINK, LINK
func (fn *FuseNode) Attr(_ context.Context, a *fuse.Attr) error {
	log.Println("Attr", fn.ino)
	fillAttr(fn.attr, a)
	a.Valid = time.Minute // Attributes caching enabled
	return nil
}

// GETATTR request handler
func (fn *FuseNode) Getattr(_ context.Context, _ *fuse.GetattrRequest,
	resp *fuse.GetattrResponse) error {
	log.Println("Getattr", fn.ino)

	stat, err := fn.fs.Back.Stat(fn.ino)
	if err != nil {
		return FuseError(err)
	}
	fillAttr(stat, &resp.Attr)
	resp.Attr.Valid = time.Minute // Attributes caching enabled
	return nil
}

func (fn *FuseNode) Lookup(_ context.Context, name string) (fs.Node, error) {
	log.Println("Lookup", fn.ino, name)

	stat, err := fn.fs.Back.Lookup(fn.ino, name)
	if err != nil {
		return nil, FuseError(err)
	}

	return fn.fs.LoadNode(stat.Ino, stat), nil
}

func (fn *FuseNode) Open(
	_ context.Context,
	req *fuse.OpenRequest, _ *fuse.OpenResponse) (fs.Handle, error) {
	log.Println("Open", fn.ino, req.Dir, req.Flags)

	err := fn.fs.Back.Open(fn.ino, int(req.Flags))
	if err != nil {
		return nil, FuseError(err)
	}
	return NewFuseHandle(fn.fs, fn.ino, int(req.Flags), req.Header.Pid), nil
}

func (fn *FuseNode) Create(
	_ context.Context,
	req *fuse.CreateRequest,
	_ *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	log.Println("Create", fn.ino, req.Name, req.Flags, req.Mode)
	stat, err := fn.fs.Back.Create(fn.ino, req.Name, int(req.Flags), req.Mode)
	if err != nil {
		return nil, nil, FuseError(err)
	}
	return fn.fs.LoadNode(stat.Ino, stat),
		NewFuseHandle(fn.fs, stat.Ino, int(req.Flags), req.Header.Pid),
		nil
}

func (fn *FuseNode) Mkdir(
	_ context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	log.Println("Mkdir", fn.ino, req.Name, req.Mode)
	stat, err := fn.fs.Back.Mkdir(fn.ino, req.Name, req.Mode)
	if err != nil {
		return nil, FuseError(err)
	}
	return fn.fs.LoadNode(stat.Ino, stat), nil
}

func (fn *FuseNode) Remove(_ context.Context, req *fuse.RemoveRequest) error {
	log.Printf("Remove %v %s: Dir %v", fn.ino, req.Name, req.Dir)
	if req.Dir {
		return FuseError(fn.fs.Back.Rmdir(fn.ino, req.Name))
	}
	return FuseError(fn.fs.Back.Unlink(fn.ino, req.Name))
}

func (fn *FuseNode) Rename(
	_ context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	log.Printf("Rename <%v, %s> to <%v, %s>",
		fn.ino, req.OldName, req.NewDir, req.NewName)
	dNode, _ := newDir.(*FuseNode)
	return FuseError(fn.fs.Back.Rename(
		fn.ino, req.OldName, dNode.ino, req.NewName))
}

func (fn *FuseNode) Link(
	_ context.Context, req *fuse.LinkRequest, old fs.Node) (fs.Node, error) {
	oldFn, _ := old.(*FuseNode)
	log.Printf("Link <%v, %s> to %v", fn.ino, req.NewName, oldFn.ino)
	stat, err := fn.fs.Back.Link(oldFn.ino, fn.ino, req.NewName)
	if err != nil {
		return nil, FuseError(err)
	}
	return fn.fs.LoadNode(stat.Ino, stat), nil
}

func (fn *FuseNode) Setattr(
	_ context.Context,
	req *fuse.SetattrRequest, _ *fuse.SetattrResponse) error {
	log.Println("Setattr", fn.ino, req)

	attrs := make(map[string]interface{})

	// Chmod, change permissions of a file
	if req.Valid&fuse.SetattrMode != 0 {
		attrs["mode"] = req.Mode
	}

	// Chown, change file owner and group
	if req.Valid&fuse.SetattrGid != 0 {
		attrs["gid"] = req.Gid
	}
	if req.Valid&fuse.SetattrUid != 0 {
		attrs["uid"] = req.Uid
	}

	// Open(O_TRUNC), truncate ftruncate ...
	if req.Valid&fuse.SetattrSize != 0 {
		attrs["size"] = req.Size
	}

	// Change file last access and modification times:
	//   futimes, utimes, utimensat ...
	if req.Valid&fuse.SetattrAtime != 0 {
		attrs["atime"] = req.Atime
	}
	if req.Valid&fuse.SetattrMtime != 0 {
		attrs["mtime"] = req.Mtime
	}

	if len(attrs) == 0 {
		return FuseError(syscall.EINVAL)
	}

	stat, err := fn.fs.Back.Setattr(fn.ino, attrs)
	if err != nil {
		return FuseError(err)
	}
	fn.Update(stat)
	return nil
}

func (fn *FuseNode) Forget() {
	log.Println("Forget", fn.ino)
	fn.fs.RemoveNode(fn.ino)
}

// This should be a Handle method, but brazil.org/fuse treats
//   it as a Node method :(
func (fn *FuseNode) Fsync(_ context.Context, req *fuse.FsyncRequest) error {
	log.Printf("Fsync %v: Handle %v, Flags %v, Dir %v",
		fn.ino, req.Handle, req.Flags, req.Dir)
	return FuseError(fn.fs.Back.Fsync(fn.ino, req.Flags, req.Dir))
}

func fillAttr(stat *Stat, attr *fuse.Attr) {
	if stat == nil || attr == nil {
		log.Printf("Warnning: fillAttr(%v, %v)", stat, attr)
		return
	}
	attr.Inode = stat.Ino
	attr.Size = stat.Size
	attr.BlockSize = stat.BlockSize
	attr.Blocks = stat.Blocks
	attr.Atime = stat.Atime
	attr.Mtime = stat.Mtime
	attr.Ctime = stat.Ctime
	attr.Crtime = stat.Crtime
	attr.Mode = stat.Mode
	attr.Nlink = stat.Nlink
	attr.Uid = stat.UID
	attr.Gid = stat.GID
}
