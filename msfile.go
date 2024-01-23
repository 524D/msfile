package main

// msfile.go - A utility to get and compare Mass Spectrometry file metadata
// msfile is similar to the Linux file command, but is designed to work with Mass Spectrometry files
// Output of msfile is a JSON string, which can be used by other programs

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/524D/msfile/fcompare"
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
			fileinfo.PartialChecksum, isFull, err = fcompare.GetPartialChecksum(filename)
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
			fileinfo.FullChecksum, err = fcompare.GetChecksum(filename)
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

	for _, fn := range flag.Args() {
		canKeep, _ := fcompare.TestKeepAtime(fn)
		if !canKeep {
			log.Fatalln("Warning: unable to preserve file times for", fn)
		}
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
