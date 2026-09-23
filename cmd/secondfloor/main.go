package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DavidBuchanan314/secondfloor/secondfloor"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s <leveldb-path>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	keys, err := secondfloor.ListKeys(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, key := range keys {
		fmt.Printf("%q\n", key)
	}
}
