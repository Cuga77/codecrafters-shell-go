package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/chzyer/readline"
)

const typeFound string = " is a shell builtin"

var shellBuiltIn []string = []string{"echo", "exit", "type", "pwd", "cd", "history"}
var escapeOptionsDoubleQuoted []rune = []rune{'\\', '$', '"', ' '}
var escapeOptionUnquoted []rune = []rune{'\\', '$', '"', ' ', '\''}
var history []string = []string{}

// the AutoCompleter interface requires one method
// Do(line []rune, pos int) (newLine [][]rune, length int)
// AutoComplete in the readline.Config struct is of type AutoCompleter
// so we need to give our TabAutoCompleter a Do method with this
// signature, instantiate the TabAutoCompleter and pass it as the autocompleter
type TabAutoCompleter struct {
	Commands  []string
	Path      string
	TabCount  int
	LastInput string
}

func (tac *TabAutoCompleter) Do(line []rune, pos int) ([][]rune, int) {
	input := string(line[:pos])

	autoCompleteResults := make([][]rune, 0)
	executableResults := getExecutables(tac.Path, input)
	for _, cmd := range tac.Commands {
		if strings.HasPrefix(cmd, input) {
			autoCompleteResults = append(autoCompleteResults, []rune(cmd[pos:]+" "))
		}
	}
	for _, cmdExec := range executableResults {
		autoCompleteResults = append(autoCompleteResults, []rune(cmdExec[pos:]+" "))
	}
	if len(autoCompleteResults) == 0 {
		fmt.Fprint(os.Stdout, "\x07")
		return nil, pos
	}
	sort.Slice(autoCompleteResults, func(i, j int) bool {
		return string(autoCompleteResults[i]) < string(autoCompleteResults[j])
	})
	if len(executableResults) == 1 {
		return [][]rune{[]rune(executableResults[0][pos:] + " ")}, pos
	}
	if len(executableResults) == 0 && len(autoCompleteResults) >= 1 {
		return autoCompleteResults, pos
	}
	if len(executableResults) > 1 {
		autoCompleteStrings := make([]string, 0)
		shortestMatch := findShortestString(autoCompleteResults)
		hasSharedPrefix := haveSharedPrefix(shortestMatch, autoCompleteResults)
		if hasSharedPrefix {
			return [][]rune{[]rune(shortestMatch)}, pos
		} else {
			if tac.TabCount == 0 {
				fmt.Fprint(os.Stdout, "\a")
				tac.TabCount++
				tac.LastInput = input
				return nil, pos
			} else {
				for _, match := range executableResults {
					autoCompleteStrings = append(autoCompleteStrings, match)
				}
				sort.Slice(autoCompleteStrings, func(i, j int) bool {
					return string(autoCompleteStrings[i]) < string(autoCompleteStrings[j])
				})
				fmt.Println()
				fmt.Println(strings.Join(autoCompleteStrings, "  "))
				fmt.Printf("$ %s", input)
				tac.TabCount++
			}
		}

	}
	return nil, pos
}
func findShortestString(autoCompleteResults [][]rune) string {
	shortestLength := 100000
	shortestCandidate := ""
	for _, result := range autoCompleteResults {
		if len(result) < shortestLength {
			shortestLength = len(result)
			shortestCandidate = string(result)
		}
	}
	return strings.Trim(shortestCandidate, " ")
}
func haveSharedPrefix(shortestMatch string, autoCompleteResults [][]rune) bool {
	for _, runeSliceRes := range autoCompleteResults {
		stringSliceRes := string(runeSliceRes)
		if !strings.HasPrefix(stringSliceRes, shortestMatch) {
			return false
		}
	}
	return true
}
func main() {
	PATH := os.Getenv("PATH")
	completer := &TabAutoCompleter{
		Commands: shellBuiltIn,
		Path:     PATH,
		TabCount: 0,
	}
	l, err := readline.NewEx(&readline.Config{
		Prompt:       "$ ",
		AutoComplete: completer,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	_, err = fmt.Fprint(os.Stdout, "$ ")
	for {
		command, err := l.Readline()
		if err != nil {
			log.Println("Error reading string from standard in " + err.Error())
		}
		if strings.Contains(command, "|") {
			pipedCommands := separatePipedCommands(command)
			pipedCommandProccesor(pipedCommands, PATH)
		} else {
			commandProcessor(command, PATH)
		}
		history = append(history, command)
		completer.TabCount = 0
		completer.LastInput = ""
		fmt.Fprint(os.Stdout, "$ ")
	}
}
func separatePipedCommands(input string) []string {
	inDoubleQuotes := false
	inSingleQuotes := false
	pipeParts := make([]string, 0)
	currCommand := ""
	for i := range input {
		switch input[i] {
		case '|':
			if !inDoubleQuotes && !inSingleQuotes {
				pipeParts = append(pipeParts, strings.TrimSpace(currCommand))
				currCommand = ""
			} else {
				currCommand += string(input[i])
			}
		case '"':
			inDoubleQuotes = !inDoubleQuotes
			currCommand += string('"')
		case '\'':
			inSingleQuotes = !inSingleQuotes
			currCommand += string('\'')
		default:
			currCommand += string(input[i])
		}
	}
	if currCommand != "" {
		pipeParts = append(pipeParts, strings.TrimSpace(currCommand))
	}
	return pipeParts
}
func pipedCommandProccesor(pipedCommands []string, PATH string) {
	var cmds []*exec.Cmd
	var readers []*io.PipeReader
	var writers []*io.PipeWriter
	var wg sync.WaitGroup
	//directories := strings.Split(PATH, ":")
	outputFilePath := ""
	errFilePath := ""
	outputAppendFilePath := ""
	errFileAppendFilePath := ""

	var prevInputPipeReader *io.PipeReader
	// for potential  redirects in the last command in the pipe
	outputWriter := os.Stdout
	errWriter := os.Stderr // Changed to os.Stderr for proper error stream

	for i, cmd := range pipedCommands {
		if i == len(pipedCommands)-1 {
			outputFilePath, errFilePath, outputAppendFilePath, errFileAppendFilePath = parseOutputRedirect(cmd)
			// remove redirection so this is not interpreted as a command argument
			removedRedirect := removeRedirection(cmd)
			cmd = removedRedirect
			var err error
			if outputFilePath != "" {
				os.MkdirAll(filepath.Dir(outputFilePath), 0755)
				outputWriter, err = os.OpenFile(outputFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			}
			if errFilePath != "" {
				os.MkdirAll(filepath.Dir(errFilePath), 0755)
				errWriter, err = os.OpenFile(errFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			}
			if outputAppendFilePath != "" {
				os.MkdirAll(filepath.Dir(outputAppendFilePath), 0755)
				outputWriter, err = os.OpenFile(outputAppendFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
			}
			if errFileAppendFilePath != "" {
				os.MkdirAll(filepath.Dir(errFileAppendFilePath), 0755)
				errWriter, err = os.OpenFile(errFileAppendFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
			}
			if err != nil {
				fmt.Println("Error creating out/err writer: " + err.Error())
			}
		}
		cmd = strings.TrimSpace(cmd)
		cmdName, cmdArgs := parseCommandArgs(cmd)
		if slices.Contains(shellBuiltIn, cmdName) {
			var r io.Reader
			var w io.Writer
			// create pipe reader/writer for reading and writing output
			if i < len(pipedCommands)-1 {
				r, w = io.Pipe()
			} else {
				r = nil
				w = outputWriter // Use final output writer
			}
			// create a goroutine to simulate built-in command execution
			wg.Add(1)
			go func(cmdName string, in io.Reader, out, errPipe io.Writer, passedCmdArgs []string) {
				defer wg.Done()
				if pipeWriter, ok := out.(*io.PipeWriter); ok {
					defer pipeWriter.Close()
				}
				var inputBytes []byte
				var cmdArgs []string
				var input string
				if r, ok := in.(*io.PipeReader); ok && r != nil {
					if cmdName != "type" {
						inputBytes, _ = io.ReadAll(r)
						input = string(inputBytes)
						cmdArgs = strings.Fields(input)
					} else {
						io.Copy(io.Discard, in)
						cmdArgs = passedCmdArgs
						input = strings.Join(cmdArgs, " ")
					}
				} else {
					cmdArgs = passedCmdArgs
					input = strings.Join(cmdArgs, " ")
				}
				directories := strings.Split(PATH, ":")
				shellBuiltInHandler(cmdName, input, out, errPipe, directories, cmdArgs)
			}(cmdName, prevInputPipeReader, w, errWriter, cmdArgs)
			if pipeReader, ok := r.(*io.PipeReader); ok && r != nil {
				prevInputPipeReader = pipeReader
			} else {
				prevInputPipeReader = nil
			}
			continue
		}

		// FIX: Added PATH search for external commands in pipes
		var pathToExecutable string
		directories := strings.Split(PATH, ":")
		for _, dir := range directories {
			p, _ := checkForExecutable(dir, cmdName)
			if p != "" {
				pathToExecutable = p
				break
			}
		}

		var cmdExec *exec.Cmd
		if pathToExecutable != "" {
			cmdExec = exec.Command(pathToExecutable, cmdArgs...)
		} else {
			// Let OS handle the error for command not found
			cmdExec = exec.Command(cmdName, cmdArgs...)
		}

		if prevInputPipeReader != nil {
			cmdExec.Stdin = prevInputPipeReader
		} else {
			cmdExec.Stdin = os.Stdin
		}

		if i < len(pipedCommands)-1 {
			reader, writer := io.Pipe()
			cmdExec.Stdout = writer
			cmdExec.Stderr = writer // Also pipe stderr

			prevInputPipeReader = reader
			readers = append(readers, reader)
			writers = append(writers, writer)

		} else { // Last command
			cmdExec.Stdout = outputWriter
			cmdExec.Stderr = errWriter
		}
		cmds = append(cmds, cmdExec)
	}
	// Start all of the commands we have collected in cmds
	for _, cmd := range cmds {
		err := cmd.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting command %v: %v\n", cmd.String(), err)
		}
	}
	for i, cmd := range cmds {
		err := cmd.Wait()
		if err != nil {
			// Silently ignore wait errors, as they often mean the command failed,
			// which is expected behavior (e.g., grep not finding matches).
			// The error message from the command itself would have been piped.
		}
		if i < len(writers) {
			writers[i].Close()
		}
	}
	wg.Wait()
}
func commandProcessor(input, PATH string) {
	directories := strings.Split(PATH, ":")
	// default stdOut and stdErr output locations
	outputFilePath := ""
	errFilePath := ""
	outputAppendFilePath := ""
	errFileAppendFilePath := ""
	outputWriter := os.Stdout
	errWriter := os.Stderr // Changed to os.Stderr for proper error stream

	// create an argParts without the redirection symbol
	outputFilePath, errFilePath, outputAppendFilePath, errFileAppendFilePath = parseOutputRedirect(input)

	// remove redirection so this is not interpreted as a command argument
	removedRedirect := removeRedirection(input)
	cmdParsed, argsParts := parseCommandArgs(removedRedirect)
	if cmdParsed == "" {
		return // Handle empty input
	}

	commandName := cmdParsed
	argsString := strings.Join(argsParts, " ")
	var err error
	if outputFilePath != "" {
		os.MkdirAll(filepath.Dir(outputFilePath), 0755)
		outputWriter, err = os.OpenFile(outputFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	}
	if errFilePath != "" {
		os.MkdirAll(filepath.Dir(errFilePath), 0755)
		errWriter, err = os.OpenFile(errFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	}
	if outputAppendFilePath != "" {
		os.MkdirAll(filepath.Dir(outputAppendFilePath), 0755)
		outputWriter, err = os.OpenFile(outputAppendFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	}
	if errFileAppendFilePath != "" {
		os.MkdirAll(filepath.Dir(errFileAppendFilePath), 0755)
		errWriter, err = os.OpenFile(errFileAppendFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	}
	if err != nil {
		fmt.Println("Error creating out/err writer: " + err.Error())
	}
	if outputWriter != os.Stdout {
		defer outputWriter.Close()
	}
	if errWriter != os.Stderr {
		defer errWriter.Close()
	}
	if slices.Contains(shellBuiltIn, commandName) {
		shellBuiltInHandler(commandName, argsString, outputWriter, errWriter, directories, argsParts)
	} else {
		// FIX: Using a standard loop and correcting the exec.Command call
		var pathToExecutable string
		for i := 0; i < len(directories); i++ {
			p, _ := checkForExecutable(directories[i], commandName)
			if p != "" {
				pathToExecutable = p
				break
			}
		}

		if pathToExecutable != "" {
			cmd := exec.Command(pathToExecutable, argsParts...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = outputWriter
			cmd.Stderr = errWriter
			err := cmd.Run()
			if err != nil {
				// Error is already written to stderr by the command
			}
			return
		}

		fmt.Fprintln(errWriter, commandName+": command not found")
		return
	}
}
func checkForExecutable(path, command string) (string, error) {
	// An empty path in PATH should be ignored
	if path == "" {
		return "", nil
	}
	c, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	for _, entry := range c {
		if entry.Name() == command {
			// It's good practice to ensure it's not a directory
			if !entry.IsDir() {
				return filepath.Join(path, entry.Name()), nil
			}
		}
	}
	return "", nil
}
func checkForExecutableSuffix(path, input string) ([]string, error) {
	c, err := os.ReadDir(path)
	res := make([]string, 0)
	if err != nil {
		return nil, err
	}
	for _, entry := range c {
		if strings.HasPrefix(entry.Name(), input) {
			res = append(res, entry.Name())
		}
	}
	return res, nil
}
func getExecutables(PATH string, input string) []string {
	directories := strings.Split(PATH, ":")
	res := make([]string, 0)
	// FIX: Using a standard loop
	for i := 0; i < len(directories); i++ {
		pathsToExecutables, _ := checkForExecutableSuffix(directories[i], input)
		res = append(res, pathsToExecutables...)
	}
	return res
}

func parseCommandArgs(input string) (string, []string) {
	commandArgString := strings.TrimSpace(input)
	if commandArgString == "" {
		return "", []string{}
	}
	args := []string{}
	var token strings.Builder
	escapeChar := false
	inDoubleQuotes := false
	inSingleQuotes := false
	for i := 0; i < len(commandArgString); i++ {
		char := commandArgString[i]
		switch {
		case inSingleQuotes:
			if char == '\'' {
				inSingleQuotes = false
			} else {
				token.WriteByte(char)
			}
		case escapeChar:
			var escapeOptions []rune
			if inDoubleQuotes {
				escapeOptions = escapeOptionsDoubleQuoted
			} else {
				escapeOptions = escapeOptionUnquoted
			}
			if slices.Contains(escapeOptions, rune(char)) {
				token.WriteByte(char)
			} else {
				token.WriteByte('\\')
				token.WriteByte(char)
			}
			escapeChar = false
		case char == '\\':
			escapeChar = true
		case char == '"':
			inDoubleQuotes = !inDoubleQuotes
		case char == '\'':
			inSingleQuotes = !inSingleQuotes
		case char == ' ' || char == '\t':
			if inDoubleQuotes || inSingleQuotes {
				token.WriteByte(char)
			} else {
				if token.Len() > 0 {
					args = append(args, token.String())
					token.Reset()
				}
			}
		default:
			token.WriteByte(char)
		}
	}
	if token.Len() > 0 {
		args = append(args, token.String())
	}
	if len(args) == 0 {
		return "", []string{}
	}
	commandName := args[0]
	return commandName, args[1:]
}

func shellBuiltInHandler(commandName, argsString string, outputWriter, errWriter io.Writer, directories, argsParts []string) {
	switch commandName {
	case "exit":
		if len(argsParts) > 0 && argsParts[0] == "0" {
			os.Exit(0)
		}

	case "echo":
		fmt.Fprintln(outputWriter, argsString)
		return

	case "type":
		if len(argsParts) == 0 {
			// No arguments, do nothing or print error
			return
		}
		typeArg := argsParts[0] // Only consider the first argument for type
		if slices.Contains(shellBuiltIn, typeArg) {
			fmt.Fprintln(outputWriter, typeArg+typeFound)
			return
		}
		// FIX: Using a standard loop
		for i := 0; i < len(directories); i++ {
			pathToExecutable, _ := checkForExecutable(directories[i], typeArg)
			if pathToExecutable != "" {
				fmt.Fprintln(outputWriter, typeArg+" is "+pathToExecutable)
				return
			}
		}
		fmt.Fprintln(errWriter, typeArg+": not found")
		return

	case "pwd":
		if len(argsParts) > 0 {
			fmt.Fprintln(errWriter, "pwd: too many arguments")
			return
		}
		workingDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(errWriter, "Error running command: "+err.Error())
			return
		}
		fmt.Fprintln(outputWriter, workingDir)
		return

	case "cd":
		if len(argsParts) != 1 {
			fmt.Fprintln(errWriter, "cd: wrong number of arguments")
			return
		}
		cdPath := argsParts[0]
		if cdPath == "~" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintln(errWriter, "cd: cannot find home directory: "+err.Error())
				return
			}
			cdPath = homeDir
		} else if strings.HasPrefix(cdPath, "~/") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintln(errWriter, "cd: cannot find home directory: "+err.Error())
				return
			}
			cdPath = filepath.Join(homeDir, cdPath[2:])
		}
		err := os.Chdir(cdPath)
		if err != nil {
			fmt.Fprintln(errWriter, "cd: "+cdPath+": No such file or directory")
			return
		}
	case "history":
		// The current command is added in main loop, so we don't add it here
		for i, cmd := range history {
			fmt.Fprintf(outputWriter, "  %d  %s\n", i, cmd)
		}
	}
}

func parseOutputRedirect(input string) (string, string, string, string) {
	stdOutRedirectPattern := `(?:^|\s)1?>(?:\s*"([^"]+)"|\s*'([^']+)'|\s*([^\s>]+))`
	stdOutAppendPattern := `(?:^|\s)1?>>(?:\s*"([^"]+)"|\s*'([^']+)'|\s*([^\s>]+))`
	stdErrRedirectPattern := `(?:^|\s)2>(?:\s*"([^"]+)"|\s*'([^']+)'|\s*([^\s>]+))`
	stdErrAppendPattern := `(?:^|\s)2>>(?:\s*"([^"]+)"|\s*'([^']+)'|\s*([^\s>]+))`

	stdOutReg := regexp.MustCompile(stdOutRedirectPattern)
	stdErrReg := regexp.MustCompile(stdErrRedirectPattern)

	stdOutAppendReg := regexp.MustCompile(stdOutAppendPattern)
	stdErrAppendReg := regexp.MustCompile(stdErrAppendPattern)

	stdOutMatch := stdOutReg.FindStringSubmatch(input)
	stdErrMatch := stdErrReg.FindStringSubmatch(input)

	stdOutAppendMatch := stdOutAppendReg.FindStringSubmatch(input)
	stdErrAppendMatch := stdErrAppendReg.FindStringSubmatch(input)

	stdOutRes := ""
	stdErrRes := ""
	stdOutAppendRes := ""
	stdErrAppendRes := ""
	if len(stdOutMatch) > 1 {
		stdOutRes = stdOutMatch[1] + stdOutMatch[2] + stdOutMatch[3]
	}
	if len(stdErrMatch) > 1 {
		stdErrRes = stdErrMatch[1] + stdErrMatch[2] + stdErrMatch[3]
	}
	if len(stdOutAppendMatch) > 1 {
		stdOutAppendRes = stdOutAppendMatch[1] + stdOutAppendMatch[2] + stdOutAppendMatch[3]
	}
	if len(stdErrAppendMatch) > 1 {
		stdErrAppendRes = stdErrAppendMatch[1] + stdErrAppendMatch[2] + stdErrAppendMatch[3]
	}
	return stdOutRes, stdErrRes, stdOutAppendRes, stdErrAppendRes

}

func removeRedirection(input string) string {
	stdOutRedirectPattern := `\s*1?>?>\s*(?:"[^"]*"|'[^']*'|[^\s]+)`
	stdErrRedirectPattern := `\s*2>>?\s*(?:"[^"]*"|'[^']*'|[^\s]+)`

	res := regexp.MustCompile(stdErrRedirectPattern).ReplaceAllString(input, "")
	res = regexp.MustCompile(stdOutRedirectPattern).ReplaceAllString(res, "")

	return res
}
