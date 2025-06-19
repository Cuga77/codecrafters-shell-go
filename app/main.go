package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var _ = fmt.Fprint

func main() {
	for {
		fmt.Fprint(os.Stdout, "$ ")

		command, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
			os.Exit(1)
		}
		if strings.TrimSpace(command) == "exit 0" {
			os.Exit(0)
		}

		if strings.TrimSpace(command) == "echo" {
			fmt.Println(command[:len(command)-2])
		}

		fmt.Println(command[:len(command)-1] + ": command not found")
	}
}
