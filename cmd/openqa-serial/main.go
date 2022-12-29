package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Declare ANSI color codes
const ANSI_RED = "\u001b[31m"
const ANSI_GREEN = "\u001b[32m"
const ANSI_YELLOW = "\u001b[33m"
const ANSI_BRIGHTYELLOW = "\u001b[33;1m"
const ANSI_BLUE = "\u001b[34m"
const ANSI_MAGENTA = "\u001b[35m"
const ANSI_CYAN = "\u001b[36m"
const ANSI_WHITE = "\u001b[37m"
const ANSI_RESET = "\u001b[0m"

const ANSI_ALT_SCREEN = "\x1b[?1049h"
const ANSI_EXIT_ALT_SCREEN = "\x1b[?1049l"

type Entry struct {
	Command    string
	Output     string
	token      string // Matching token to find
	Dumb       bool   // Dumb entry, i.e. a simple line that is not matched
	ReturnCode int
}

type Config struct {
	input   string // Input file or URL
	Numbers bool   // Display command numbers
	Colors  bool
}

var cf Config

func (cf *Config) SetDefaults() {
	cf.input = ""
	cf.Numbers = true
	cf.Colors = true
}

func readFile(filename string) (io.Reader, error) {
	content, err := os.ReadFile(filename)
	encoded := string(content)
	reader := strings.NewReader(encoded)
	return reader, err
}

// Clean leading ; and trim the token
func cleanToken(token string) string {
	for len(token) > 0 && strings.HasPrefix(token, ";") {
		token = token[1:]
	}
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "echo ") {
		token = token[5:]
	}
	token = strings.TrimSpace(token)
	return token
}

func returnCode(str string) (int, error) {
	n := len(str)
	if n < 3 {
		return 0, fmt.Errorf("invalid return code string")
	}

	if str[0] != '-' {
		return 0, fmt.Errorf("invalid return code prefix")
	}
	if str[n-1] != '-' {
		return 0, fmt.Errorf("invalid return code suffix")
	}
	rem := str[1 : n-1]
	return strconv.Atoi(rem)
}

func parse(reader io.Reader) ([]Entry, error) {
	var err error
	entries := make([]Entry, 0)
	scanner := bufio.NewScanner(reader)

	// Regex for script_run
	// echo Gi8yV-$?-
	re_sr := regexp.MustCompile(`;[ ]+echo [a-zA-Z0-9~_]{5}-\$\?-.*`)

	entry := Entry{}
	for scanner.Scan() {
		line := strings.Trim(scanner.Text(), "\n")

		// Are we searching for the matching token?
		if entry.token != "" && !strings.HasPrefix(line, ">") && !strings.Contains(line, "_EOT") && !strings.Contains(line, "EOT_") {
			i := strings.Index(line, entry.token)
			if i >= 0 {
				// Complete entry - Fetch return code
				rem := strings.TrimSpace(line[i+len(entry.token):])
				entry.ReturnCode, err = returnCode(rem)
				if err != nil {
					return entries, err
				}
				entries = append(entries, entry)
				entry = Entry{}
			} else {
				// Add to output
				if entry.Output == "" {
					entry.Output = line
				} else {
					entry.Output = fmt.Sprintf("%s\n%s", entry.Output, line)
				}
			}
		} else {
			// Beginning of new command?
			loc := re_sr.FindStringIndex(line)
			if loc != nil {
				match := line[loc[0] : loc[1]-4]
				entry.Command = strings.TrimSpace(line[:loc[0]])
				entry.token = cleanToken(match)
				entry.Dumb = false
			} else {
				entry.Command = line
				entry.Dumb = true
				entries = append(entries, entry)
				entry = Entry{}
			}
		}

	}

	return entries, scanner.Err()
}

func cleanFragment(url string) string {
	i := strings.Index(url, "#")
	if i > 0 {
		return url[:i]
	}
	return url
}

func parseProgramArguments() error {
	args := os.Args[1:]
	progname := os.Args[0]
	n := len(args)
	if n == 0 {
		return nil
	}

	for i := 0; i < n; i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg[0] == '-' {
			if arg == "-h" || arg == "--help" {
				fmt.Println("openqa-serial terminal reader")
				fmt.Println("  Small helper to make the serial terminal better readable")
				fmt.Println("")
				fmt.Printf("Usage: %s [OPTIONS] INPUT\n", progname)
				fmt.Println("OPTIONS")
				fmt.Println("   -h, --help                                Print help message")
				fmt.Println("   -n, --no-numbers                          Don't display command numbers")
				fmt.Println("")
				fmt.Println("INPUT supports links to openQA jobs and file names")
				fmt.Println("      You can just point to any openQA job or the serial_terminal.txt (or any other) asset file therein")
				fmt.Println("")
				fmt.Println("2022, phoenix - Have a lot of fun!")
				os.Exit(0)
			} else if arg == "-n" || arg == "--no-numbers" || arg == "--nonumbers" {
				cf.Numbers = false
			} else {
				return fmt.Errorf("illegal argument: %s", arg)
			}
		} else {
			if cf.input != "" {
				return fmt.Errorf("multiple inputs are not supported")
			}
			cf.input = arg
		}
	}

	return nil
}

func main() {
	var r io.Reader
	var err error

	cf.SetDefaults()

	// Parse input
	if err := parseProgramArguments(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	/**** Read input ****/
	if cf.input == "" {
		fmt.Fprintf(os.Stderr, "no input given\n")
		os.Exit(1)
	}
	// Is it a URL?
	if strings.Contains(cf.input, "://") {
		url := cleanFragment(cf.input)
		// Append filename if not yet there
		if !strings.Contains(url, "/file/") {
			url += "/file/serial_terminal.txt"
		}

		// Fetch content
		res, err := http.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "http error: %s\n", err)
			os.Exit(1)
		}
		if res.StatusCode != 200 {
			fmt.Fprintf(os.Stderr, "error: http status code %d\n", err)
			os.Exit(1)
		}
		r = res.Body
		//defer res.Body.Close()
	} else {
		// Assume it's a file
		r, err = readFile(cf.input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading: %s\n", err)
			os.Exit(1)
		}
	}

	entries, err := parse(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %s\n", err)
		os.Exit(2)
	}

	for i, entry := range entries {
		color := cf.Colors && !entry.Dumb
		if color {
			// Apply colors
			if entry.ReturnCode == 0 {
				fmt.Print(ANSI_GREEN)
			} else {
				fmt.Print(ANSI_RED)
			}
		}
		if cf.Numbers {
			fmt.Printf("%6d ", i)
		}
		fmt.Printf("%s", entry.Command)
		fmt.Println()

		// Print also output if not just a line
		if !entry.Dumb {
			for _, line := range strings.Split(entry.Output, "\n") {
				if cf.Numbers {
					fmt.Print("       ")
				}
				fmt.Printf("%s\n", line)
			}
		}
		if color {
			fmt.Print(ANSI_RESET)
		}
	}
}
