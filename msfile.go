package main

// msfile.go - A utility to get and compare Mass Spectrometry file metadata
// msfile is similar to the Linux file command, but is designed to work with Mass Spectrometry files
// Output of msfile is a JSON string, which can be used by other programs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/djherbis/atime"
)

// For files less than minPartialChecksumSize, we use the full checksum as the partial checksum
// because the speed benefit of reading 1M three times is probably less than reading the entire file once
const minPartialChecksumSize = 16 * 1024 * 1024

type params struct {
	compare bool
	json    bool
	method  string
}

type FileInfo struct {
	Filename        string
	Size            int64
	Atime           int64
	Mtime           int64
	PartialChecksum string
	FullChecksum    string
	Properties      map[string]string
}

// flags:
//  -compare: compare two files
//  -json: produce output in JSON format
//  -comparemethod: partial, size, full (default: partial)

var par params

// parse flags
func handleCommandLine() {
	flag.BoolVar(&par.compare, "compare", false, "compare files, instead of printing results")
	flag.BoolVar(&par.json, "json", false, "produce output in JSON format")
	flag.StringVar(&par.method, "comparemethod", "partial", "method to use when comparing files (partial, size, full))")

	flag.Parse()

}

func getPartialChecksum(filename string) (string, bool, error) {
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

func getChecksum(filename string) (string, error) {
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

func processFile(filename string) (FileInfo, error) {
	var fileinfo FileInfo

	fileinfo.Properties = make(map[string]string)
	fileinfo.Filename = filename
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

	// Convert times to Unix time
	fileinfo.Atime = atime.Unix()
	fileinfo.Mtime = mtime.Unix()

	// Restore file times before we return
	defer os.Chtimes(filename, atime, mtime)

	fileinfo.Size = fi.Size()

	if par.compare {
		// Compare files

		// Use appropriate method to compare files
		switch par.method {
		case "partial":
			// Get partial checksum
			isFull := false
			fileinfo.PartialChecksum, isFull, err = getPartialChecksum(filename)
			if err != nil {
				return fileinfo, err
			}
			if isFull {
				fileinfo.FullChecksum = fileinfo.PartialChecksum
			}
		case "size":
			// Compare file sizes
		case "full":
			// Get full checksum
			fileinfo.FullChecksum, err = getChecksum(filename)
			if err != nil {
				return fileinfo, err
			}
		default:
			log.Fatal("Invalid compare method")
		}
	}

	return fileinfo, nil

}

func main() {
	handleCommandLine()

	// Print usage if no arguments are provided
	if flag.NArg() == 0 {
		fmt.Println("Usage: msfile [options] file1 [file2]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Check if we are comparing files
	if par.compare {
		// This only works with 2 files
		if flag.NArg() != 2 {
			log.Fatal("Compare option only works with 2 files")
		} else {
			inf1, err := processFile(flag.Args()[0])
			if err != nil {
				log.Fatal(err)
			}
			inf2, err := processFile(flag.Args()[1])
			if err != nil {
				log.Fatal(err)
			}
			if (par.method == "partial" && inf1.PartialChecksum == inf2.PartialChecksum) ||
				(par.method == "size" && inf1.Size == inf2.Size) ||
				(par.method == "full" && inf1.FullChecksum == inf2.FullChecksum) {
				fmt.Println("Files are the same")
			} else {
				fmt.Println("Files are different")
			}
		}
	} else {

		// for all remaining arguments
		for _, arg := range flag.Args() {
			// process each file
			inf, err := processFile(arg)
			if err != nil {
				log.Fatal(err)
			}
			// Output in JSON format if requested
			if par.json {
				// Convert inf to a JSON string
				j, err := json.Marshal(inf)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println(string(j))
			} else {
				fmt.Printf("%+v\n", inf)
			}

		}
	}
}
