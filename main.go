package main

import (
	"fmt"
	"flag"
	"io/ioutil"
	"encoding/json"
	"bytes"
	"strings"
)

type Description struct {
	States []*State
	// The name of the state machine which will also be used as the name of the
	// generated type.
	Name string

	// Package name
	Package string

	// The initial state
	Init string
}

type State struct {
	// Name of the state
	Name string

	// Function to execute whenever a transition to this state happens.
	On   string

	// State transitions
	Transitions []*Transition
}

type Transition struct {
	// Name of the event.
	Event string

	// State to transition to should the specified event occur
	To    string

	// Function to execute before the state is switched to the target state.
	Action string
}

func main() {
	inputFile := flag.String("in", "desc.json", "Path to input file.")
	outputFile := flag.String("out", "statemachine.go", "Path to output file.")
	pkg := flag.String("package","main", "Package name (used in the `package ...` statement).")

	flag.Parse()

	data, err := ioutil.ReadFile(*inputFile)

	if err != nil {
		panic(err.Error())
	}

	var desc *Description
	err = json.Unmarshal(data, &desc)

	if err != nil {
		panic(err.Error())
	}

	buf := new(bytes.Buffer)

	compile(desc, *pkg, buf)

	err = ioutil.WriteFile(*outputFile, buf.Bytes(), 0644)

	if err != nil {
		panic(err.Error())
	}
}

func writef(buf *bytes.Buffer, fmtstr string, args ...interface{}) {
	buf.WriteString(fmt.Sprintf(fmtstr, args...))
}

func camel(str string) string {
	first := str[0]

	return strings.ToUpper(string(first)) + str[1:]
}

func compile(desc *Description, pkg string, buf *bytes.Buffer) {
	writef(buf, "// Code generated by genstatem; DO NOT EDIT.\n\n")
	writef(buf, "package %s\n\n", pkg)
	writef(buf, "import \"fmt\"\n")
	writef(buf, "import \"errors\"\n")
	writef(buf, "import \"sync\"\n\n")
	writef(buf, "type Event string\n")
	writef(buf, "type State string\n")
	writef(buf, "type %s struct {\n", desc.Name)
	writef(buf, "\tstate State\n")
	writef(buf, "\tmu *sync.RWMutex\n")
	writef(buf, "}\n\n")
	writef(buf, "func (sm *%s) State() State {\n", desc.Name)
	writef(buf, "\tsm.mu.RLock()\n")
	writef(buf, "\tdefer sm.mu.RUnlock()\n\n")
	writef(buf, "\treturn sm.state\n}\n\n")

	statesMap := make(map[string]*State)
	eventsMap := make(map[string]bool)

	for _, state := range desc.States {
		it := statesMap[state.Name]

		if it != nil {
			panic(fmt.Sprintf("Duplicate state: %s", state.Name))
		}

		statesMap[state.Name] = state

		transitionMap := make(map[string]bool)

		for _, transition := range state.Transitions {
			it := transitionMap[transition.Event]

			if it {
				panic(fmt.Sprintf("Can't have two transitions for the same event. Duplicate event: %s.", transition.Event))
			}

			transitionMap[transition.Event] = true

			eventsMap[transition.Event] = true
		}
	}

	for _, state := range statesMap {
		writef(buf, "const State%s = %q\n", camel(state.Name), state.Name)
	}

	for k, _ := range eventsMap {
		writef(buf, "const Event%s = %q\n", camel(k), k)
	}

	writef(buf, "\n\n")

	writef(buf, "func (sm *%s) Event(event Event) error {\n", desc.Name)
	writef(buf, "\tsm.mu.Lock()\n")
	writef(buf, "\tdefer sm.mu.Unlock()\n\n")
	writef(buf, "\tswitch sm.state {\n")

	for _, state := range statesMap {
		writef(buf, "\tcase %q:\n", state.Name)
		
		writef(buf, "\t\tswitch event {\n")

		for _, transition := range state.Transitions {

			targetState := statesMap[transition.To]

			if targetState == nil {
				panic(fmt.Sprintf("Target state in transition from state `%s` to `%s` on event `%s` does not exist.", 
					state.Name, transition.To, transition.Event))
			}

			writef(buf, "\t\tcase %q:\n", transition.Event)
			writef(buf, "\t\t\t%s(event, sm.state)\n", transition.Action)
			writef(buf, "\t\t\tsm.state = %q\n", transition.To)

			// Does the target state have an on?
			if targetState.On != "" {
				writef(buf, "\t\t\t%s(event, sm.state)\n", targetState.On)
			}
		}

		writef(buf, "\t\tdefault:\n")
		writef(buf, "\t\t\treturn errors.New(%s)\n", "fmt.Sprintf(\"Event `%s` is not valid during state `%s`, event, sm.state\")")

		writef(buf, "\t\t}\n")
	}

	writef(buf, "\t}\n\n")
	writef(buf, "\treturn nil\n")
	writef(buf, "}\n\n")

	writef(buf, "func New%s() *%s{\n", desc.Name, desc.Name)
	writef(buf, "\treturn &%s{state:%q, mu: &sync.RWMutex{}}\n", desc.Name, desc.Init)
	writef(buf, "}\n\n")
}