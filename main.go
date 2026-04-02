package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("xin-code %s (%s %s)\n", Version, Commit, Date)
		os.Exit(0)
	}
	fmt.Println("xin-code", Version)
}
