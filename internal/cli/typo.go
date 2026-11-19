package cli

import (
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

const minTypoWordLength = 4

func probableTypo(root *cobra.Command, args []string) error {
	if len(args) == 0 || strings.ContainsFunc(args[0], unicode.IsSpace) {
		return nil
	}
	if len(args) == 1 {
		return usagef("%q is one word, so it reads as a mistyped command and not a prompt; run observe --help for the commands, or write the prompt as a sentence, or quote it with a second word to send it", text(args[0]))
	}
	if match, ok := nearestCommand(commandNames(root), strings.ToLower(args[0])); ok {
		return usagef("unknown command %q, did you mean %q? To send these words to the agent as a prompt, quote them as one argument", text(args[0]), match)
	}
	return nil
}

func nearestCommand(names []string, word string) (string, bool) {
	if len(word) < minTypoWordLength {
		return "", false
	}
	best, bestDistance := "", len(word)
	for _, name := range names {
		if distance := editDistance(name, word); distance < bestDistance {
			best, bestDistance = name, distance
		}
	}
	allowed := 1
	if len(word) > 6 {
		allowed = 2
	}
	return best, bestDistance <= allowed && bestDistance > 0
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}
