package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lightninglabs/loop/fsm"
)

func main() {
	out := flag.String("out", "", "outfile")
	stateMachine := flag.String("fsm", "", "the swap state machine to parse")
	flag.Parse()

	if filepath.Ext(*out) != ".md" {
		fmt.Println("Wrong argument: out must be a .md file")
		os.Exit(1)
	}

	fp, err := filepath.Abs(*out)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	switch *stateMachine {
	case "example":
		exampleFSM := &fsm.ExampleFSM{}
		writeMermaidFile(fp, exampleFSM.GetStates())
	default:
		fmt.Println("Missing or wrong argument: fsm must be one of:")
		fmt.Println("\treservations")
		fmt.Println("\texample")
		os.Exit(1)
	}
}

func writeMermaidFile(filename string, states fsm.States) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	var b bytes.Buffer
	fmt.Fprint(&b, "```mermaid\nstateDiagram-v2\n")

	sortedStates := sortedKeys(states)
	for _, state := range sortedStates {
		edges := states[fsm.StateType(state)]
		// write state name
		if len(state) > 0 {
			fmt.Fprintf(&b, "%s\n", state)
		} else {
			state = "[*]"
		}
		// write transitions
		for edge, target := range edges.Transitions {
			fmt.Fprintf(&b, "%s --> %s: %s\n", state, target, edge)
		}

	}

	fmt.Fprint(&b, "```")
	f.Write(b.Bytes())

	return nil
}

func sortedKeys(m fsm.States) []string {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = string(k)
		i++
	}
	sort.Strings(keys)
	return keys
}
