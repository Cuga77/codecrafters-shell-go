package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
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
func getstdin(cmdIndex int, pipes [][]int) uintptr {
	if cmdIndex == 0 {
		return uintptr(syscall.Stdin)
	}
	return uintptr(pipes[cmdIndex-1][0])
}
func getstdout(cmdIndex int, pipes [][]int) uintptr {
	if cmdIndex == len(pipes) {
		return uintptr(syscall.Stdout)
	}
	return uintptr(pipes[cmdIndex][1])
}
func splitCommand(parts []string) [][]string {
	var commands [][]string
	var currentcommand []string

	for _, part := range parts {
		if part == "|" {
			if len(currentcommand) > 0 {
				commands = append(commands, currentcommand)
				currentcommand = nil
			}

		} else {
			currentcommand = append(currentcommand, part)
		}
	}
	if len(currentcommand) > 0 {
		commands = append(commands, currentcommand)
	}

	return commands
}
func executePipeline(commands [][]string) error {
	if len(commands) == 0 {
		return nil
	}

	pipes := make([][]int, len(commands)-1)
	for i := 0; i < len(commands)-1; i++ {
		pipe := make([]int, 2)
		if err := syscall.Pipe(pipe); err != nil {
			return err
		}
		pipes[i] = pipe
	}

	var pids []int

	for i, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}

		cmdPath, err := exec.LookPath(cmd[0])
		if err != nil {
			return fmt.Errorf("command not found: %s", cmd[0])
		}

		pid, err := syscall.ForkExec(cmdPath, cmd, &syscall.ProcAttr{
			Files: []uintptr{
				getstdin(i, pipes),
				getstdout(i, pipes),
				uintptr(syscall.Stderr),
			},
		})
		if err != nil {
			return err
		}
		pids = append(pids, pid)

		if i > 0 {
			syscall.Close(pipes[i-1][0])
		}

		if i < len(pipes) {
			syscall.Close(pipes[i][1])
		}
	}

	for _, pipe := range pipes {
		syscall.Close(pipe[0])
		syscall.Close(pipe[1])
	}

	for _, pid := range pids {
		var status syscall.WaitStatus
		syscall.Wait4(pid, &status, 0, nil)
	}

	return nil
}

var builtin = map[string]bool{
	"exit": true, "echo": true, "type": true, "pwd": true, "cd": true,
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	prefix := strs[0]
	for i := 1; i < len(strs); i++ {
		for len(prefix) > 0 && !strings.HasPrefix(strs[i], prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		if prefix == "" {
			break
		}
	}
	return prefix
}

type customCompleter struct {
	innerCompleter readline.AutoCompleter
	lastPrefix     string
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
	var commandNames []string
	for cmd := range commandSet {
		commandNames = append(commandNames, cmd)
	}
	sort.Strings(commandNames)
	for _, cmd := range commandNames {
		items = append(items, readline.PcItem(cmd))
	}
	return items

}
func (c *customCompleter) Do(line []rune, pos int) ([][]rune, int) {
	currentPrefix := string(line[:pos])
	suggestions, n := c.innerCompleter.Do(line, pos)

	if len(suggestions) == 0 {
		c.lastPrefix = ""
		fmt.Print(TERMINAL_BELL)
		return nil, 0
	}

	if len(suggestions) == 1 {
		c.lastPrefix = ""
		return suggestions, n
	}
	if len(suggestions) > 1 {
		var fullSuggestions []string
		for _, suggestion := range suggestions {
			fullSuggestions = append(fullSuggestions, currentPrefix+string(suggestion))
		}

		commonPrefix := longestCommonPrefix(fullSuggestions)

		if len(commonPrefix) > len(currentPrefix) {
			c.lastPrefix = ""
			completion := commonPrefix[len(currentPrefix):]
			return [][]rune{[]rune(completion)}, n
		}

		if currentPrefix == c.lastPrefix {

			c.lastPrefix = ""
			sort.Strings(fullSuggestions)

			fmt.Print("\n")
			for i, completion := range fullSuggestions {
				if i > 0 {
					fmt.Print(" ")
				}
				fmt.Print(completion)
			}
			fmt.Printf("\n$ %s", currentPrefix)

			return nil, 0
		} else {
			c.lastPrefix = currentPrefix
			fmt.Print(TERMINAL_BELL)
			return nil, 0
		}
	}

	return nil, 0

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
		commands := splitCommand(parts)

		if len(commands) > 1 {

			if err := executePipeline(commands); err != nil {
				fmt.Fprintln(os.Stderr, "pipeline error:", err)
			}
		} else {
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
				cmd := exec.Command(newcommand, args...)

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
				if err := cmd.Run(); err != nil {
					if _, ok := err.(*exec.Error); ok {
						fmt.Printf("%s: command not found\n", newcommand)
					}
				}
			}
		}
	}
}
