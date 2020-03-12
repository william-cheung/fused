package main

import (
	"log"
	"os"
	"sync"
	"syscall"
	"time"
)

// This is a compile-time assertion to ensure that MemFS implements
// BackendFS interface
var _ BackendFS = (*MemFS)(nil)

// nolint: deadcode
func NewMemFS() *MemFS {
	fs := &MemFS{
		itable: make(map[uint64]*MemInode),

		// Next free ino, starting from 2
		// 0 is resevred for indicating errors, 1 is ino of root directory
		inoNextFree: 2,
	}
	mode := os.ModeDir | 0777
	fs.itable[1] = NewMemInode(fs, nil, 1, mode)
	return fs
}

type MemFS struct {
	mu          sync.Mutex // protects the following fields
	inoNextFree uint64
	itable      map[uint64]*MemInode
}

func (fs *MemFS) LoadInode(ino uint64) (*MemInode, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	inode, ok := fs.itable[ino]
	return inode, ok
}

func (fs *MemFS) RemoveInode(ino uint64) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.itable, ino)
}

func (fs *MemFS) StoreInode(
	ino uint64, inode *MemInode) *MemInode {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.itable[ino] = inode
	return inode
}

func (fs *MemFS) GenerateIno() uint64 {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	ino := fs.inoNextFree
	fs.inoNextFree++
	return ino
}

func (fs *MemFS) Stat(ino uint64) (*Stat, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Stat(), nil
}

func (fs *MemFS) Open(ino uint64, flags int) error {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	inode.Reference()
	return nil
}

func (fs *MemFS) Create(
	ino uint64, name string, flags int, mode os.FileMode) (*Stat, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	childIno, err := inode.AddDirent(0, name, modeType(mode))
	if err != nil {
		return nil, err
	}
	childInode := NewMemInode(fs, inode, childIno, mode)
	childInode.Reference()
	fs.StoreInode(childIno, childInode)
	return childInode.Stat(), nil
}

func (fs *MemFS) Mkdir(
	ino uint64, name string, mode os.FileMode) (*Stat, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	childIno, err := inode.AddDirent(0, name, modeType(mode))
	if err != nil {
		return nil, err
	}
	childInode := NewMemInode(fs, inode, childIno, mode)
	fs.StoreInode(childIno, childInode)
	return childInode.Stat(), nil
}

func (fs *MemFS) Rmdir(ino uint64, name string) error {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Rmdir(name)
}

func (fs *MemFS) Unlink(ino uint64, name string) error {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Unlink(name)
}

func (fs *MemFS) Rename(
	sIno uint64, sName string, dIno uint64, dName string) error {
	sInode, ok := fs.LoadInode(sIno)
	if !ok {
		return syscall.ENOENT
	}
	dInode, ok := fs.LoadInode(dIno)
	if !ok {
		return syscall.ENOENT
	}

	// nolint: gocritic
	if sIno == dIno {
		sInode.Lock()
		defer sInode.Unlock()
	} else if sIno < dIno {
		sInode.Lock()
		defer sInode.Unlock()
		dInode.Lock()
		defer dInode.Unlock()
	} else {
		dInode.Lock()
		defer dInode.Unlock()
		sInode.Lock()
		defer sInode.Unlock()
	}

	sDirent, err := sInode.GetDirent(sName)
	if err != nil {
		return err
	}
	dDirent, err := dInode.GetDirent(dName)
	if err != nil && err != syscall.ENOENT {
		return err
	}

	if err != syscall.ENOENT && sDirent.Ino == dDirent.Ino {
		// If oldpath(<sIno, sName>) and newpath(<dIno, dName>)
		// are existing hard links referring to the same file,
		// then rename does nothing, and returns a success status.
		return nil
	}

	if err == nil {
		if !sDirent.Type.IsDir() && dDirent.Type.IsDir() {
			return syscall.EISDIR
		}
		if sDirent.Type.IsDir() && !dDirent.Type.IsDir() {
			return syscall.ENOTDIR
		}

		if dDirent.Type.IsDir() {
			if err := dInode.Rmdir(dName); err != nil {
				return err
			}
		} else {
			if err := dInode.Unlink(dName); err != nil {
				return err
			}
		}
	}

	// nolint: errcheck
	dInode.AddDirent(sDirent.Ino, dName, sDirent.Type)
	// nolint: errcheck
	sInode.RemoveDirent(sName)
	return nil
}

func (fs *MemFS) Setattr(
	ino uint64, attrs map[string]interface{}) (*Stat, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Setattr(attrs)
}

func (fs *MemFS) Lookup(ino uint64, name string) (*Stat, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Lookup(name)
}

