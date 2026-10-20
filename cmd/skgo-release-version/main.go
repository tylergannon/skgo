// Command skgo-release-version validates and derives releases from Conventional Commits.
package main

import (
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
	// Nested plugin products have independent releases. Only a commit that
	// touches core inputs contributes to the root version, regardless of its
	// Conventional Commit type. Mixed commits still contribute normally.
	var messages []string
	for _, revision := range strings.Fields(git("rev-list", base+"..HEAD")) {
		paths := strings.Fields(git("diff-tree", "--root", "--no-commit-id", "--name-only", "-r", revision))
		if !releaseversion.TouchesCore(paths) {
			continue
		}
		messages = append(messages, git("show", "-s", "--format=%B", revision))
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
