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
		}

		command = strings.TrimSpace(command)

		switch {
		case command == "exit 0":
			os.Exit(0)
		case strings.HasPrefix(command, "echo "):
			fmt.Println(strings.TrimPrefix(command, "echo "))
		default:
			fmt.Println(command + ": command not found")
		}
	}
}
