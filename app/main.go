package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
)

// Ensures gofmt doesn't remove the "fmt" import in stage 1 (feel free to remove this!)
var _ = fmt.Fprint
var tabCounter int

// CustomCompleter implements readline.AutoCompleter
type CustomCompleter struct {
	words []string

	tabSuggestion []string
	prefix        string
}

func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	sort.Strings(strs)
	//fmt.Println(strs)
	first := strs[0]
	last := strs[len(strs)-1]
	minLen := min(len(first), len(last))
	for i := 0; i < minLen; i++ {
		if first[i] != last[i] {
			return first[:i]
		}
	}
	return first[:minLen]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Do is the core method for the completer. It's called by readline when the user presses Tab.
// Its job is to return a list of suggestions.
func (c *CustomCompleter) Do(line []rune, pos int) (newLine [][]rune, offset int) {
	prefix := string(line[:pos])

	var suggestions []string
	for _, word := range c.words {
		if strings.HasPrefix(word, prefix) {
			suggestions = append(suggestions, word)
		}
	}

	if len(suggestions) == 0 {
		fmt.Print("\a")
		return nil, 0
	}

	if len(suggestions) == 1 {
		completion := suggestions[0][len(prefix):] + " "
		tabCounter = 0
		return [][]rune{[]rune(completion)}, len(completion)
	}

	// Multiple suggestions
	common := commonPrefix(suggestions)
	// fmt.Println("")
	// fmt.Println(common)
	// fmt.Println("")
	if len(common) > len(prefix) {
		// Partial autocomplete to common prefix
		completion := common[len(prefix):]
		tabCounter = 0
		c.prefix = common
		return [][]rune{[]rune(completion)}, len(completion)
	}

	// If prefix unchanged, show suggestions on second tab
	if prefix == c.prefix {
		tabCounter++
	} else {
		tabCounter = 1
		c.prefix = prefix
		c.tabSuggestion = suggestions
	}

	if tabCounter == 2 {
		fmt.Print("\n" + strings.Join(suggestions, "  ") + "\n")
		return [][]rune{[]rune("")}, len(prefix)
	}

	fmt.Print("\a")
	return nil, 0
}

func extractCommand(dir string) ([]string, error) {
	c, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0)
	for _, entry := range c {
		res = append(res, entry.Name())
	}
	return res, err
}
func ReadDir(PATH string) []string {
	path_split := strings.Split(PATH, ":")
	res := make([]string, 0)
	for _, dir := range path_split {
		executable, _ := extractCommand(dir)
		res = append(res, executable...)
	}
	return res
}

func fileExists(filepath string) bool {
	_, err := os.Stat(filepath)
	return !os.IsNotExist(err)
}

var specialRunes = map[rune]struct{}{
	'\\': {},
	'$':  {},
	'"':  {},
	'\n': {},
}

func isSpecialInDoubleQuotes(r rune) bool {
	_, ok := specialRunes[r]
	return ok
}

type fileHandle struct {
	fileName       string
	fileappendmode bool
}

func handleShellInput(input string) (string, []string, fileHandle, fileHandle) {
	var result []string
	var buffer strings.Builder
	var stack []rune
	escaped := false
	var Stdoutfile fileHandle
	var Stderrfile fileHandle
	var mode string
	//fmt.Println(input)

	key := 0
	for key < len(input) {

		if len(stack) == 0 && !escaped {
			switch {
			case strings.HasPrefix(input[key:], "1>>"):
				mode = "stdout_append"
				key += 3
				continue
			case strings.HasPrefix(input[key:], "2>>"):
				mode = "stderr_append"
				key += 3
				continue
			case strings.HasPrefix(input[key:], "1>"):
				mode = "stdout"
				key += 2
				continue
			case strings.HasPrefix(input[key:], "2>"):
				mode = "stderr"
				key += 2
				continue
			case strings.HasPrefix(input[key:], ">>"):
				mode = "stdout_append"
				key += 2
				continue
			case input[key] == '>':
				mode = "stdout"
				key++
				continue
			}
		}

		r := rune(input[key])
		switch {
		case escaped:
			buffer.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = false
			if len(stack) == 0 || (len(stack) > 0 && stack[len(stack)-1] == rune('"') && isSpecialInDoubleQuotes(rune(input[key+1]))) {
				escaped = true
			} else {
				buffer.WriteRune(r)
			}
		case r == '\'' || r == '"':
			if len(stack) > 0 && stack[len(stack)-1] == r {
				stack = stack[:len(stack)-1] // pop
			} else if len(stack) > 0 && stack[len(stack)-1] != r {
				buffer.WriteRune(r)
			} else {
				stack = append(stack, r) // push
			}
			// Don't write quotes to buffer
		case unicode.IsSpace(r) && len(stack) == 0:
			if buffer.Len() > 0 {
				token := buffer.String()
				switch mode {
				case "stdout":
					Stdoutfile.fileName = token
					Stdoutfile.fileappendmode = false
				case "stdout_append":
					Stdoutfile.fileName = token
					Stdoutfile.fileappendmode = true
				case "stderr":
					Stderrfile.fileName = token
					Stderrfile.fileappendmode = false
				case "stderr_append":
					Stderrfile.fileName = token
					Stderrfile.fileappendmode = true
				default:
					result = append(result, buffer.String())
				}
				mode = "" // reset after use
				buffer.Reset()
			}

		default:
			buffer.WriteRune(r)
		}
		key++
	}

	if buffer.Len() > 0 {
		token := buffer.String()
		switch mode {
		case "stdout":
			Stdoutfile.fileName = token
			Stdoutfile.fileappendmode = false
		case "stdout_append":
			Stdoutfile.fileName = token
			Stdoutfile.fileappendmode = true
		case "stderr":
			Stderrfile.fileName = token
			Stderrfile.fileappendmode = false
		case "stderr_append":
			Stderrfile.fileName = token
			Stderrfile.fileappendmode = true
		default:
			result = append(result, buffer.String())
		}
	}
	// fmt.Println(result)
	// fmt.Println(Stdoutfile)
	// fmt.Println("we11111")
	// fmt.Println(Stderrfile)
	return result[0], result[1:], Stdoutfile, Stderrfile
}

