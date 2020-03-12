package main

import (
	"io"
	"log"
	"syscall"

	"bazil.org/fuse"
	"golang.org/x/net/context"
)

// FuseHandle implements:
//   fuse.HandleReadAller
//   fuse.HandleReadDirAller
//   fuse.HandleReader
//   fuse.HandleWriter
//   fuse.HandleFlusher
//   fuse.HandleReleaser
type FuseHandle struct {
	fs    *FS
	ino   uint64
	flags int    // Open flags
	pid   uint32 // Process that opens this handle
}

func NewFuseHandle(fs *FS, ino uint64, flags int, pid uint32) *FuseHandle {
	return &FuseHandle{
		fs:    fs,
		ino:   ino,
		flags: flags & (syscall.O_RDONLY | syscall.O_WRONLY | syscall.O_RDWR),
		pid:   pid,
	}
}

func (fh *FuseHandle) ReadDirAll(_ context.Context) ([]fuse.Dirent, error) {
	log.Println("ReadDirAll", fh.ino)
	dirents, _, err := fh.fs.Back.Readdir(fh.ino, "", 0)
	if err != nil {
		return nil, FuseError(err)
	}

	log.Printf("ReadDirAll %v: Size %v", fh.ino, len(dirents))
	res := make([]fuse.Dirent, 0)
	for _, dirent := range dirents {
		res = append(res, fuse.Dirent{
			Inode: dirent.Ino,
			Name:  dirent.Name,
			Type:  fuse.DirentType(dirent.Type),
		})
	}
	return res, nil
}

func (fh *FuseHandle) ReadAll(_ context.Context) ([]byte, error) {
	log.Println("ReadAll", fh.ino)
	b, err := fh.fs.Back.Read(fh.ino, 0, -1)
	if err != nil && err != io.EOF {
		return nil, FuseError(err)
	}
	log.Printf("ReadAll %v: %v Bytes", fh.ino, len(b))
	return b, nil
}

func (fh *FuseHandle) Read(
	_ context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	log.Printf(
		"Read %v: Offset %v, Size %v", fh.ino, req.Offset, req.Size)
	// TODO check req.Flags
	b, err := fh.fs.Back.Read(fh.ino, req.Offset, req.Size)
	if err != nil && err != io.EOF {
		return FuseError(err)
	}
	resp.Data = b
	return nil
}

func (fh *FuseHandle) Write(
	_ context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	log.Printf(
		"Write %v: Size %v Offset %v", fh.ino, len(req.Data), req.Offset)
	// TODO check req.Flags
	if req.Header.Pid != fh.pid {
		log.Printf("Write access denied: the writer (pid %v) is not "+
			"the creator (pid %v)", req.Header.Pid, fh.pid)
		return FuseError(syscall.EACCES)
	}
	n, err := fh.fs.Back.Write(fh.ino, req.Offset, req.Data)
	if err != nil {
		return FuseError(err)
	}
	resp.Size = n
	return nil
}

func (fh *FuseHandle) Flush(_ context.Context, _ *fuse.FlushRequest) error {
	log.Println("Flush", fh.ino)
	return FuseError(fh.fs.Back.Flush(fh.ino))
}

func (fh *FuseHandle) Release(_ context.Context, req *fuse.ReleaseRequest) error {
	log.Println("Realese", fh.ino, req.Flags, req.ReleaseFlags)
	flags := int(req.Flags) & (syscall.O_RDONLY | syscall.O_WRONLY | syscall.O_RDWR)
	if flags != fh.flags {
		log.Printf("Bug: 'flags' in RELEASE request (%v) is not same as "+
			"'flags' in the corresponding OPEN request (%v)",
			fuse.OpenFlags(flags), fuse.OpenFlags(fh.flags))
	}
	// Releasedir: req.Flags&syscall.O_DIRECTORY != 0

	return FuseError(fh.fs.Back.Release(fh.ino, fh.flags))
}
