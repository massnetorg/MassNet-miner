package engine_test

import (
	"testing"

	"massnet.org/mass/poc/engine"
)

func TestWorkSpaceState(t *testing.T) {
	tests := []struct {
		state   engine.WorkSpaceState
		str     string
		isValid bool
		flag    engine.WorkSpaceStateFlags
	}{
		{
			state:   engine.Registered,
			str:     "registered",
			isValid: true,
			flag:    engine.SFRegistered,
		},
		{
			state:   engine.Plotting,
			str:     "plotting",
			isValid: true,
			flag:    engine.SFPlotting,
		},
		{
			state:   engine.Ready,
			str:     "ready",
			isValid: true,
			flag:    engine.SFReady,
		},
		{
			state:   engine.Mining,
			str:     "mining",
			isValid: true,
			flag:    engine.SFMining,
		},
		{
			state:   engine.FirstState,
			str:     "registered",
			isValid: true,
			flag:    engine.SFRegistered,
		},
		{
			state:   engine.LastState,
			str:     "mining",
			isValid: true,
			flag:    engine.SFMining,
		},
		{
			state:   engine.LastState + 1,
			str:     "invalid(4)",
			isValid: false,
			flag:    engine.LastStateFlags << 1,
		},
		{
			state:   engine.LastState + 2,
			str:     "invalid(5)",
			isValid: false,
			flag:    engine.LastStateFlags << 2,
		},
	}

	for i, test := range tests {
		if str := test.state.String(); str != test.str {
			t.Errorf("%d, WorkSpaceState String not matched, expected %s, got %s", i, test.str, str)
		}
		if isValid := test.state.IsValid(); isValid != test.isValid {
			t.Errorf("%d, WorkSpaceState IsValid not matched, expected %v, got %v", i, test.isValid, isValid)
		}
		if flag := test.state.Flag(); flag != test.flag {
			t.Errorf("%d, WorkSpaceState Flag not matched, expected %v, got %v", i, test.flag, flag)
		}
	}
}

func TestWorkSpaceStateFlags_IsNone(t *testing.T) {
	tests := []struct {
		flags  engine.WorkSpaceStateFlags
		isNone bool
	}{
		{
			flags:  engine.SFAll,
			isNone: false,
		},
		{
			flags:  engine.SFMining,
			isNone: false,
		},
		{
			flags:  engine.FirstStateFlags,
			isNone: false,
		},
		{
			flags:  engine.LastStateFlags,
			isNone: false,
		},
		{
			flags:  engine.SFRegistered | engine.SFPlotting | engine.SFReady | engine.SFMining,
			isNone: false,
		},
		{
			flags:  engine.SFAll + 1,
			isNone: true,
		},
		{
			flags:  engine.SFAll | engine.SFRegistered | engine.SFPlotting | engine.SFReady | engine.SFMining + 1,
			isNone: true,
		},
		{
			flags:  engine.SFAll<<1 + 1,
			isNone: false,
		},
		{
			flags:  engine.WorkSpaceStateFlags(0),
			isNone: true,
		},
	}

	for i, test := range tests {
		if isNone := test.flags.IsNone(); isNone != test.isNone {
			t.Errorf("%d, WorkSpaceStateFlags IsNone not matched, expected %v, got %v", i, test.isNone, isNone)
		}
	}
}

func TestWorkSpaceStateFlags_Contains(t *testing.T) {
	tests := []struct {
		flags    engine.WorkSpaceStateFlags
		flag     engine.WorkSpaceStateFlags
		contains bool
	}{
		{
			flags:    engine.SFAll,
			flag:     engine.SFAll,
			contains: true,
		},
		{
			flags:    engine.SFAll,
			flag:     engine.SFRegistered,
			contains: true,
		},
		{
			flags:    engine.SFAll,
			flag:     engine.FirstStateFlags,
			contains: true,
		},
		{
			flags:    engine.SFAll,
			flag:     engine.LastStateFlags,
			contains: true,
		},
		{
			flags:    engine.SFAll,
			flag:     engine.LastStateFlags + 1,
			contains: true,
		},
		{
			flags:    engine.SFAll,
			flag:     engine.LastStateFlags << 1,
			contains: false,
		},
		{
			flags:    engine.SFAll << 1,
			flag:     engine.SFAll,
			contains: false,
		},
		{
			flags:    engine.SFAll<<1 + 1,
			flag:     engine.SFAll,
			contains: true,
		},
		{
			flags:    engine.SFRegistered | engine.SFPlotting | engine.SFReady | engine.SFMining,
			flag:     engine.SFAll,
			contains: true,
		},
		{
			flags:    engine.SFRegistered | engine.SFPlotting,
			flag:     engine.SFPlotting,
			contains: true,
		},
	}

	for i, test := range tests {
		if contains := test.flags.Contains(test.flag); contains != test.contains {
			t.Errorf("%d, WorkSpaceStateFlags Contains not matched, expected %v, got %v", i, test.contains, contains)
		}
	}
}

