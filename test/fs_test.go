package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	crand "crypto/rand"
	"encoding/base64"
)

var mountpoint = flag.String(
	"mountpoint", "/tmp/fuse", "path name of the mount point")

func fstype() string {
	if !strings.HasPrefix(*mountpoint, "/") {
		panic("mountpoint is a relative path")
	}

	cmd := exec.Command("mount")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		panic(err.Error())
	}
	scanner := bufio.NewScanner(&out)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 3 {
			if fields[2] == *mountpoint {
				return fields[0]
			}
			if runtime.GOOS == "darwin" &&
				fields[2] == "/private"+*mountpoint {
				return fields[0]
			}
		}
	}
	log.Printf("warnning: filesystem type is not specified")
	return ""
}

func randstring(n int) string {
	b := make([]byte, 2*n)
	crand.Read(b)
	s := base64.URLEncoding.EncodeToString(b)
	return s[0:n]
}

func realpath(name string) string {
	return filepath.Join(*mountpoint, name)
}

func createDir(dir string, subdirs []string) error {
	dirname := realpath(dir)
	if err := os.Mkdir(dirname, 0755); err != nil {
		return err
	}
	for _, subdir := range subdirs {
		name := filepath.Join(dirname, subdir)
		if err := os.Mkdir(name, 0755); err != nil {
			return err
		}
	}
	return nil
}

// flag: 0 silent on error; 1 log on error; >=2 return error
func cleanDir(dir string, subdirs []string, flag int) error {
	dirname := realpath(dir)
	for _, subdir := range subdirs {
		name := filepath.Join(dirname, subdir)
		if err := os.Remove(name); err != nil {
			if flag == 0 {
				continue
			} else if flag == 1 {
				log.Printf("cleanDir: " + err.Error())
				continue
			}
			return err
		}
	}
	if err := os.Remove(dirname); err != nil {
		if flag == 0 {
			return nil
		} else if flag == 1 {
			log.Printf("cleanDir: " + err.Error())
		} else {
			return err
		}
	}
	return nil
}

func checkDirExist(dir string) error {
	dirname := realpath(dir)
	f, err := os.Open(dirname)
	if err != nil {
		return err
	}
	return f.Close()
}

func removeDir(dir string) error {
	dirname := realpath(dir)
	for i := 0; i < 3; i++ {
		err := os.Remove(dirname)
		if i == 0 && err != nil {
			return err
		}
		if i != 0 && err == nil {
			return fmt.Errorf(
				"Removing a non-exist directory '%s' should fail",
				dirname)
		}
	}
	return nil
}

