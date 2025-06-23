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

		switch command {
		case "echo":
			fmt.Println(strings.TrimPrefix(command, "echo "))
		case "exit 0":
			os.Exit(0)
		default:
			fmt.Println(command[:len(command)-1] + ": command not found")
		}
	}
}