func TestWorkSpaceStateFlags_States(t *testing.T) {
	tests := []struct {
		flags  engine.WorkSpaceStateFlags
		states []engine.WorkSpaceState
	}{
		{
			flags:  engine.SFRegistered,
			states: []engine.WorkSpaceState{engine.Registered},
		},
		{
			flags:  engine.SFRegistered | engine.SFReady,
			states: []engine.WorkSpaceState{engine.Registered, engine.Ready},
		},
		{
			flags:  engine.SFReady | engine.SFRegistered,
			states: []engine.WorkSpaceState{engine.Registered, engine.Ready},
		},
		{
			flags:  engine.SFAll,
			states: []engine.WorkSpaceState{engine.Registered, engine.Plotting, engine.Ready, engine.Mining},
		},
		{
			flags:  engine.SFAll + 1,
			states: []engine.WorkSpaceState{},
		},
	}

	for i, test := range tests {
		states := test.flags.States()
		if len(states) != len(test.states) {
			t.Errorf("%d, WorkSpaceStateFlags States length not matched, expected %v, got %v", i, test.states, states)
			continue
		}
		for j := range states {
			if states[j] != test.states[j] {
				t.Errorf("%d, WorkSpaceStateFlags States not matched, expected %v, got %v", i, test.states, states)
				break
			}
		}
	}
}

func TestWorkSpaceStateFlags_String(t *testing.T) {
	tests := []struct {
		flags engine.WorkSpaceStateFlags
		str   string
	}{
		{
			flags: engine.SFRegistered,
			str:   "SFRegistered",
		},
		{
			flags: engine.SFRegistered | engine.SFMining,
			str:   "SFRegistered|SFMining",
		},
		{
			flags: engine.SFAll,
			str:   "SFRegistered|SFPlotting|SFReady|SFMining",
		},
		{
			flags: engine.SFAll<<1 + 1,
			str:   "SFRegistered|SFPlotting|SFReady|SFMining",
		},
		{
			flags: engine.SFAll + 1,
			str:   "SFNone",
		},
		{
			flags: engine.WorkSpaceStateFlags(0),
			str:   "SFNone",
		},
	}

	for i, test := range tests {
		if str := test.flags.String(); str != test.str {
			t.Errorf("%d, WorkSpaceStateFlags String not matched, expected %v, got %v", i, test.str, str)
		}
	}
}

func TestActionType_Invalid(t *testing.T) {
	tests := []struct {
		action  engine.ActionType
		isValid bool
	}{
		{
			action:  engine.Plot,
			isValid: true,
		},
		{
			action:  engine.Mine,
			isValid: true,
		},
		{
			action:  engine.Stop,
			isValid: true,
		},
		{
			action:  engine.Remove,
			isValid: true,
		},
		{
			action:  engine.Delete,
			isValid: true,
		},
		{
			action:  engine.LastAction,
			isValid: true,
		},
		{
			action:  engine.LastAction + 1,
			isValid: false,
		},
	}

	for i, test := range tests {
		if isValid := test.action.IsValid(); isValid != test.isValid {
			t.Errorf("%d, ActionType IsValid not matched, expected %v, got %v", i, test.isValid, isValid)
		}
	}
}

func TestActionType_String(t *testing.T) {
	tests := []struct {
		action engine.ActionType
		str    string
	}{
		{
			action: engine.Plot,
			str:    "plot",
		},
		{
			action: engine.Mine,
			str:    "mine",
		},
		{
			action: engine.Stop,
			str:    "stop",
		},
		{
			action: engine.Remove,
			str:    "remove",
		},
		{
			action: engine.Delete,
			str:    "delete",
		},
		{
			action: engine.FirstAction,
			str:    "plot",
		},
		{
			action: engine.LastAction,
			str:    "delete",
		},
		{
			action: engine.LastAction + 1,
			str:    "invalid(5)",
		},
		{
			action: engine.LastAction + 2,
			str:    "invalid(6)",
		},
	}

	for i, test := range tests {
		if str := test.action.String(); str != test.str {
			t.Errorf("%d, ActionType String not matched, expected %v, got %v", i, test.str, str)
		}
	}
}
