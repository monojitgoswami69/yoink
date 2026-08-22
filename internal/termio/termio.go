// Package termio provides terminal input helpers (masked password input, basic line input).
package termio

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ReadLine reads a single line of input from stdin (no masking).
func ReadLine() string {
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// ReadSecret reads a line without echoing characters. Falls back to plain
// reading when stdin is not a terminal (e.g. piped input in CI).
func ReadSecret() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ReadLine(), nil
	}
	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}
