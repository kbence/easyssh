package executors

import (
	"fmt"
	"strings"

	"sort"

	"github.com/abesto/easyssh/fromsexp"
	"github.com/abesto/easyssh/interfaces"
	"github.com/abesto/easyssh/target"
	"github.com/abesto/easyssh/util"
)

func Make(input string) interfaces.Executor {
	return fromsexp.MakeFromString(input, sexpTransforms, makeByName).(interfaces.Executor)
}

func SupportedExecutorNames() []string {
	names := make([]string, len(executorMakerMap)+len(sexpTransforms))

	// High-level executors
	for i := 0; i < len(sexpTransforms); i++ {
		names[i] = sexpTransforms[i].Name
	}

	// Low-level executors
	i := len(sexpTransforms)
	for key := range executorMakerMap {
		names[i] = key
		i++
	}

	sort.Strings(names)
	return names
}

func makeFromSExp(data []interface{}) interfaces.Executor {
	return fromsexp.Make(data, sexpTransforms, makeByName).(interfaces.Executor)
}

const (
	nameAssertCommand                 = "assert-command"
	nameAssertNoCommand               = "assert-no-command"
	nameExternal                      = "external"
	nameExternalInteractive           = "external-interactive"
	nameExternalSequential            = "external-sequential"
	nameExternalSequentialInteractive = "external-sequential-interactive"
	nameExternalParallel              = "external-parallel"
	nameIfOneTarget                   = "if-one-target"
	nameIfCommand                     = "if-command"
	nameNoop                          = "noop"
)

var executorMakerMap = map[string]func() interfaces.Executor{
	nameIfOneTarget:     func() interfaces.Executor { return &ifOneTarget{} },
	nameIfCommand:       func() interfaces.Executor { return &ifCommand{} },
	nameAssertCommand:   func() interfaces.Executor { return &assertCommand{require: true} },
	nameAssertNoCommand: func() interfaces.Executor { return &assertCommand{require: false} },
	nameExternal: func() interfaces.Executor {
		return &external{
			commandRunner: &util.RealInteractiveCommandRunner{},
			mode:          externalModeSingleRun,
		}
	},
	nameExternalSequential: func() interfaces.Executor {
		return &external{
			commandRunner: &util.RealInteractiveCommandRunner{},
			mode:          externalModeSequential,
		}
	},
	nameExternalParallel: func() interfaces.Executor {
		return &external{
			commandRunner: &util.RealInteractiveCommandRunner{},
			mode:          externalModeParallel,
		}
	},
	nameExternalInteractive: func() interfaces.Executor {
		return &external{
			commandRunner: &util.RealInteractiveCommandRunner{},
			mode:          externalModeSingleRun,
			interactive:   true,
		}
	},
	nameExternalSequentialInteractive: func() interfaces.Executor {
		return &external{
			commandRunner: &util.RealInteractiveCommandRunner{},
			mode:          externalModeSequential,
			interactive:   true,
		}
	},
}

var r = fromsexp.Replace
var sexpTransforms = []fromsexp.SexpTransform{
	r("(if-args)", "(if-command)"),
	r("(ssh-login)", "(assert-no-command (external-sequential-interactive ssh))"),
	r("(ssh-exec)", "(ssh-exec-sequential)"),
	r("(ssh-exec-sequential)", "(assert-command (external-sequential ssh))"),
	r("(ssh-exec-parallel)", "(assert-command (external-parallel ssh))"),
	r("(csshx)", "(assert-no-command (external-interactive csshx))"),
	r("(tmux-cssh)", "(assert-no-command (external-interactive tmux-cssh))"),
}

func makeByName(name string) interface{} {
	var d interfaces.Executor
	for key, maker := range executorMakerMap {
		if key == name {
			d = maker()
		}
	}
	if d == nil {
		util.Panicf("Executor \"%s\" is not known", name)
	}
	return d
}

const (
	externalModeSingleRun externalMode = iota
	externalModeSequential
	externalModeParallel
)

type external struct {
	initialArgs   []interface{}
	args          []string
	commandRunner util.InteractiveCommandRunner
	mode          externalMode
	interactive   bool
}