func checkDirContents(dir string, subdirs []string) error {
	dirname := realpath(dir)
	f, err := os.Open(dirname)
	if err != nil {
		return err
	}
	if err := fCheckDirContents(f, subdirs); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func fCheckDirContents(f *os.File, entries []string) error {
	names, err := f.Readdirnames(0)
	if err != nil {
		return err
	}
	if len(names) != len(entries) {
		return errors.New(
			fmt.Sprintf(
				"number of directory entries: expected %v, but got %v",
				len(entries), len(names)))
	}

	for _, entry := range entries {
		exist := false
		for _, name := range names {
			if entry == name {
				exist = true
				break
			}
		}
		if !exist {
			return fmt.Errorf(
				"directory entry with name '%s' should exist", entry)
		}
	}
	return nil
}

func testCreateDir(t *testing.T, dir string, subdirs []string) {
	log.Printf("Test: Create directories ...")
	err := createDir(dir, subdirs)
	if err != nil {
		t.Fatalf(
			"Fail to create dir '%s' with subdirs '%v': %+v",
			dir, subdirs, err)
	}
	log.Printf(" ... Passed")
}

func testCleanDir(t *testing.T, dir string, subdirs []string) {
	log.Printf("Test: Remove directories ...")
	err := cleanDir(dir, subdirs, 2)
	if err != nil {
		t.Fatalf(
			"Fail to clean dir '%s' with subdirs '%v': %+v",
			dir, subdirs, err)
	}
	log.Printf(" ... Passed")
}

func testRemoveDir(t *testing.T, dir string) {
	log.Printf("Test: Remove a directory (many times) ...")
	if err := removeDir(dir); err != nil {
		t.Fatalf(err.Error())
	}
	log.Printf(" ... Passed")
}

func TestConcurrentBasic(t *testing.T) {
	testdir := "testdir-" + randstring(8)
	testCreateDir(t, testdir, nil)
	defer testRemoveDir(t, testdir)

	nGoroutines := 16
	wg := sync.WaitGroup{}
	wg.Add(nGoroutines)
	for i := 0; i < nGoroutines; i++ {
		index := i
		go func(index int) {
			defer wg.Done()
			subdir := "subdir-" + strconv.Itoa(index)
			name := filepath.Join(testdir, subdir)
			if err := createDir(name, nil); err != nil {
				t.Errorf(err.Error())
				return
			}
			if err := checkDirExist(name); err != nil {
				t.Errorf(err.Error())
				return
			}
			removeDir(name)
		}(index)
	}
	wg.Wait()
}

func TestConcurrentMkdirRmdir(t *testing.T) {
	testdir := "testdir-" + randstring(8)
	subdirs := make([]string, 32)
	for i := 0; i < len(subdirs); i++ {
		subdirs[i] = "subdir-" + randstring(8)
	}

	log.Printf("Test: Concurrent mkdirs ...")
	if err := createDir(testdir, nil); err != nil {
		t.Fatalf(err.Error())
	}
	mkdirCount := 2
	nMkdirers := len(subdirs) / mkdirCount
	runtime.GOMAXPROCS(nMkdirers - 1)
	wg := sync.WaitGroup{}
	wg.Add(nMkdirers)
	for i := 0; i < nMkdirers; i++ {
		mkdirer := i
		go func(mkdirer int) {
			defer wg.Done()
			begin, end := mkdirer*mkdirCount, (mkdirer+1)*mkdirCount
			for j := begin; j < end && j < len(subdirs); j++ {
				name := filepath.Join(testdir, subdirs[j])
				if err := createDir(name, nil); err != nil {
					t.Errorf("Mkdirer %v: "+err.Error(), mkdirer)
					return
				}
			}
		}(mkdirer)
	}
	wg.Wait()
	if !t.Failed() {
		log.Printf(" ... Passed")
	} else {
		return
	}

	log.Printf("Test: Concurrent rmdirs ...")
	rmdirCount := 2
	nRmdirers := len(subdirs) / rmdirCount
	runtime.GOMAXPROCS(nRmdirers - 1)
	wg2 := sync.WaitGroup{}
	wg2.Add(nRmdirers)
	for i := 0; i < nRmdirers; i++ {
		rmdirer := i
		go func(rmdirer int) {
			defer wg2.Done()
			begin, end := rmdirer*rmdirCount, (rmdirer+1)*rmdirCount
			for j := begin; j < end && j < len(subdirs); j++ {
				name := filepath.Join(testdir, subdirs[j])
				if err := removeDir(name); err != nil {
					t.Errorf("Rmdirer %v: "+err.Error(), rmdirer)
					return
				}
			}
		}(rmdirer)
	}
	wg2.Wait()
	if err := removeDir(testdir); err != nil {
		t.Fatalf(err.Error())
	}
	if !t.Failed() {
		log.Printf(" ... Passed")
	}
}

func TestConcurrentReaddirs(t *testing.T) {
	testdir := "testdir-" + randstring(8)
	subdirs := []string{"d1", "d2", "d6", "d3", "d5", "d4"}

	testCreateDir(t, testdir, subdirs)
	defer testCleanDir(t, testdir, subdirs)

	log.Printf("Test: Concurrent readdirs ...")
	nReaders := 16
	runtime.GOMAXPROCS(nReaders - 1)
	wg := sync.WaitGroup{}
	wg.Add(nReaders)
	for i := 0; i < nReaders; i++ {
		reader := i
		go func(reader int) {
			defer wg.Done()
			if err := checkDirContents(testdir, subdirs); err != nil {
				t.Errorf("Reader %v: %+v", reader, err.Error())
				return
			}
		}(reader)
	}
	wg.Wait()
	if !t.Failed() {
		log.Printf(" ... Passed")
	}
}

func TestConcurrentReaddirRmdir(t *testing.T) {
	if runtime.GOOS == "linux" {
		// We skip the test due to its failure on Ubuntu 18.04
		// Notes:
		//  Rmdir will make a reference to a directory unaccessible
		//  on Ubuntu 18.04, which is not a standard behaviour. See:
		//   https://pubs.opengroup.org/onlinepubs/9699919799/functions/rmdir.html
		t.Skip()
	}

	testdir := "testdir-" + randstring(8)
	testCreateDir(t, testdir, nil)
	defer cleanDir(testdir, nil, 0)

	log.Printf("Test: Concurrent readdir & rmdir ...")
	nReaders := 16
	runtime.GOMAXPROCS(nReaders - 1)
	wg1, wg2 := sync.WaitGroup{}, sync.WaitGroup{}
	wg1.Add(nReaders)
	wg2.Add(nReaders)
	for i := 0; i < nReaders; i++ {
		reader := i
		go func(reader int) {
			defer wg2.Done()
			f, err := func() (*os.File, error) {
				defer wg1.Done()
				f, err := os.Open(realpath(testdir))
				return f, err
			}()
			if err != nil {
				t.Errorf("Reader %v: %+v", reader, err.Error())
				return
			}
			// Sleep to wait the execution of "os.Rmdir"
			time.Sleep(100 * time.Millisecond)
			if err := fCheckDirContents(f, nil); err != nil {
				t.Errorf("Reader %v: %+v", reader, err.Error())
				return
			}
		}(reader)
	}
	wg1.Wait()
	if err := removeDir(testdir); err != nil {
		t.Fatalf(err.Error())
	}
	wg2.Wait()
	if !t.Failed() {
		log.Printf(" ... Passed")
	}
}

func creat(path string, mode uint32) (int, error) {
	// Under Linux, a call to creat() (a syscall) is equivalent to calling
	// open() with flags equal to O_CREAT|O_WRONLY|O_TRUNC.
	// See http://man7.org/linux/man-pages/man2/open.2.html
	return syscall.Open(
		path, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC, mode)
}

func write(fd int, data []byte, oneshot bool) error {
	for {
		if len(data) == 0 {
			return nil
		}
		n, err := syscall.Write(fd, data)
		if err != nil {
			return err
		}
		if oneshot && n != len(data) {
			return fmt.Errorf("short write: %d instead of %d", n, len(data))
		}
		data = data[n:]
	}
}

func writeAll(fd int, data []byte) error {
	sizePerWrite := 1000
	oneshot := false
	if runtime.GOOS == "darwin" {
		// Set data size per write to page size on OS X
		sizePerWrite, oneshot = 4096, true
	}

	size := len(data)
	count := 0
	for count < size {
		n := size - count
		if n > sizePerWrite {
			n = sizePerWrite
		}
		if err := write(fd, data[count:count+n], oneshot); err != nil {
			return err
		}
		count += n
	}
	return nil
}

func creatWithContent(path string, data []byte) (int, error) {
	fd, err := creat(path, 0644)
	if err != nil {
		return -1, err
	}
	if err := writeAll(fd, data); err != nil {
		return -1, err
	}
	return fd, nil
}

func checkFileExist(path string) error {
	var stat syscall.Stat_t
	if err := syscall.Lstat(path, &stat); err != nil {
		if err != syscall.ENOENT {
			return fmt.Errorf(
				"Unexpected error when calling lstat against file %s: %v",
				path, err)
		}
		return err
	}
	return nil
}

func checkFileNonexist(path string) error {
	err := checkFileExist(path)
	if err == nil {
		return fmt.Errorf("File %s should not exist", path)
	}
	if err == syscall.ENOENT {
		return nil
	}
	return err
}

func checkFileSize(path string, expected int64) error {
	var stat syscall.Stat_t
	if err := syscall.Lstat(path, &stat); err != nil {
		return err
	}
	if stat.Size != expected {
		return fmt.Errorf("file size: expected %v, bug got %v",
			expected, stat.Size)
	}
	return nil
}

func fcheckFileSize(fd int, expected int64) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Size != expected {
		return fmt.Errorf("file size: expected %v, bug got %v",
			expected, stat.Size)
	}
	return nil
}

