package editor

import (
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
)

// cmdDabbrevExpand expands the word before point to the nearest matching word
// known to the editor (M-/).  Repeated invocations cycle through candidates in
// this order:
//  1. Words in the current buffer, closest to point first.
//  2. Words in any other open buffer.
//  3. Registered command names.
func (e *Editor) cmdDabbrevExpand() {
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()

	// Find the word prefix immediately before point.
	prefixStart := pt
	for prefixStart > 0 && isWordRune(buf.RuneAt(prefixStart-1)) {
		prefixStart--
	}
	prefix := buf.Substring(prefixStart, pt)

	if prefix == "" {
		e.Message("No word before point")
		return
	}

	// If this is a fresh invocation (prefix changed or no candidates), build list.
	if e.dabbrevPrefix != prefix || e.dabbrevCandidates == nil {
		e.dabbrevPrefix = prefix
		e.dabbrevIdx = 0
		e.dabbrevCandidates = e.buildDabbrevCandidates(prefix, pt, buf)
		if len(e.dabbrevCandidates) == 0 {
			e.Message("No expansion for %q", prefix)
			return
		}
	} else if pt == e.dabbrevLastEnd {
		// Repeated invocation: undo previous expansion, restore prefix, cycle.
		prev := e.dabbrevCandidates[(e.dabbrevIdx-1+len(e.dabbrevCandidates))%len(e.dabbrevCandidates)]
		expandedLen := len([]rune(prev))
		start := pt - expandedLen
		buf.Delete(start, expandedLen)
		buf.InsertString(start, prefix)
		buf.SetPoint(start + len([]rune(prefix)))
		pt = buf.Point()
		prefixStart = pt - len([]rune(prefix))
	} else {
		// Point moved away — treat as fresh.
		e.dabbrevPrefix = prefix
		e.dabbrevIdx = 0
		e.dabbrevCandidates = e.buildDabbrevCandidates(prefix, pt, buf)
		if len(e.dabbrevCandidates) == 0 {
			e.Message("No expansion for %q", prefix)
			return
		}
	}

	candidate := e.dabbrevCandidates[e.dabbrevIdx]
	e.dabbrevIdx = (e.dabbrevIdx + 1) % len(e.dabbrevCandidates)

	// Replace prefix before point with the full candidate.
	buf.Delete(prefixStart, len([]rune(prefix)))
	buf.InsertString(prefixStart, candidate)
	newPt := prefixStart + len([]rune(candidate))
	buf.SetPoint(newPt)
	e.dabbrevLastEnd = newPt
	e.Message("Expanding: %s  (M-/ for next)", candidate)
}

// buildDabbrevCandidates collects expansion candidates for prefix.
func (e *Editor) buildDabbrevCandidates(prefix string, pt int, cur *buffer.Buffer) []string {
	seen := map[string]bool{strings.ToLower(prefix): true}
	var result []string

	addWords := func(words []string) {
		for _, w := range words {
			if !seen[strings.ToLower(w)] {
				seen[strings.ToLower(w)] = true
				result = append(result, w)
			}
		}
	}

	// 1. Current buffer, nearest to point first.
	addWords(dabbrevWordsInText(cur.String(), prefix, pt, true))

	// 2. Other open buffers.
	for _, b := range e.buffers {
		if b == cur {
			continue
		}
		addWords(dabbrevWordsInText(b.String(), prefix, -1, false))
	}

	// 3. Registered command names.
	lowerPre := strings.ToLower(prefix)
	for name := range commands {
		if strings.HasPrefix(name, lowerPre) && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	return result
}

// dabbrevWordsInText returns words from text that start with prefix
// (case-insensitive), excluding prefix itself.
// If nearestFirst is true, results are sorted by distance from pt.
func dabbrevWordsInText(text, prefix string, pt int, nearestFirst bool) []string {
	lowerPre := strings.ToLower(prefix)
	runes := []rune(text)
	n := len(runes)

	type hit struct {
		word string
		dist int
	}
	var hits []hit
	seen := map[string]bool{}

	i := 0
	for i < n {
		if !isWordRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < n && isWordRune(runes[j]) {
			j++
		}
		word := string(runes[i:j])
		lower := strings.ToLower(word)
		if strings.HasPrefix(lower, lowerPre) && lower != lowerPre && !seen[lower] {
			seen[lower] = true
			dist := i - pt
			if dist < 0 {
				dist = -dist
			}
			hits = append(hits, hit{word: word, dist: dist})
		}
		i = j
	}

	if nearestFirst && pt >= 0 {
		// Insertion sort by distance (small lists).
		for i := 1; i < len(hits); i++ {
			for j := i; j > 0 && hits[j].dist < hits[j-1].dist; j-- {
				hits[j], hits[j-1] = hits[j-1], hits[j]
			}
		}
	}

	result := make([]string, len(hits))
	for i, h := range hits {
		result[i] = h.word
	}
	return result
}