func fileWriteAppend(input string, stdFile fileHandle) {
	file, err := os.OpenFile(stdFile.fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		//fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	_, err = file.WriteString(input)
	if err != nil {
		//fmt.Println("Error writing to file:", err)
		return
	}

}

func writeOutput(input string, stdFile fileHandle, newline bool) {
	var bufferOutput strings.Builder
	if newline {
		bufferOutput.WriteString(input + "\n")
	} else {
		bufferOutput.WriteString(input)
	}

	if stdFile.fileName != "" && stdFile.fileappendmode == true {
		fileWriteAppend(bufferOutput.String(), stdFile)
	} else if stdFile.fileName != "" && stdFile.fileappendmode == false {
		err := os.WriteFile(stdFile.fileName, []byte(bufferOutput.String()), 0644)
		if err != nil {
			return
		}
	} else {
		fmt.Print(bufferOutput.String())
	}
}

func createFile(filename string) {
	if filename != "" && !fileExists(filename) {

		dir := filepath.Dir(filename)

		// Create the directory if it doesn't exist
		if err := os.MkdirAll(dir, 0755); err != nil {
			//fmt.Println("Failed to create directory:", err)
			return
		}
		err := os.WriteFile(filename, []byte(""), 0644)
		if err != nil {
			log.Println("Error writing file:", err)
		} else {
			//log.Println("Saved file:", filename)
		}
	}
}

func makeUniqueSort(res []string) []string {
	seen := make(map[string]bool)
	var uniqueSlice []string

	for _, val := range res {
		if _, exists := seen[val]; !exists {
			seen[val] = true
			uniqueSlice = append(uniqueSlice, val)
		}
	}

	// Sort the unique slice
	sort.Strings(uniqueSlice)
	return uniqueSlice
}

type CustomListener struct{}

type customlistner interface {
	OnChange(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool)
}

func (l *CustomListener) OnChange(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
	if key != 9 {
		tabCounter = 0
	}
	return nil, 0, false
}

func parsePipeline(input string) [][]string {
	var commands [][]string
	for _, cmdStr := range strings.Split(input, "|") {
		cmdStr = strings.TrimSpace(cmdStr)
		if cmdStr == "" {
			continue
		}
		cmd, args, _, _ := handleShellInput(cmdStr)
		if cmd != "" {
			commands = append(commands, append([]string{cmd}, args...))
		}
	}
	return commands
}

const (
	EXIT    = "exit"
	ECHO    = "echo"
	TYPE    = "type"
	PWD     = "pwd"
	CD      = "cd"
	HISTORY = "history"
)

var builtins = map[string]struct{}{
	EXIT: {}, ECHO: {}, TYPE: {}, PWD: {}, CD: {},
}

func isBuiltin(cmd string) bool {
	_, ok := builtins[cmd]
	return ok
}

func executePipeline(input string) {
	commands := parsePipeline(input)
	if len(commands) == 0 {
		return
	}

	type pipeCmd struct {
		cmd  string
		args []string
	}

	var pipes []struct {
		r *io.PipeReader
		w *io.PipeWriter
	}
	for i := 0; i < len(commands)-1; i++ {
		r, w := io.Pipe()
		pipes = append(pipes, struct {
			r *io.PipeReader
			w *io.PipeWriter
		}{r, w})
	}

	var procs []func() error

	for i, cmdArgs := range commands {
		cmdName := cmdArgs[0]
		args := cmdArgs[1:]

		var stdin io.Reader = nil
		var stdout io.Writer = nil

		if i > 0 {
			stdin = pipes[i-1].r
		} else {
			stdin = os.Stdin
		}
		if i < len(commands)-1 {
			stdout = pipes[i].w
		} else {
			stdout = os.Stdout
		}

		if isBuiltin(cmdName) {
			// Built-in as pipeline participant
			captureCmd := cmdName
			captureArgs := args
			// captureStdin := stdin

			captureStdout := stdout
			procs = append(procs, func() error {
				// For built-ins, only ECHO, PWD, TYPE are supported in pipeline
				switch captureCmd {
				case ECHO:
					fmt.Fprintln(captureStdout, strings.Join(captureArgs, " "))
				case PWD:
					dir, _ := os.Getwd()
					fmt.Fprintln(captureStdout, dir)
				case TYPE:
					if len(captureArgs) < 1 {
						fmt.Fprintln(captureStdout, "usage: type <command>")
						return nil
					}
					targetCmd := captureArgs[0]
					if _, ok := builtins[targetCmd]; ok {
						fmt.Fprintf(captureStdout, "%s is a shell builtin\n", targetCmd)
						return nil
					}
					path := os.Getenv("PATH")
					found := false
					for _, dir := range strings.Split(path, ":") {
						fullPath := filepath.Join(dir, targetCmd)
						if fileInfo, statErr := os.Stat(fullPath); statErr == nil {
							mode := fileInfo.Mode()
							if !mode.IsDir() && (mode&0111 != 0) {
								fmt.Fprintf(captureStdout, "%s is %s\n", targetCmd, fullPath)
								found = true
								break
							}
						}
					}
					if !found {
						fmt.Fprintf(captureStdout, "%s: not found\n", targetCmd)
					}
				default:
					fmt.Fprintf(captureStdout, "%s: not supported in pipeline\n", captureCmd)
				}
				return nil
			})
		} else {
			// External command
			captureCmd := cmdName
			captureArgs := args
			captureStdin := stdin
			captureStdout := stdout
			procs = append(procs, func() error {
				cmd := exec.Command(captureCmd, captureArgs...)
				cmd.Stdin = captureStdin
				cmd.Stdout = captureStdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			})
		}
	}

	// Launch all procs
	var errs = make(chan error, len(procs))
	for i, proc := range procs {
		go func(i int, proc func() error) {
			err := proc()
			// Close write ends of pipes after writing
			if i < len(pipes) {
				pipes[i].w.Close()
			}
			if i > 0 {
				pipes[i-1].r.Close()
			}
			errs <- err
		}(i, proc)
	}

	// Wait for all
	for range procs {
		<-errs
	}
}

type historyEntry struct {
	cmd     string
	written bool
}

type historylist struct {
	cmdlist []historyEntry
}

func (h *historylist) push(cmd string, written bool) {
	h.cmdlist = append(h.cmdlist, historyEntry{
		cmd:     strings.TrimSpace(cmd),
		written: written,
	})
}

func (h *historylist) list() []historyEntry {
	return h.cmdlist
}

func newhistorylist() *historylist {
	return &historylist{}
}

func readHistory(filename string, historycmd *historylist, written bool) {
	file, err := os.Open(filename)
	if err != nil {
		//fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Create a new Scanner for the file
	scanner := bufio.NewScanner(file)

	// Iterate over each line
	for scanner.Scan() {
		line := scanner.Text() // Get the current line as a string
		historycmd.push(line, written)
	}

	// Check for errors during the scan
	if err := scanner.Err(); err != nil {
		log.Fatalf("error during scan: %s", err)
	}
}

func writeHistory(filename string, historycmd *historylist, fileappendmode bool) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		//fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Create a new Scanner for the file
	writer := bufio.NewWriter(file)
	for _, historycmd := range historycmd.list() {
		if historycmd.written == false {
			_, err := writer.WriteString(historycmd.cmd + "\n")
			if err != nil {
				log.Fatalf("failed writing to file: %s", err)
			}
		}
	}

	if fileappendmode == true {
		historycmd.cmdlist = nil
	}
	// Flush the buffer to ensure all data is written to the file
	err = writer.Flush()
	if err != nil {
		log.Fatalf("failed flushing buffer: %s", err)
	}
}

func main() {
	// Uncomment this block to pass the first stage

	path := os.Getenv("PATH")
	//path := "/tmp/foo"
	path_split := strings.Split(path, ":")
	execCmd := ReadDir(path)

	completer := &CustomCompleter{
		words: []string{"echo", "exit"},
	}

	var listener customlistner = &CustomListener{}

	completer.words = append(completer.words, execCmd...)
	completer.words = makeUniqueSort(completer.words)

	l, err := readline.NewEx(&readline.Config{
		Prompt:       "$ ",
		AutoComplete: completer,
		Listener:     listener,
	})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	var foundCommand bool
	var stderr bytes.Buffer
	var stdout bytes.Buffer

	var historycmd = newhistorylist()

	if os.Getenv("HISTFILE") != "" {
		readHistory(os.Getenv("HISTFILE"), historycmd, true)
	}

	// Wait for user input
	for {
		//fmt.Fprint(os.Stdout, "$ ")
		cmd, err := l.Readline()
		//cmd, err := bufio.NewReader(os.Stdin).ReadString('\n')
		//fmt.Println(len(cmd))
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
			os.Exit(1)
		}

		cmd = strings.TrimSuffix(cmd, "\n")

		if strings.Contains(cmd, "|") {
			executePipeline(cmd)
			continue
		}

		//command_parts := strings.Split(strings.TrimSpace(cmd), " ")
		//command := command_parts[0]
		//args := command_parts[1:]
		//fmt.Println(args)
		command, args, Stdoutfile, Stderrfile := handleShellInput(cmd)

		tabCounter = 0
		completer.prefix = ""
		completer.tabSuggestion = nil

		createFile(Stdoutfile.fileName)
		createFile(Stderrfile.fileName)

		historycmd.push(command+" "+strings.Join(args, " "), false)

		availableCommand := [4]string{"echo", "exit", "type", "history"}

		switch command {
		case ECHO:
			writeOutput(strings.Join(args, " "), Stdoutfile, true)
		case "exit":
			exit_code, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				os.Exit(1)
			}
			if os.Getenv("HISTFILE") != "" {
				writeHistory(os.Getenv("HISTFILE"), historycmd, true)
			}
			os.Exit(int(exit_code))
		case HISTORY:
			limit_history := 0
			if len(args) > 0 {
				if len(args) == 1 {
					var err error
					var parsed int64
					parsed, err = strconv.ParseInt(args[0], 10, 64)
					if err != nil {
						// handle error
					}
					limit_history = int(parsed)
				} else if len(args) == 2 && args[0] == "-r" {
					readHistory(args[1], historycmd, true)
					continue
				} else if len(args) == 2 && (args[0] == "-w" || args[0] == "-a") {
					fileappendmode := false
					if args[0] == "-a" {
						fileappendmode = true
					}
					writeHistory(args[1], historycmd, fileappendmode)
					continue
				}

			}
			var historylist = historycmd.list()
			lenhistory := len(historylist)
			var skipcnt = 0
			if limit_history > 0 {
				skipcnt = lenhistory - limit_history
			}
			for index, historycmd := range historylist {
				if (index + 1) <= skipcnt {
					continue
				}
				temp := strconv.Itoa(index + 1)
				writeOutput(temp+" "+historycmd.cmd, Stdoutfile, true)
			}
		case TYPE:
			slice := availableCommand[:] // Convert array to slice
			target := args[0]
			if slices.Contains(slice, target) {
				writeOutput(target+" is a shell builtin", Stdoutfile, true)
			} else {
				foundCommand = false
				if len(path) > 0 {
					for _, exec_path := range path_split {
						if fileExists(exec_path + "/" + target) {
							writeOutput(target+" is "+exec_path+"/"+target, Stdoutfile, true)
							foundCommand = true
							break
						}
					}
					if !foundCommand {
						writeOutput(target+": not found", Stdoutfile, true)
					}
				} else {
					writeOutput(target+": not found", Stdoutfile, true)
				}
			}
		default:
			if len(path) > 0 {
				foundCommand = false
				for _, exec_path := range path_split {
					if fileExists(exec_path + "/" + command) {
						foundCommand = true
						exec_cmd := exec.Command(command, args[0:]...)
						exec_cmd.Stderr = &stderr
						exec_cmd.Stdout = &stdout
						if err := exec_cmd.Run(); err != nil {
							writeOutput(stderr.String(), Stderrfile, false)
							stderr.Reset()
						}
						if stdout.String() != "" {
							writeOutput(stdout.String(), Stdoutfile, false)
							stdout.Reset()
						}
						break
					}
				}
			}
			if !foundCommand {
				writeOutput(command+": command not found", Stdoutfile, true)
			}

		}
	}
}
