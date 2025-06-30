package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
)

const (
	StateNormal = iota
	StateInSingleQuote
	StateInDoubleQuote
	StateEscape
	StateInDoubleQuoteEscape
)

const TERMINAL_BELL = "\x07"

func parse(line string) ([]string, error) {
	var parts []string
	var currentPart strings.Builder
	state := StateNormal
	line = strings.TrimSpace(line)

	for _, r := range line {
		switch state {
		case StateNormal:
			if r == '\'' {
				state = StateInSingleQuote
			} else if r == '"' {
				state = StateInDoubleQuote
			} else if r == '\\' {
				state = StateEscape
			} else if unicode.IsSpace(r) {
				if currentPart.Len() > 0 {
					parts = append(parts, currentPart.String())
					currentPart.Reset()
				}
			} else {
				currentPart.WriteRune(r)
			}
		case StateInSingleQuote:
			if r == '\'' {
				state = StateNormal
			} else {
				currentPart.WriteRune(r)
			}
		case StateInDoubleQuote:
			if r == '"' {
				state = StateNormal
			} else if r == '\\' {
				state = StateInDoubleQuoteEscape
			} else {
				currentPart.WriteRune(r)
			}
		case StateInDoubleQuoteEscape:
			if r == '"' || r == '\\' || r == '$' || r == '`' {
				currentPart.WriteRune(r)
			} else {
				currentPart.WriteRune('\\')
				currentPart.WriteRune(r)
			}
			state = StateInDoubleQuote
		case StateEscape:
			currentPart.WriteRune(r)
			state = StateNormal
		}
	}

	if currentPart.Len() > 0 {
		parts = append(parts, currentPart.String())
	}

	if state == StateInSingleQuote {
		return nil, errors.New("unclosed single quote")
	}
	if state == StateInDoubleQuote || state == StateInDoubleQuoteEscape {
		return nil, errors.New("unclosed double quote")
	}
	if state == StateEscape {
		return nil, errors.New("unterminated escape sequence")
	}
	return parts, nil
}

var builtin = map[string]bool{
	"exit": true, "echo": true, "type": true, "pwd": true, "cd": true,
}

type customCompleter struct {
	innerCompleter readline.AutoCompleter
	lastLine       []rune
}

func buildAllCommands() []readline.PrefixCompleterInterface {
	var items []readline.PrefixCompleterInterface
	commandSet := make(map[string]bool)

	for cmd := range builtin {
		commandSet[cmd] = true
	}

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}
			info, err := file.Info()
			if err != nil {
				continue
			}

			if info.Mode().Perm()&0111 != 0 {
				commandSet[file.Name()] = true
			}
		}
	}

	for cmd := range commandSet {
		items = append(items, readline.PcItem(cmd))
	}
	return items
}

func (c *customCompleter) Do(line []rune, pos int) ([][]rune, int) {
	suggestions, n := c.innerCompleter.Do(line, pos)

	if len(suggestions) == 0 {
		c.lastLine = nil
		fmt.Print(TERMINAL_BELL)
		return nil, 0
	}

	if len(suggestions) == 1 {
		c.lastLine = nil
		return suggestions, n
	}

	if equal(c.lastLine, line) {
		c.lastLine = nil
		fmt.Println()
		var stringMatches []string
		for _, r := range suggestions {
			stringMatches = append(stringMatches, string(r))
		}
		fmt.Println(strings.Join(stringMatches, "  "))
		return nil, 0
	} else {
		c.lastLine = make([]rune, len(line))
		copy(c.lastLine, line)
		fmt.Print(TERMINAL_BELL)
		return nil, 0
	}
}