func checkFileContent(path string, expected []byte) error {
	if err := checkFileSize(path, int64(len(expected))); err != nil {
		return err
	}
	return checkFileContentAt(path, 0, expected)
}

func checkFileContentAt(path string, offset int, expected []byte) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	return fcheckFileContentAt(fd, offset, expected)
}

func fcheckFileContentAt(fd int, offset int, expected []byte) error {
	_, err := syscall.Seek(fd, int64(offset), io.SeekStart)
	if err != nil {
		return err
	}

	size := len(expected)
	buff := make([]byte, 1024)
	count := 0
	shortReads := 0
	for count < size {
		p := buff
		if size-count < len(buff) {
			p = buff[:size-count]
		}
		n, err := syscall.Read(fd, p)
		if err != nil {
			return err
		}
		if n < len(p) {
			log.Printf("Warning: short read %d instead of %d", n, len(p))
			shortReads++
			if shortReads > 10 {
				return fmt.Errorf("too many short reads")
			}
		}
		if !bytes.Equal(p[:n], expected[count:count+n]) {
			return fmt.Errorf(
				"file content at offset %d, expected '%s', but got '%s'",
				offset, string(expected[count:count+n]), string(p[:n]))
		}
		count += n
		offset += n
	}
	return nil
}

func TestReadFromWriteOnlyFile(t *testing.T) {
	testfile := realpath("testfile-" + randstring(8))
	fd, err := creat(testfile, 0644)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer func() {
		if err := syscall.Unlink(testfile); err != nil {
			t.Fatalf(err.Error())
		}
		if err := syscall.Close(fd); err != nil {
			t.Fatalf(err.Error())
		}
	}()
	err = write(fd, []byte("hello, world"), false)
	if err != nil {
		t.Fatalf(err.Error())
	}
	syscall.Seek(fd, 0, io.SeekStart)
	buff := make([]byte, 16)
	_, err = syscall.Read(fd, buff)
	if err == nil || err != syscall.EBADF {
		t.Fatalf("Error expected '%s', but got '%s'", syscall.EBADF, err)
	}
}

