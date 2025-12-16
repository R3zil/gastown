package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("gt - Gas Town CLI")
	fmt.Println("Version: 0.0.1 (development)")
	if len(os.Args) > 1 {
		fmt.Printf("Command: %s\n", os.Args[1])
		fmt.Println("Not yet implemented. See gastown-py for current functionality.")
	}
}
