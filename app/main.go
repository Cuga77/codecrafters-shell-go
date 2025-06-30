package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var COMMANDS = []string{
	"echo",
	"exit",
	"type",
	"pwd",
	"cd",
}

func parseCommand(command string) []string {
	var args []string
	var currentArg strings.Builder
	runes := []rune(strings.TrimSpace(command))
	i := 0
	inQuote := rune(0)

	for i < len(runes) {
		r := runes[i]

		if inQuote == '\'' {
			if r == '\'' {
				inQuote = 0
			} else {
				currentArg.WriteRune(r)
			}
		} else if inQuote == '"' {
			if r == '"' {
				inQuote = 0
			} else if r == '\\' {
				i++
				if i < len(runes) {
					nextChar := runes[i]
					if nextChar == '"' || nextChar == '\\' || nextChar == '$' {
						currentArg.WriteRune(nextChar)
					} else {
						currentArg.WriteRune('\\')
						currentArg.WriteRune(nextChar)
					}
				} else {
					currentArg.WriteRune('\\')
				}
			} else {
				currentArg.WriteRune(r)
			}
		} else {
			if r == ' ' || r == '\t' {
				if currentArg.Len() > 0 {
					args = append(args, currentArg.String())
					currentArg.Reset()
				}
			} else if r == '\'' || r == '"' {
				inQuote = r
			} else if r == '\\' {
				i++
				if i < len(runes) {
					currentArg.WriteRune(runes[i])
				}
			} else {
				currentArg.WriteRune(r)
			}
		}
		i++
	}

	if currentArg.Len() > 0 {
		args = append(args, currentArg.String())
	}
	return args
}

func exit(args []string) {
	os.Exit(0)
}

func echo(args []string, stdoutFile string, stderrFile string) {
	var writer io.Writer = os.Stdout
	var fileToClose *os.File

	if stderrFile != "" {
		file, err := os.Create(stderrFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
			return
		}
		writer = file
		fileToClose = file
	} else if stdoutFile != "" {
		file, err := os.Create(stdoutFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
			return
		}
		writer = file
		fileToClose = file
	}

	if fileToClose != nil {
		defer fileToClose.Close()
	}

	fmt.Fprintln(writer, strings.Join(args, " "))
}

func pwd(args []string, stdoutFile string) {
	var writer io.Writer = os.Stdout
	if stdoutFile != "" {
		file, err := os.Create(stdoutFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
			return
		}
		defer file.Close()
		writer = file
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		return
	}
	fmt.Fprintln(writer, dir)
}

func changeDirectory(args []string, stderrFile string) {
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

	var errWriter io.Writer = os.Stderr
	if stderrFile != "" {
		file, err := os.Create(stderrFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
			return
		}
		defer file.Close()
		errWriter = file
	}

	err = os.Chdir(targetDir)
	if err != nil {
		fmt.Fprintf(errWriter, "cd: %s: No such file or directory\n", targetDir)
	}
}

func isCommandInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func typeBuiltIn(args []string, stdoutFile string) {
	if len(args) == 0 {
		return
	}

	var writer io.Writer = os.Stdout
	if stdoutFile != "" {
		file, err := os.Create(stdoutFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
			return
		}
		defer file.Close()
		writer = file
	}

	command := args[0]
	if isCommandInSlice(command, COMMANDS) {
		fmt.Fprintf(writer, "%s is a shell builtin\n", command)
		return
	}

	path, err := exec.LookPath(command)
	if err != nil {
		fmt.Fprintf(writer, "%s: not found\n", command)
	} else {
		fmt.Fprintf(writer, "%s is %s\n", command, path)
	}
}

func invalidCommand(programName string, stderrFile string) {
	var errWriter io.Writer = os.Stderr
	if stderrFile != "" {
		file, err := os.Create(stderrFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
			return
		}
		defer file.Close()
		errWriter = file
	}
	fmt.Fprintf(errWriter, "%s: command not found\n", programName)
}

func executeExternalCommand(programName string, args []string, stdoutFile string, stderrFile string) {
	path, err := exec.LookPath(programName)
	if err != nil {
		invalidCommand(programName, stderrFile)
		return
	}

	cmd := exec.Command(path, args...)

	if stdoutFile != "" {
		file, err := os.Create(stdoutFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
			return
		}
		defer file.Close()
		cmd.Stdout = file
	} else {
		cmd.Stdout = os.Stdout
	}

	if stderrFile != "" {
		file, err := os.Create(stderrFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
			return
		}
		defer file.Close()
		cmd.Stderr = file
	} else {
		cmd.Stderr = os.Stderr
	}

	cmd.Run()
}

func main() {
	for {
		fmt.Fprint(os.Stdout, "$ ")
		input, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
			return
		}

		commandWords := parseCommand(input)
		if len(commandWords) == 0 {
			continue
		}

		var stdoutFile, stderrFile string
		cleanCommandWords := []string{}
		i := 0
		for i < len(commandWords) {
			word := commandWords[i]
			isRedirect := false

			if word == ">" || word == "1>" {
				if i+1 < len(commandWords) {
					stdoutFile = commandWords[i+1]
					i += 2
					isRedirect = true
				}
			} else if word == "2>" {
				if i+1 < len(commandWords) {
					stderrFile = commandWords[i+1]
					i += 2
					isRedirect = true
				}
			}

			if !isRedirect {
				cleanCommandWords = append(cleanCommandWords, word)
				i++
			}
		}

		if len(cleanCommandWords) == 0 {
			if stdoutFile != "" {
				os.Create(stdoutFile)
			}
			if stderrFile != "" {
				os.Create(stderrFile)
			}
			continue
		}

		switch cleanCommandWords[0] {
		case "exit":
			exit(cleanCommandWords[1:])
		case "echo":
			echo(cleanCommandWords[1:], stdoutFile, stderrFile)
		case "pwd":
			pwd(cleanCommandWords[1:], stdoutFile)
		case "cd":
			changeDirectory(cleanCommandWords[1:], stderrFile)
		case "type":
			typeBuiltIn(cleanCommandWords[1:], stdoutFile)
		default:
			executeExternalCommand(cleanCommandWords[0], cleanCommandWords[1:], stdoutFile, stderrFile)
		}
	}
}