func (fs *MemFS) Readdir(
	ino uint64, marker string, n int) ([]Dirent, string, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, "", syscall.ENOENT
	}

	if len(marker) > 0 || n > 0 {
		return nil, "", syscall.ENOSYS
	}

	inode.Lock()
	defer inode.Unlock()

	dirents, err := inode.Readdir(-1)
	return dirents, "", err
}

func (fs *MemFS) Read(ino uint64, offset int64, n int) ([]byte, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return nil, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Read(offset, n)
}

func (fs *MemFS) Write(ino uint64, offset int64, data []byte) (int, error) {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return 0, syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Write(offset, data)
}

func (fs *MemFS) Fsync(ino uint64, datasync uint32, dir bool) error {
	// nolint: unparam
	return nil
}

func (fs *MemFS) Flush(ino uint64) error {
	return syscall.ENOSYS
}

func (fs *MemFS) Release(ino uint64, flags int) error {
	inode, ok := fs.LoadInode(ino)
	if !ok {
		return syscall.ENOENT
	}

	inode.Lock()
	defer inode.Unlock()

	return inode.Release()
}

type MemInode struct {
	fs *MemFS

	ino uint64

	mu sync.Mutex // Lock protecting the following fields

	// Number of hard links: 0 or 1 in current implementation
	nlink uint32

	// Reference counter, also number of open file handles that point to this
	// inode.
	// Generally, this is a field of in-core inodes. Disk inodes do not have
	// this field
	count uint32

	mode os.FileMode

	// Note that distinction between writing contents of an inode to storage
	// and writing the contents of a file to storage. The contents of a file
	// change only when writing it. The contents of inode change when changing
	// the contents of a file or when changing its owner, permission, or link
	// settings. Changing the contents of a file implies a change to the inode,
	// but not vise versa.
	//           -- Book: Design of the Unix Operating System By Maurice Bach
	//
	// Note that atime always >= mtime, ctime always >= mtime
	atime  time.Time
	mtime  time.Time
	ctime  time.Time
	crtime time.Time

	dirents *ListMap
	data    []byte
}

// nolint: errcheck
func NewMemInode(
	fs *MemFS, parent *MemInode, ino uint64, mode os.FileMode) *MemInode {
	crtime := time.Now()
	inode := &MemInode{
		fs: fs,

		ino:    ino,
		nlink:  1,
		count:  0,
		mode:   mode,
		atime:  crtime,
		mtime:  crtime,
		ctime:  crtime,
		crtime: crtime,

		dirents: NewListMap(),
		data:    make([]byte, 0),
	}

	if mode&os.ModeDir != 0 {
		inode.AddDirent(ino, ".", modeType(mode))
		inode.nlink++
		if parent != nil {
			inode.AddDirent(parent.ino, "..", modeType(parent.mode))
			parent.nlink++
		} else { // Root directory
			inode.AddDirent(ino, "..", modeType(mode))
		}
	}

	return inode
}

func (inode *MemInode) Lock() {
	inode.mu.Lock()
	inode.atime = time.Now()
}

func (inode *MemInode) Unlock() {
	inode.mu.Unlock()
}

func (inode *MemInode) Reference() {
	inode.count++
}

func (inode *MemInode) AddDirent(
	ino uint64, name string, t os.FileMode) (uint64, error) {
	dirent := inode.dirents.Get(name)
	if dirent != nil {
		return 0, syscall.EEXIST
	}

	if ino == 0 {
		ino = inode.fs.GenerateIno()
	}

	inode.dirents.Put(name, &Dirent{
		Ino: ino, Name: name, Type: t,
	})

	inode.mtime = time.Now()
	inode.ctime = inode.mtime
	return ino, nil
}

func (inode *MemInode) GetDirent(name string) (*Dirent, error) {
	dirent := inode.dirents.Get(name)
	if dirent == nil {
		return nil, syscall.ENOENT
	}
	return dirent.(*Dirent), nil
}

func (inode *MemInode) RemoveDirent(name string) (uint64, error) {
	dirent := inode.dirents.Get(name)
	if dirent == nil {
		return 0, syscall.ENOENT
	}
	inode.dirents.Delete(name)

	inode.mtime = time.Now()
	inode.ctime = inode.mtime
	return dirent.(*Dirent).Ino, nil
}

