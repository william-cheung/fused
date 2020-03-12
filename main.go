package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
)

const version = "0.0.1"

var fstypes = [...]string{"memfs"}

var usage = func() {
	fmt.Fprintf(os.Stderr, "usage: %s [options] mountpoint\n", os.Args[0])
	fmt.Fprintf(os.Stderr, " options:\n")
	flag.PrintDefaults()
}

func validateFSType(fstype string) bool {
	for _, t := range fstypes {
		if fstype == t {
			return true
		}
	}
	return false
}

func NewFS(fstype string) fs.FS {
	var back BackendFS
	switch fstype {
	default:
		back = NewMemFS()
	case "memfs":
		back = NewMemFS()
		// other fs types ...
	}
	return &FS{Back: back}
}

func main() {
	flag.Usage = usage
	vflag := flag.Bool("version", false, "print version information")
	tflag := flag.String("type", "memfs", fmt.Sprintf(
		"specify filesystem type. filesystems supported: %v", fstypes))
	flag.Parse()
	if *vflag {
		fmt.Fprintf(os.Stderr, "%s\n", version)
	}
	if flag.NArg() != 1 {
		if *vflag {
			os.Exit(0)
		}
		usage()
		os.Exit(2)
	}
	fstype := *tflag
	if !validateFSType(fstype) {
		usage()
		os.Exit(2)
	}

	mountpoint := flag.Arg(0)

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName(fstype),
		fuse.Subtype(fstype),
		fuse.LocalVolume(),
		fuse.VolumeName(fstype),
		fuse.NoAppleDouble(),
		fuse.NoAppleXattr(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	err = fs.Serve(c, NewFS(fstype))
	if err != nil {
		log.Fatal(err)
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; err != nil {
		log.Fatal(err)
	}
}