func TestConcurrentReads(t *testing.T) {
	testfile := realpath("testfile-" + randstring(8))

	defer syscall.Unlink(testfile)

	filelen := 8000
	content := []byte(randstring(filelen))

	log.Printf("Test: File write (%d bytes) ...", len(content))

	fd, err := creatWithContent(testfile, content)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if err := syscall.Close(fd); err != nil {
		t.Fatalf(err.Error())
	}

	log.Printf(" ... Passed")

	log.Printf("Test: Concurrent reads ...")
	nreaders := 4
	wg := sync.WaitGroup{}
	wg.Add(nreaders)
	for r := 0; r < nreaders; r++ {
		go func(reader int) {
			defer wg.Done()
			if err := checkFileContent(testfile, []byte(content)); err != nil {
				t.Errorf("Reader %v: %+v", reader, err.Error())
			}
		}(r)
	}
	wg.Wait()
	if !t.Failed() {
		log.Printf(" ... Passed")
	} else {
		t.FailNow()
	}

	log.Printf("Test: Concurrent random reads ...")
	nRandReads := 4
	wg = sync.WaitGroup{}
	wg.Add(nRandReads)
	for r := 0; r < nRandReads; r++ {
		go func(reader int) {
			defer wg.Done()

			offset := rand.Intn(filelen)
			n := filelen - offset
			if n > 1024 {
				n = 1024
			}
			expected_data := []byte(content[offset : offset+n])
			err := checkFileContentAt(testfile, offset, expected_data)
			if err != nil {
				t.Errorf("Reader %v: %+v", reader, err.Error())
			}
		}(r)
	}
	wg.Wait()
	if !t.Failed() {
		log.Printf(" ... Passed")
	}
}

func TestConcurrentWrites(t *testing.T) {
	filelen := 4096
	nwriters := 8

	log.Printf("Test: Concurrent Writes (%d writers, %d bytes each) ...",
		nwriters, filelen)

	wg := sync.WaitGroup{}
	wg.Add(nwriters)
	for w := 0; w < nwriters; w++ {
		go func(writer int) {
			defer wg.Done()

			testfile := realpath("testfile-" + randstring(8))
			content := []byte(randstring(filelen))

			fd, err := creatWithContent(testfile, content)
			if err != nil {
				t.Errorf(
					"Writer %v: creat & write %s: %+v",
					writer, testfile, err)
				return
			}

			defer func() {
				if err := syscall.Close(fd); err != nil {
					t.Errorf("Writer %v: close %s: %v", writer, testfile, err)
				}
			}()

			if err := checkFileContent(testfile, []byte(content)); err != nil {
				t.Errorf("Writer %v: read %s: %+v", writer, testfile, err)
				return
			}
		}(w)
	}
	wg.Wait()

	if !t.Failed() {
		log.Printf(" ... Passed")
	}
}

func TestConcurrentRenames(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// Skip the test under OS X
		// Something seems not right under OS X
		t.Skip()
	}

	oldName := realpath("testfile-" + randstring(8))
	newName := realpath("testfile-" + randstring(8))

	fd, err := creat(oldName, 0644)
	if err != nil {
		t.Fatalf(err.Error())
	}

	defer func() {
		_ = syscall.Unlink(oldName)
		_ = syscall.Unlink(newName)
		_ = syscall.Close(fd)
	}()

	concurrency := 16
	count := int32(0)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			err := syscall.Rename(oldName, newName)
			if err != nil {
				if err != syscall.ENOENT {
					t.Errorf(
						"Unexpected error when renaming %s to %s: %v",
						oldName, newName, err)
				}
				return
			}
			atomic.AddInt32(&count, 1)
		}()
	}
	wg.Wait()

	if err := checkFileNonexist(oldName); err != nil {
		t.Fatalf(err.Error())
	}

	if err := checkFileExist(newName); err != nil {
		t.Fatalf(err.Error())
	}

	if count != 1 {
		t.Fatalf("# of succeeded renames is %d instead of 1", count)
	}
}
