package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"voitts/internal/tts"
)

// reportWidth is the column the report wraps details at. It is a backstop for
// the pathological line — a list of search paths — not a target: a resolved
// path and its sample rate fit on one line.
const reportWidth = 90

// A result is one line of the preflight report.
type result struct {
	name   string // the piece being checked
	ok     bool
	detail string // what was found, or what went wrong
	hint   string // how to fix it; printed only on failure, may span lines
}

// preflight verifies everything the server needs at runtime — whatever the
// platform builds its virtual microphone out of, Piper for synthesis, and the
// voice it speaks with — and writes a report to w. The web UI is not among
// them: it is compiled into this binary by internal/web.
//
// The audio half differs by platform and comes from audioChecks, defined in
// check_linux.go and check_windows.go. So do the install hints for Piper and
// its voice, which name a package manager and a directory.
//
// piperCmd overrides Piper discovery, and is checked as given: the report has
// to answer whether the server is about to start, not whether it would have
// started with different arguments.
//
// It returns an error naming the unmet dependencies if any check failed. The
// checks are read-only: nothing is loaded, started or downloaded here.
func preflight(ctx context.Context, w io.Writer) error {
	results := append(audioChecks(ctx), checkPiper(""), checkVoice())

	report(w, results)

	var missing []string
	for _, r := range results {
		if !r.ok {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("unmet dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// checkPiper resolves the synthesizer the same way tts.New does, which for the
// Python module means importing it rather than trusting the file to be there.
// An empty override selects the same discovery the server would do unaided.
func checkPiper(override string) result {
	r := result{name: "piper", hint: piperHint()}

	argv, err := tts.LookupCmd(override)
	if err != nil {
		// The lookup error names every interpreter and directory it tried,
		// which is the useful half of the answer: an install that landed
		// somewhere unsearched looks identical to no install at all.
		r.detail = oneLine(err.Error())
		return r
	}

	// Details are read, not pasted, so the path is shortened where the platform
	// has a shorthand worth using. shortenHome is where that is decided.
	argv[0] = shortenHome(argv[0])
	r.ok, r.detail = true, strings.Join(argv, " ")
	return r
}

// checkVoice resolves the voice the server runs with. Both the model and its
// companion .onnx.json are required — the sample rate lives in the config, and
// the whole pipeline is configured from it.
func checkVoice() result {
	r := result{name: "voice", hint: voiceHint()}

	path, rate, err := tts.LookupVoice(tts.DefaultModel)
	if err != nil {
		// The lookup error carries its own download line; the headline is the
		// part that says what is missing and where it was looked for.
		r.detail = oneLine(err.Error())
		return r
	}

	r.ok = true
	r.detail = fmt.Sprintf("%s (%d Hz)", shortenHome(path), rate)
	return r
}

// report renders the checks as an aligned block, each failure followed by its
// hint indented under the detail column.
func report(w io.Writer, results []result) {
	width := 0
	for _, r := range results {
		width = max(width, len(r.name))
	}
	indent := strings.Repeat(" ", width+6)

	fmt.Fprintln(w)
	for _, r := range results {
		mark := "x"
		if r.ok {
			mark = "+"
		}

		detail := wrap(r.detail, reportWidth-len(indent))
		fmt.Fprintf(w, "  %s %-*s  %s\n", mark, width, r.name, detail[0])
		for _, line := range detail[1:] {
			fmt.Fprintln(w, indent+line)
		}

		if r.ok || r.hint == "" {
			continue
		}
		// Hints are commands to copy, so they are printed as written. Only the
		// gutter marks them, and only on the first line of the block.
		for i, line := range strings.Split(r.hint, "\n") {
			gutter := indent
			if i == 0 {
				gutter = fmt.Sprintf("%*s", len(indent), "fix: ")
			}
			fmt.Fprintln(w, gutter+line)
		}
	}
	fmt.Fprintln(w)
}

// wrap breaks text into lines of at most width columns, at spaces. A word too
// long to fit — a search path, usually — takes a line of its own rather than
// being cut. The result always has at least one line.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}

// oneLine reduces messages to their headline, keeping the report one line per
// check. Later arguments are fallbacks for when the earlier ones are blank.
func oneLine(messages ...string) string {
	for _, msg := range messages {
		if head, _, _ := strings.Cut(strings.TrimSpace(msg), "\n"); head != "" {
			return head
		}
	}
	return ""
}
