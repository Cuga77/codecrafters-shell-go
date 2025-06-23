package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var _ = fmt.Fprint

var commands = []string{
	"echo",
	"cd",
	"exit",
	"tipe",
}

func main() {
	for {
		fmt.Fprint(os.Stdout, "$ ")
		command, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
		}

		command = strings.TrimSpace(command)

		switch {
		case strings.HasPrefix(command, "type "):
			cmd := strings.TrimPrefix(command, "type ")
			for _, validCommand := range commands {
				if validCommand == cmd {
					fmt.Println(cmd, "is a shell builtin")
					continue
				}
			}
			fmt.Println(cmd + ": not found")
		case command == "exit 0":
			os.Exit(0)
		case strings.HasPrefix(command, "echo "):
			fmt.Println(strings.TrimPrefix(command, "echo "))
		default:
			fmt.Println(command + ": command not found")
		}
	}
}
