package main

import (
	//"log"
	"os"
	"syscall"

	"bazil.org/fuse"
)

func FuseError(err error) error {
	if err == nil {
		return nil
	}

	//log.Println("FuseError", err)
	if e, ok := err.(fuse.Errno); ok {
		return e
	}
	if ok, e := convertUnderlyingError(err); ok {
		return e
	}
	if e, ok := err.(syscall.Errno); ok {
		return fuse.Errno(e)
	}
	return fuse.EIO
}

// golint: error should be the last type when returning multiple items
func convertUnderlyingError(err error) (bool, error) {
	err = underlyingError(err)
	switch err {
	case os.ErrClosed:
		return true, fuse.Errno(syscall.EBADF)
	case os.ErrExist:
		return true, fuse.EEXIST
	case os.ErrInvalid:
		return true, fuse.Errno(syscall.EINVAL)
	case os.ErrNotExist:
		return true, fuse.ENOENT
	case os.ErrPermission:
		return true, fuse.EPERM
	}
	return false, err
}

// underlyingError returns the underlying error for known os error types.
func underlyingError(err error) error {
	switch err := err.(type) {
	case *os.PathError:
		return err.Err
	case *os.LinkError:
		return err.Err
	case *os.SyscallError:
		return err.Err
	}
	return err
}