func equal(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func main() {
	allCommandItems := buildAllCommands()
	prefixCompleter := readline.NewPrefixCompleter(allCommandItems...)
	completer := &customCompleter{
		innerCompleter: prefixCompleter,
	}
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "$ ",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		command, err := rl.Readline()

		if err == readline.ErrInterrupt {
			if command == "" {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}
		cleancommand := strings.TrimSpace(command)
		if cleancommand == "" {
			continue
		}
		parts, err := parse(cleancommand)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if len(parts) == 0 {
			continue
		}

		var outputFile string
		var appendFile string
		var errorFile string
		var appendErrorFile string
		var commandParts []string

		i := 0
		for i < len(parts) {
			part := parts[i]
			switch part {
			case ">", "1>":
				if i+1 < len(parts) {
					outputFile = parts[i+1]
					i += 2
				} else {
					i++
				}
			case ">>", "1>>":
				if i+1 < len(parts) {
					appendFile = parts[i+1]
					i += 2
				} else {
					i++
				}
			case "2>":
				if i+1 < len(parts) {
					errorFile = parts[i+1]
					i += 2
				} else {
					i++
				}
			case "2>>":
				if i+1 < len(parts) {
					appendErrorFile = parts[i+1]
					i += 2
				} else {
					i++
				}
			default:
				commandParts = append(commandParts, part)
				i++
			}
		}

		if len(commandParts) == 0 {
			continue
		}
		newcommand := commandParts[0]
		args := commandParts[1:]

		switch newcommand {
		case "exit":
			os.Exit(0)
		case "echo":
			textToPrint := strings.Join(args, " ") + "\n"

			if appendErrorFile != "" {
				file, err := os.OpenFile(appendErrorFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err == nil {
					file.Close()
				}
			} else if errorFile != "" {
				file, err := os.Create(errorFile)
				if err == nil {
					file.Close()
				}
			}

			if appendFile != "" {
				file, err := os.OpenFile(appendFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err == nil {
					file.WriteString(textToPrint)
					file.Close()
				}
			} else if outputFile != "" {
				os.WriteFile(outputFile, []byte(textToPrint), 0644)
			} else {
				fmt.Fprint(os.Stdout, textToPrint)
			}
			continue

		case "type":
			if len(args) == 0 {
				continue
			}
			if _, isbuiltin := builtin[args[0]]; isbuiltin {
				fmt.Printf("%s is a shell builtin\n", args[0])
			} else {
				if path, err := exec.LookPath(args[0]); err != nil {
					fmt.Printf("%s: not found\n", args[0])
				} else {
					fmt.Printf("%s is %s\n", args[0], path)
				}
			}
		case "pwd":
			if dir, err := os.Getwd(); err != nil {
				fmt.Fprintln(os.Stderr, err)
			} else {
				fmt.Println(dir)
			}
		case "cd":
			var targetdir string
			if len(args) == 0 || args[0] == "~" {
				if homeDir, err := os.UserHomeDir(); err != nil {
					fmt.Fprintf(os.Stderr, "cd: could not get home directory: %v\n", err)
					continue
				} else {
					targetdir = homeDir
				}
			} else {
				targetdir = args[0]
			}
			if err := os.Chdir(targetdir); err != nil {
				fmt.Printf("cd: %s: No such file or directory\n", targetdir)
			}
		default:
			path, err := exec.LookPath(newcommand)
			if err != nil {
				fmt.Printf("%s: command not found\n", newcommand)
				continue
			}

			cmd := &exec.Cmd{
				Path: path,
				Args: append([]string{newcommand}, args...),
			}

			if appendFile != "" {
				file, err := os.OpenFile(appendFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					fmt.Fprintln(os.Stderr, "error opening file:", err)
					continue
				}
				defer file.Close()
				cmd.Stdout = file
			} else if outputFile != "" {
				file, err := os.Create(outputFile)
				if err != nil {
					fmt.Fprintln(os.Stderr, "error creating file:", err)
					continue
				}
				defer file.Close()
				cmd.Stdout = file
			} else {
				cmd.Stdout = os.Stdout
			}

			if appendErrorFile != "" {
				file, err := os.OpenFile(appendErrorFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					fmt.Fprintln(os.Stderr, "error opening file:", err)
					continue
				}
				defer file.Close()
				cmd.Stderr = file
			} else if errorFile != "" {
				file, err := os.Create(errorFile)
				if err != nil {
					fmt.Fprintln(os.Stderr, "error creating file:", err)
					continue
				}
				defer file.Close()
				cmd.Stderr = file
			} else {
				cmd.Stderr = os.Stderr
			}

			cmd.Run()
		}
	}
}
