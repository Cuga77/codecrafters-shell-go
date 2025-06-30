package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/chzyer/readline"
)

var COMMANDS = []string{
	"echo",
	"exit",
	"type",
	"pwd",
	"cd",
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

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

func getWriter(filename string, append bool, defaultWriter io.Writer) (io.WriteCloser, error) {
	if filename != "" {
		flags := os.O_WRONLY | os.O_CREATE
		if append {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		return os.OpenFile(filename, flags, 0644)
	}
	return &nopCloser{defaultWriter}, nil
}

func handleEmptyStderrRedirect(stderrFile string, appendStderr bool) {
	if stderrFile != "" {
		writer, err := getWriter(stderrFile, appendStderr, os.Stderr)
		if err == nil {
			writer.Close()
		}
	}
}

func echo(args []string, stdoutFile string, stderrFile string, appendStdout bool, appendStderr bool) {
	handleEmptyStderrRedirect(stderrFile, appendStderr)

	writer, err := getWriter(stdoutFile, appendStdout, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
		return
	}
	defer writer.Close()

	fmt.Fprintln(writer, strings.Join(args, " "))
}

func pwd(args []string, stdoutFile string, stderrFile string, appendStdout bool, appendStderr bool) {
	handleEmptyStderrRedirect(stderrFile, appendStderr)

	writer, err := getWriter(stdoutFile, appendStdout, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
		return
	}
	defer writer.Close()

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		return
	}
	fmt.Fprintln(writer, dir)
}

func changeDirectory(args []string, stderrFile string, appendStderr bool) {
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

	errWriter, err := getWriter(stderrFile, appendStderr, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	defer errWriter.Close()

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

func typeBuiltIn(args []string, stdoutFile string, stderrFile string, appendStdout bool, appendStderr bool) {
	handleEmptyStderrRedirect(stderrFile, appendStderr)

	if len(args) == 0 {
		return
	}

	writer, err := getWriter(stdoutFile, appendStdout, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
		return
	}
	defer writer.Close()

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

func invalidCommand(programName string, stderrFile string, appendStderr bool) {
	errWriter, err := getWriter(stderrFile, appendStderr, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	defer errWriter.Close()
	fmt.Fprintf(errWriter, "%s: command not found\n", programName)
}

func executeExternalCommand(programName string, args []string, stdoutFile string, stderrFile string, appendStdout bool, appendStderr bool) {
	path, err := exec.LookPath(programName)
	if err != nil {
		invalidCommand(programName, stderrFile, appendStderr)
		return
	}

	cmd := &exec.Cmd{
		Path: path,
		Args: append([]string{programName}, args...),
	}

	stdoutWriter, err := getWriter(stdoutFile, appendStdout, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	defer stdoutWriter.Close()
	cmd.Stdout = stdoutWriter

	stderrWriter, err := getWriter(stderrFile, appendStderr, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating redirection file: %v\n", err)
		return
	}
	defer stderrWriter.Close()
	cmd.Stderr = stderrWriter

	cmd.Run()
}

func main() {
	completer := readline.NewPrefixCompleter(
		readline.PcItem("echo"),
		readline.PcItem("exit"),
		readline.PcItem("type"),
		readline.PcItem("pwd"),
		readline.PcItem("cd"),
	)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:       "$ ",
		AutoComplete: completer,
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		input, err := rl.Readline()
		if err == io.EOF || err == readline.ErrInterrupt {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
			continue
		}

		commandWords := parseCommand(input)
		if len(commandWords) == 0 {
			continue
		}

		var stdoutFile, stderrFile string
		var appendStdout, appendStderr bool
		cleanCommandWords := []string{}
		i := 0
		for i < len(commandWords) {
			word := commandWords[i]
			isRedirect := false

			if word == ">" || word == "1>" {
				if i+1 < len(commandWords) {
					stdoutFile = commandWords[i+1]
					appendStdout = false
					i += 2
					isRedirect = true
				}
			} else if word == ">>" || word == "1>>" {
				if i+1 < len(commandWords) {
					stdoutFile = commandWords[i+1]
					appendStdout = true
					i += 2
					isRedirect = true
				}
			} else if word == "2>" {
				if i+1 < len(commandWords) {
					stderrFile = commandWords[i+1]
					appendStderr = false
					i += 2
					isRedirect = true
				}
			} else if word == "2>>" {
				if i+1 < len(commandWords) {
					stderrFile = commandWords[i+1]
					appendStderr = true
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
				writer, err := getWriter(stdoutFile, appendStdout, os.Stdout)
				if err == nil {
					writer.Close()
				}
			}
			if stderrFile != "" {
				writer, err := getWriter(stderrFile, appendStderr, os.Stderr)
				if err == nil {
					writer.Close()
				}
			}
			continue
		}

		switch cleanCommandWords[0] {
		case "exit":
			exit(cleanCommandWords[1:])
		case "echo":
			echo(cleanCommandWords[1:], stdoutFile, stderrFile, appendStdout, appendStderr)
		case "pwd":
			pwd(cleanCommandWords[1:], stdoutFile, stderrFile, appendStdout, appendStderr)
		case "cd":
			changeDirectory(cleanCommandWords[1:], stderrFile, appendStderr)
		case "type":
			typeBuiltIn(cleanCommandWords[1:], stdoutFile, stderrFile, appendStdout, appendStderr)
		default:
			executeExternalCommand(cleanCommandWords[0], cleanCommandWords[1:], stdoutFile, stderrFile, appendStdout, appendStderr)
		}
	}
}