func (e *external) makeSingleRunJob(targets []target.Target, command []string) util.InteractiveCommandRunnerJob {
	return util.InteractiveCommandRunnerJob{
		Interactive: e.interactive,
		Label:       strings.Join(target.Strings(targets), " "),
		Argv:        append(e.args, append(target.Strings(targets), command...)...),
	}
}

func (e *external) makeJobPerTarget(targets []target.Target, command []string) []util.InteractiveCommandRunnerJob {
	jobs := make([]util.InteractiveCommandRunnerJob, len(targets))
	for i, target := range targets {
		jobs[i] = util.InteractiveCommandRunnerJob{
			Interactive: e.interactive,
			Label:       target.String(),
			Argv:        append(e.args, append([]string{target.String()}, command...)...),
		}
	}
	return jobs
}

func (e *external) Exec(targets []target.Target, command []string) {
	util.RequireArgumentsAtLeast(e, 1, e.initialArgs)
	if e.mode == externalModeSingleRun {
		e.commandRunner.Run(e.makeSingleRunJob(targets, command))
	} else if e.mode == externalModeSequential {
		for _, job := range e.makeJobPerTarget(targets, command) {
			e.commandRunner.Run(job)
		}
	} else if e.mode == externalModeParallel {
		util.Logger.Infof("Parallelly executing %s on %s", command, targets)
		e.commandRunner.RunParallel(e.makeJobPerTarget(targets, command))
	} else {
		util.Panicf("Unknown externalMode %s", e.mode)
	}
}

func (e *external) SetArgs(args []interface{}) {
	util.RequireArgumentsAtLeast(e, 1, args)
	e.initialArgs = args
	e.args = make([]string, len(args))
	for i, arg := range args {
		e.args[i] = string(arg.([]byte))
	}
}

func (e *external) String() string {
	rawName := "external"
	if e.mode == externalModeSequential {
		rawName += "-sequential"
	} else if e.mode == externalModeParallel {
		rawName += "-parallel"
	}
	if e.interactive {
		rawName += "-interactive"
	}
	return fmt.Sprintf("<%s %s>", rawName, e.args)
}

type ifOneTarget struct {
	initialArgs []interface{}
	one         interfaces.Executor
	more        interfaces.Executor
}

func (e *ifOneTarget) Exec(targets []target.Target, command []string) {
	util.RequireArguments(e, 2, e.initialArgs)
	if len(targets) == 1 {
		util.Logger.Debugf("%s got one target, using %s", e, e.one)
		e.one.Exec(targets, command)
	} else {
		util.Logger.Debugf("%s got more than one target, using %s", e, e.more)
		e.more.Exec(targets, command)
	}
}
func (e *ifOneTarget) SetArgs(args []interface{}) {
	util.RequireArguments(e, 2, args)
	e.initialArgs = args
	e.one = makeFromSExp(args[0].([]interface{}))
	e.more = makeFromSExp(args[1].([]interface{}))
}
func (e *ifOneTarget) String() string {
	return fmt.Sprintf("<%s %s %s>", nameIfOneTarget, e.one, e.more)
}

type ifCommand struct {
	initialArgs    []interface{}
	withCommand    interfaces.Executor
	withoutCommand interfaces.Executor
}

func (e *ifCommand) Exec(targets []target.Target, args []string) {
	util.RequireArguments(e, 2, e.initialArgs)
	if len(args) < 1 {
		util.Logger.Debugf("%s got no args, using %s", e, e.withoutCommand)
		e.withoutCommand.Exec(targets, args)
	} else {
		util.Logger.Debugf("%s got args, using %s", e, e.withCommand)
		e.withCommand.Exec(targets, args)
	}
}
func (e *ifCommand) SetArgs(args []interface{}) {
	util.RequireArguments(e, 2, args)
	e.withCommand = makeFromSExp(args[0].([]interface{}))
	e.withoutCommand = makeFromSExp(args[1].([]interface{}))
	e.initialArgs = args
}
func (e *ifCommand) String() string {
	return fmt.Sprintf("<%s %s %s>", nameIfCommand, e.withCommand, e.withoutCommand)
}
