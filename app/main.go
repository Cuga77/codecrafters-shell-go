package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

var _ = fmt.Fprint

var COMMANDS = []string{
	"echo",
	"cd",
	"exit",
	"type",
	"pwd",
}

func contains(command string, commands []string) bool {
	return slices.Contains(commands, command)
}

func invalidCommand(command string) {
	fmt.Println(command[:] + ": command not found")
}

func echo(input []string) {
	if len(input) < 1 {
		fmt.Println("Not enough arguments for echo")
	}
	fmt.Println(strings.Join(input, " "))
}

func pwd(args []string) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		return
	}
	fmt.Println(dir)
}

func exit(input []string) {
	if len(input) < 1 {
		fmt.Println("Not enough arguments for exit")
		return
	}
	exitCode, err := strconv.Atoi(input[0])
	if err == nil {
		os.Exit(exitCode)
	} else {
		fmt.Println(err)
	}
}

func changeDirectory(args []string) {
	var targetDir string
	var err error
	if len(args) == 0 || args[0] == "~" {
		targetDir, err = os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cd: %v\n", err)
			return
		}
	} else {
		targetDir = args[0]
	}

	err = os.Chdir(targetDir)
	if err != nil {
		fmt.Printf("cd: %s: No such file or directory\n", targetDir)
	}
}

func searchPath(command string) (string, bool) {
	path := os.ExpandEnv("$PATH")
	pathParts := strings.Split(path, ":")
	for _, part := range pathParts {
		files, err := os.ReadDir(part)
		if err != nil {
			continue
		}
		for _, file := range files {
			if strings.TrimSpace(file.Name()) == strings.TrimSpace(command) {
				info, err := file.Info()
				if err == nil && info.Mode().Perm()&0111 != 0 {
					return part, true

				}
			}
		}
	}
	return "", false
}

func typeBuiltIn(input []string) {
	if len(input) < 1 {
		fmt.Println("Not enough arguments for type")
		return
	} else if contains(input[0], COMMANDS) {
		fmt.Println(input[0], "is a shell builtin")
		return
	}
	path, found := searchPath(input[0])
	if found {
		fmt.Printf("%s is %s/%s\n", input[0], path, input[0])
	} else {
		fmt.Printf("%s: not found\n", input[0])
	}
}

func executeExternalCommand(programName string, args []string) {
	fullPath, found := searchPath(programName)
	if found {
		cmd := &exec.Cmd{
			Path:   fullPath + "/" + programName,
			Args:   append([]string{programName}, args...),
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}

		err := cmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		}
	} else {
		invalidCommand(programName)
	}
}

func parseCommand(command string) []string {
	var args []string
	var currentArg strings.Builder
	var inQuote rune = 0
	var isEscaped bool = false

	for _, r := range strings.TrimSpace(command) {
		if isEscaped {
			currentArg.WriteRune(r)
			isEscaped = false
			continue
		}

		if r == '\\' {
			if inQuote == '\'' {
				currentArg.WriteRune(r)
			} else {
				isEscaped = true
			}
			continue
		}

		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
			} else {
				currentArg.WriteRune(r)
			}
			continue
		}

		if r == '\'' || r == '"' {
			inQuote = r
			continue
		}

		if r == ' ' || r == '\t' {
			if currentArg.Len() > 0 {
				args = append(args, currentArg.String())
				currentArg.Reset()
			}
			continue
		}

		currentArg.WriteRune(r)
	}

	if currentArg.Len() > 0 {
		args = append(args, currentArg.String())
	}

	return args
}

func main() {
	for {
		fmt.Fprint(os.Stdout, "$ ")
		command, err := bufio.NewReader(os.Stdin).ReadString('\n')
		commandWords := parseCommand(strings.TrimSpace(command))

		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
		}

		switch commandWords[0] {
		case "exit":
			exit(commandWords[1:])
		case "echo":
			echo(commandWords[1:])
		case "type":
			typeBuiltIn(commandWords[1:])
		case "pwd":
			pwd(commandWords[1:])
		case "cd":
			changeDirectory(commandWords[1:])
		default:
			executeExternalCommand(commandWords[0], commandWords[1:])
		}
	}
}
