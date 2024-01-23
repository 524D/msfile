package fcompare

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/djherbis/atime"
)

// For files less than minPartialChecksumSize, we use the full checksum as the partial checksum
// because the speed benefit of reading 1M three times is probably less than reading the entire file once
const minPartialChecksumSize = 16 * 1024 * 1024

type CompareMethod int

const (
	// Define the compare methods as constants
	CmpSize CompareMethod = iota
	CmpPartial
	CmpFull
)

// Check if we can keep the atime (access time) of files
// For this, we assume that we can set the atime if we can
// create a new file in the same directory as the given file,
// and if we can set it's atime
func TestKeepAtime(fn string) (bool, error) {
	// Get directory of file
	dir := filepath.Dir(fn)
	// Create a new file in the same directory
	f, err := os.CreateTemp(dir, "fcompare")
	if err != nil {
		return false, err
	}
	tfn := f.Name()
	f.Close()
	// Delete the new file when we are done
	defer os.Remove(tfn)

	// Set atime of new file to 2000-01-01 00:00:00
	aTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(tfn, aTime, aTime); err != nil {
		return false, err
	}

	// Get atime of new file
	aTimeChk, err := atime.Stat(tfn)
	if err != nil {
		return false, err
	}
	// Check if atime is 2000-01-01 00:00:00
	if aTime.Unix() != aTimeChk.Unix() {
		// We can't set the atime, so we can't keep the atime
		return false, nil
	}
	return true, nil
}

func CompareFiles(fns []string, method CompareMethod, keepATime bool, checkKeepAtime bool) ([][]int, error) {
	if checkKeepAtime {
		canKeep, err := TestKeepAtime(fns[0])
		if err != nil {
			return nil, err
		}
		if !canKeep {
			return nil, errors.New("can't keep atime")
		}
	}

	// Compare files, and return a list of files that are the same
	// The list of files is returned as a list of lists of integers
	// Each list of integers contains the indexes of files that are the same
	// For example, if files 1, 2, and 3 are the same, and files 4 and 5 are the same, then the return value is:
	// [[1, 2, 3], [4, 5]]
	var equalFiles [][]int
	var fis = make(map[string][]int)
	var err error
	for i, fn := range fns {
		fi, err := processFile(fn, method, keepATime)
		if err != nil {
			return equalFiles, err
		}
		// Check if we already have the same file in fis
		fis[fi] = append(fis[fi], i)
	}
	for _, v := range fis {
		equalFiles = append(equalFiles, v)
	}
	return equalFiles, err
}

func GetPartialChecksum(filename string) (string, bool, error) {
	// The partial checksum is the SHA256 sum of the first 1M of the file, plus the middle 1M of the file, plus the last 1M of the file
	// If the file is less than 16M, then the partial checksum is the SHA256 sum of the entire file
	// The limit of 16M is used because reding 16M is probably faster than reading 1M three times
	// The middle of the file is defined as the middle 1M of the file, rounded down to the nearest 1M

	isFull := false // Indicates if the partial checksum is the same as the full checksum
	// Get file size
	fi, err := os.Stat(filename)
	if err != nil {
		return "", false, err
	}
	filesize := fi.Size()

	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	h := sha256.New()

	// If the file is less than 16M, then the partial checksum is the SHA256 sum of the entire file
	if filesize <= minPartialChecksumSize {
		// Compute SHA256 sum of entire file
		if _, err := io.Copy(h, f); err != nil {
			return "", false, err
		}
		isFull = true

	} else {
		// Compute SHA256 sum of first 1M of file
		if _, err := io.CopyN(h, f, 1024*1024); err != nil {
			return "", false, err
		}

		// Compute SHA256 sum of middle 1M of file

		// Compute the middle of the file, rounded down to the nearest 1M
		filemid := filesize / 2
		filemid = filemid - (filemid % (1024 * 1024))

		// Seek to middle of file
		if _, err := f.Seek(filemid, io.SeekStart); err != nil {
			return "", false, err
		}
		if _, err := io.CopyN(h, f, 1024*1024); err != nil {
			return "", false, err
		}

		// Compute SHA256 sum of last 1M of file
		if _, err := f.Seek(-1024*1024, io.SeekEnd); err != nil {
			return "", false, err
		}
		if _, err := io.Copy(h, f); err != nil {
			return "", false, err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), isFull, nil
}

func GetChecksum(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	h := sha256.New()

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func processFile(filename string, method CompareMethod, keepATime bool) (string, error) {
	var fileinfo string

	// Get file times
	atime, err := atime.Stat(filename)
	if err != nil {
		log.Fatal(err.Error())
	}
	fi, err := os.Stat(filename)
	if err != nil {
		return fileinfo, err
	}
	mtime := fi.ModTime()

	if keepATime {
		// Restore file times before we return
		defer os.Chtimes(filename, atime, mtime)
	}

	switch method {
	case CmpPartial:
		// Get partial checksum
		fileinfo, _, err = GetPartialChecksum(filename)
		if err != nil {
			return fileinfo, err
		}
	case CmpSize:
		// Compare file sizes
		fSize := fi.Size()
		fileinfo = strconv.FormatInt(fSize, 10)
	case CmpFull:
		// Get full checksum
		fileinfo, err = GetChecksum(filename)
		if err != nil {
			return fileinfo, err
		}
	default:
		log.Fatal("Invalid compare method")
	}

	return fileinfo, nil
}
