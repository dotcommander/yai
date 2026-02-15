package cmd

import (
	"fmt"
	"os"
	"runtime/pprof"
)

// memprofile writes heap and alloc profiles to the current directory.
// It's intended for local debugging and is hidden from help.
var memprofile bool

func maybeWriteMemProfile() {
	if !memprofile {
		return
	}

	heap, err := os.Create("yai_heap.profile")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer func() { _ = heap.Close() }()

	allocs, err := os.Create("yai_allocs.profile")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer func() { _ = allocs.Close() }()

	if err := pprof.Lookup("heap").WriteTo(heap, 0); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := pprof.Lookup("allocs").WriteTo(allocs, 0); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}
