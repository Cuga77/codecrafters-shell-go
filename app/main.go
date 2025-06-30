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

func getOutputWriter(stdoutFile string) (io.WriteCloser, error) {
	if stdoutFile != "" {
		file, err := os.Create(stdoutFile)
		if err != nil {
			return nil, err
		}
		return file, nil
	}
	return os.Stdout, nil
}

func exit(args []string) {
	os.Exit(0)
}

func echo(args []string, stdoutFile string) {
	writer, err := getOutputWriter(stdoutFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	if writer != os.Stdout {
		defer writer.Close()
	}
	fmt.Fprintln(writer, strings.Join(args, " "))
}

func pwd(args []string, stdoutFile string) {
	writer, err := getOutputWriter(stdoutFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	if writer != os.Stdout {
		defer writer.Close()
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		return
	}
	fmt.Fprintln(writer, dir)
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

	writer, err := getOutputWriter(stdoutFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	if writer != os.Stdout {
		defer writer.Close()
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

func invalidCommand(programName string) {
	fmt.Printf("%s: command not found\n", programName)
}

func executeExternalCommand(programName string, args []string, stdoutFile string) {
	path, err := exec.LookPath(programName)
	if err != nil {
		invalidCommand(programName)
		return
	}

	cmd := &exec.Cmd{
		Path: path,
		Args: append([]string{programName}, args...),
	}

	cmd.Stderr = os.Stderr

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

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || !exitErr.Success() {
			// Command ran but returned a non-zero exit code.
			// The command's own stderr should have printed the message.
		}
	}
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

		stdoutFile := ""
		cleanCommandWords := []string{}
		isRedirecting := false

		for i, word := range commandWords {
			if word == ">" || word == "1>" {
				if i+1 < len(commandWords) {
					stdoutFile = commandWords[i+1]
					cleanCommandWords = commandWords[:i]
					isRedirecting = true
					break
				}
			}
		}

		if !isRedirecting {
			cleanCommandWords = commandWords
		}

		if len(cleanCommandWords) == 0 {
			if isRedirecting {
				os.Create(stdoutFile)
			}
			continue
		}

		switch cleanCommandWords[0] {
		case "exit":
			exit(cleanCommandWords[1:])
		case "echo":
			echo(cleanCommandWords[1:], stdoutFile)
		case "pwd":
			pwd(cleanCommandWords[1:], stdoutFile)
		case "cd":
			changeDirectory(cleanCommandWords[1:])
		case "type":
			typeBuiltIn(cleanCommandWords[1:], stdoutFile)
		default:
			executeExternalCommand(cleanCommandWords[0], cleanCommandWords[1:], stdoutFile)
		}
	}
}
