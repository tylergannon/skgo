// Command skgo-release-version validates and derives releases from Conventional Commits.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tylergannon/skgo/internal/releaseversion"
)

func main() {
	check := flag.String("check", "", "validate the subject of this git revision")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	if *check != "" {
		subject := git("show", "-s", "--format=%s", *check)
		if err := releaseversion.ValidateSubject(subject); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(subject)
		return
	}

	base := git("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*")
	raw := gitBytes("log", "--format=%B%x00", base+"..HEAD")
	parts := bytes.Split(bytes.TrimRight(raw, "\x00\n"), []byte{0})
	messages := make([]string, 0, len(parts))
	for _, part := range parts {
		if message := strings.TrimSpace(string(part)); message != "" {
			messages = append(messages, message)
		}
	}
	next, err := releaseversion.Next(base, releaseversion.Analyze(messages))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if next == "" {
		fmt.Println("none")
		return
	}
	fmt.Println(next)
}

func git(args ...string) string {
	return strings.TrimSpace(string(gitBytes(args...)))
}

func gitBytes(args ...string) []byte {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git %s: %v\n%s", strings.Join(args, " "), err, out)
		os.Exit(1)
	}
	return out
}