func (inode *MemInode) Setattr(attrs map[string]interface{}) (*Stat, error) {
	if mode, ok := attrs["mode"]; ok {
		m, _ := mode.(os.FileMode)
		inode.mode = m
		inode.ctime = time.Now()
	}

	if _, ok := attrs["gid"]; ok {
		return nil, syscall.ENOSYS
	}
	if _, ok := attrs["uid"]; ok {
		return nil, syscall.ENOSYS
	}

	if atime, ok := attrs["atime"]; ok {
		at, _ := atime.(time.Time)
		inode.atime = at
		inode.ctime = time.Now()
	}

	if mtime, ok := attrs["mtime"]; ok {
		mt, _ := mtime.(time.Time)
		inode.mtime = mt
		inode.ctime = time.Now()
	}

	if size, ok := attrs["size"]; ok {
		if inode.mode&os.ModeDir != 0 {
			return nil, syscall.EISDIR
		}
		sz, _ := size.(uint64)
		inode.data = PadRight(inode.data, 0, int(sz))
		inode.mtime = time.Now()
		inode.ctime = inode.mtime
	}

	return inode.Stat(), nil
}

func (inode *MemInode) Lookup(name string) (*Stat, error) {
	dirent := inode.dirents.Get(name)
	if dirent == nil {
		return nil, syscall.ENOENT
	}
	return inode.fs.Stat(dirent.(*Dirent).Ino)
}

func (inode *MemInode) Rmdir(name string) error {
	dirent := inode.dirents.Get(name)
	if dirent == nil {
		return syscall.ENOENT
	}

	if dirent.(*Dirent).Type&os.ModeDir == 0 {
		return syscall.ENOTDIR
	}

	child, _ := inode.fs.LoadInode(dirent.(*Dirent).Ino)
	child.Lock()
	defer child.Unlock()

	if child.mode&os.ModeDir != 0 && child.dirents.Len() > 2 {
		// The directory contains entries other than . and ..
		return syscall.ENOTEMPTY
	}
	child.dirents.Delete("..")
	inode.nlink--
	child.dirents.Delete(".")
	child.nlink--
	return inode.doRemove(child, name)
}

func (inode *MemInode) Unlink(name string) error {
	dirent := inode.dirents.Get(name)
	if dirent == nil {
		return syscall.ENOENT
	}

	child, _ := inode.fs.LoadInode(dirent.(*Dirent).Ino)
	child.Lock()
	defer child.Unlock()
	return inode.doRemove(child, name)
}

func (inode *MemInode) doRemove(child *MemInode, name string) error {
	_, _ = inode.RemoveDirent(name)

	if child.nlink == 0 {
		log.Println("(*MemInode).doRemove: nlink is already 0")
	} else {
		child.nlink--
		child.ctime = time.Now()
	}

	if child.nlink == 0 && child.count == 0 {
		inode.fs.RemoveInode(child.ino)
		return nil
	}
	return nil
}

// Returns the first n Dirent in a directory
// nolint: unparam
func (inode *MemInode) Readdir(n int) ([]Dirent, error) {
	if n <= 0 {
		n = inode.dirents.Len()
	}

	res := make([]Dirent, 0)
	count := 0
	for _, dirent := range inode.dirents.Values() {
		if count < n {
			res = append(res, *(dirent.(*Dirent)))
			count++
		} else {
			break
		}
	}
	return res, nil
}

func (inode *MemInode) Read(offset int64, n int) ([]byte, error) {
	// TODO check offset

	if n <= 0 || n >= len(inode.data)-int(offset) {
		return inode.data[offset:], nil
	}
	return inode.data[offset : offset+int64(n)], nil
}

func (inode *MemInode) Write(offset int64, data []byte) (int, error) {
	// TODO check offset

	buff := PadRight(inode.data, 0, int(offset))
	buff = append(buff, data...)
	if len(buff) > len(inode.data) {
		inode.data = buff
	} else {
		inode.data = append(buff, inode.data[len(buff):]...)
	}

	if len(data) > 0 {
		inode.mtime = inode.atime
		inode.ctime = inode.atime
	}
	return len(data), nil
}

func PadRight(src []byte, pad byte, lengh int) []byte {
	for {
		src = append(src, pad)
		if len(src) > lengh {
			return src[0:lengh]
		}
	}
}

func (inode *MemInode) Release() error {
	if inode.count > 0 {
		inode.count--
	}
	if inode.nlink == 0 && inode.count == 0 {
		inode.fs.RemoveInode(inode.ino)
	}
	return nil
}

func (inode *MemInode) Stat() *Stat {
	size := uint64(len(inode.data))
	return &Stat{
		Ino:       inode.ino,
		Mode:      inode.mode,
		Nlink:     inode.nlink,
		UID:       0,
		GID:       0,
		Size:      size,
		BlockSize: 512,
		Blocks:    (size + 511) / 512,
		Atime:     inode.atime,
		Mtime:     inode.mtime,
		Ctime:     inode.ctime,
		Crtime:    inode.crtime,
	}
}

func modeType(mode os.FileMode) os.FileMode {
	return mode & os.ModeType
}
