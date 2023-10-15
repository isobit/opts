package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

type Runner interface {
	Run() error
}

type ContextRunner interface {
	Run(context.Context) error
}

type Beforer interface {
	Before() error
}

type Setuper interface {
	SetupCommand(cmd *Command)
}

type ExitCoder interface {
	ExitCode() int
}

type Command struct {
	cli             *CLI
	name            string
	help            string
	description     string
	config          interface{}
	internalConfig  internalConfig
	fields          []field
	argsField       *argsField
	flagset         *flag.FlagSet
	flagsetInternal *flag.FlagSet
	parent          *Command
	commands        []*Command
	commandMap      map[string]*Command
}

type internalConfig struct {
	Help bool `cli:"short=h,help=show usage help"`
}

func (cli *CLI) New(name string, config interface{}, opts ...CommandOption) *Command {
	cmd, err := cli.Build(name, config, opts...)
	if err != nil {
		panic(fmt.Sprintf("cli: %s", err))
	}
	return cmd
}

func (cli *CLI) Build(name string, config interface{}, opts ...CommandOption) (*Command, error) {
	if config == nil {
		config = &struct{}{}
	}
	cmd := &Command{
		cli:        cli,
		name:       name,
		config:     config,
		fields:     []field{},
		commands:   []*Command{},
		commandMap: map[string]*Command{},
	}

	internalFields, _, err := cli.getFieldsFromConfig(&cmd.internalConfig)
	if err != nil {
		return nil, fmt.Errorf("error building internal config: %w", err)
	}
	cmd.fields = append(cmd.fields, internalFields...)
	cmd.flagsetInternal = newFlagSet(name, internalFields)

	configFields, argsField, err := cli.getFieldsFromConfig(config)
	if err != nil {
		return nil, err
	}
	cmd.fields = append(cmd.fields, configFields...)
	cmd.argsField = argsField
	cmd.flagset = newFlagSet(name, cmd.fields)

	if setuper, ok := cmd.config.(Setuper); ok {
		setuper.SetupCommand(cmd)
	}

	for _, opt := range opts {
		opt.Apply(cmd)
	}

	return cmd, nil
}

func newFlagSet(name string, fields []field) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, f := range fields {
		fs.Var(f.flagValue, f.Name, f.Help)
		if f.ShortName != "" {
			fs.Var(f.flagValue, f.ShortName, f.Help)
		}
	}
	return fs
}

func (cmd *Command) SetHelp(help string) *Command {
	cmd.help = help
	return cmd
}

func (cmd *Command) SetDescription(description string) *Command {
	cmd.description = description
	return cmd
}

// AddCommand registers another Command instance as a subcommand of this Command
// instance.
func (cmd *Command) AddCommand(subCmd *Command) *Command {
	if cmd.argsField != nil {
		// TODO return error
		panic("cli: subcommands cannot be added to a command with an args field")
	}
	subCmd.parent = cmd
	cmd.commands = append(cmd.commands, subCmd)
	cmd.commandMap[subCmd.name] = subCmd
	return cmd
}

func (cmd *Command) Apply(parent *Command) {
	parent.AddCommand(cmd)
}

// Parse is a convenience method for calling ParseArgs(os.Args[1:])
func (cmd *Command) Parse() ParseResult {
	return cmd.ParseArgs(os.Args[1:])
}

// ParseArgs parses the passed-in args slice, along with environment variables,
// into the config fields, and returns a ParseResult which can be used for
// further method chaining.
//
// If there are args remaining after parsing this Command's fields, subcommands
// will be recursively parsed until a concrete result is returned
//
// If a Before method is implemented on the config, this method will call it
// before calling Run or recursing into any subcommand parsing.
func (cmd *Command) ParseArgs(args []string) ParseResult {
	return cmd.ParseArgsGNU(args)

	// if args == nil {
	// 	args = []string{}
	// }

	// r := ParseResult{Command: cmd}

	// if cmd.parent == nil && len(args) > 0 {
	// 	// Do a minimal recursive parsing pass (only on the internal flagset)
	// 	// so we can exit early with help if the help flag is passed on this
	// 	// command or any subcommand before proceeding.
	// 	helpParsedArgs := cmd.helpPass(args)
	// 	if helpParsedArgs.Err == ErrHelp {
	// 		return helpParsedArgs
	// 	}
	// }

	// // Parse arguments using the flagset.
	// if err := cmd.flagset.Parse(args); err != nil {
	// 	return r.err(UsageErrorf("failed to parse args: %w", err))
	// }

	// // Return ErrHelp if help was requested.
	// if cmd.internalConfig.Help {
	// 	return r.err(ErrHelp)
	// }

	// // Parse environment variables.
	// if err := cmd.parseEnvVars(); err != nil {
	// 	return r.err(UsageErrorf("failed to parse environment variables: %w", err))
	// }

	// // Handle remaining arguments so we get unknown command errors before
	// // invoking Before.
	// var subCmd *Command
	// rargs := cmd.flagset.Args()
	// if len(rargs) > 0 {
	// 	switch {
	// 	case cmd.argsField != nil:
	// 		cmd.argsField.setter(rargs)

	// 	case len(cmd.commandMap) > 0:
	// 		cmdName := rargs[0]
	// 		if cmd, ok := cmd.commandMap[cmdName]; ok {
	// 			subCmd = cmd
	// 		} else {
	// 			return r.err(UsageErrorf("unknown command: %s", cmdName))
	// 		}

	// 	default:
	// 		return r.err(UsageErrorf("command does not take arguments"))
	// 	}
	// }

	// // Return an error if any required fields were not set at least once.
	// if err := cmd.checkRequired(); err != nil {
	// 	return r.err(UsageError(err))
	// }

	// // If the config implements a Before method, run it before we recursively
	// // parse subcommands.
	// if beforer, ok := cmd.config.(Beforer); ok {
	// 	if err := beforer.Before(); err != nil {
	// 		return r.err(err)
	// 	}
	// }

	// // Recursive to subcommand parsing, if applicable.
	// if subCmd != nil {
	// 	return subCmd.ParseArgs(rargs[1:])
	// }

	// r.runFunc = getRunFunc(cmd.config)
	// if r.runFunc == nil && len(cmd.commands) != 0 {
	// 	return r.err(UsageErrorf("no command specified"))
	// }

	// return r
}

type runFunc struct {
	run             func(context.Context) error
	supportsContext bool
}

func getRunFunc(config interface{}) *runFunc {
	if r, ok := config.(Runner); ok {
		run := func(context.Context) error {
			return r.Run()
		}
		return &runFunc{
			run:             run,
			supportsContext: false,
		}
	}
	if r, ok := config.(ContextRunner); ok {
		return &runFunc{
			run:             r.Run,
			supportsContext: true,
		}
	}
	return nil
}

// parseEnvVars sets any unset field values using the environment variable
// matching the "env" tag of the field, if present.
func (cmd *Command) parseEnvVars() error {
	for _, f := range cmd.fields {
		if f.EnvVarName == "" || f.flagValue.setCount > 0 {
			continue
		}
		val, ok, err := cmd.cli.LookupEnv(f.EnvVarName)
		if err != nil {
			// TODO?
			return err
		}
		if ok {
			if err := f.flagValue.Set(val); err != nil {
				return fmt.Errorf("error parsing %s: %w", f.EnvVarName, err)
			}
		}
	}
	return nil
}

// checkRequired returns an error if any fields are required but have not been set.
func (cmd *Command) checkRequired() error {
	for _, f := range cmd.fields {
		if f.Required && f.flagValue.setCount < 1 {
			return fmt.Errorf("required flag %s not set", f.Name)
		}
	}
	return nil
}

// UsageError wraps the given error as a UsageErrorWrapper.
func UsageError(err error) UsageErrorWrapper {
	return UsageErrorWrapper{Err: err}
}

// UsageErrorf is a convenience method for wrapping the result of fmt.Errorf as
// a UsageErrorWrapper.
func UsageErrorf(format string, v ...interface{}) UsageErrorWrapper {
	return UsageErrorWrapper{Err: fmt.Errorf(format, v...)}
}

// UsageErrorWrapper wraps another error to indicate that the error was due to
// incorrect usage. When this error is handled, help text should be printed in
// addition to the error message.
type UsageErrorWrapper struct {
	Err error
}

func (w UsageErrorWrapper) Unwrap() error {
	return w.Err
}

func (w UsageErrorWrapper) Error() string {
	return w.Err.Error()
}

// ParseResult contains information about the results of command argument
// parsing.
type ParseResult struct {
	Err     error
	Command *Command
	runFunc *runFunc
}

// Convenience method for returning errors wrapped as a ParsedResult.
func (r ParseResult) err(err error) ParseResult {
	r.Err = err
	return r
}

func (r ParseResult) writeHelpIfUsageOrHelpError(err error) {
	if err == nil || r.Command == nil || r.Command.cli.HelpWriter == nil {
		return
	}
	_, isUsageErr := err.(UsageErrorWrapper)
	if isUsageErr || err == ErrHelp {
		r.Command.WriteHelp(r.Command.cli.HelpWriter)
	}
}

// Run calls the Run method of the Command config for the parsed command or, if
// an error occurred during parsing, prints the help text and returns that
// error instead. If help was requested, the error will flag.ErrHelp. If the
// underlying command Run method accepts a context, context.Background() will
// be passed.
func (r ParseResult) Run() error {
	return r.RunWithContext(context.Background())
}

// RunWithContext is like Run, but it accepts an explicit context which will be
// passed to the command's Run method, if it accepts one.
func (r ParseResult) RunWithContext(ctx context.Context) error {
	if r.Err != nil {
		r.writeHelpIfUsageOrHelpError(r.Err)
		return r.Err
	}
	if r.runFunc == nil {
		return fmt.Errorf("no run method implemented")
	}
	if err := r.runFunc.run(ctx); err != nil {
		r.writeHelpIfUsageOrHelpError(err)
		return err
	}
	return nil
}

// RunWithSigCancel is like Run, but it automatically registers a signal
// handler for SIGINT and SIGTERM that will cancel the context that is passed
// to the command's Run method, if it accepts one.
func (r ParseResult) RunWithSigCancel() error {
	ctx, stop := r.contextWithSigCancelIfSupported(context.Background())
	defer stop()
	return r.RunWithContext(ctx)
}

// RunFatal is like Run, except it automatically handles printing out any
// errors returned by the Run method of the underlying Command config, and
// exits with an appropriate status code.
//
// If no error occurs, the exit code will be 0. If an error is returned and it
// implements the ExitCoder interface, the result of ExitCode() will be used as
// the exit code. If an error is returned that does not implement ExitCoder,
// the exit code will be 1.
func (r ParseResult) RunFatal() {
	r.RunFatalWithContext(context.Background())
}

// RunFatalWithContext is like RunFatal, but it accepts an explicit context
// which will be passed to the command's Run method if it accepts one.
func (r ParseResult) RunFatalWithContext(ctx context.Context) {
	err := r.RunWithContext(ctx)
	if err != nil {
		if err != ErrHelp && r.Command != nil && r.Command.cli.ErrWriter != nil {
			fmt.Fprintf(r.Command.cli.ErrWriter, "error: %s\n", err)
		}
		if ec, ok := err.(ExitCoder); ok {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

// RunFatalWithSigCancel is like RunFatal, but it automatically registers a
// signal handler for SIGINT and SIGTERM that will cancel the context that is
// passed to the command's Run method, if it accepts one.
func (r ParseResult) RunFatalWithSigCancel() {
	ctx, stop := r.contextWithSigCancelIfSupported(context.Background())
	defer stop()
	r.RunFatalWithContext(ctx)
}

func (r ParseResult) contextWithSigCancelIfSupported(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.runFunc == nil || !r.runFunc.supportsContext {
		return ctx, func() {}
	}
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		// Cancel the signal notify on the first signal so that subsequent
		// SIGINT/SIGTERM immediately interrupt the program using the usual go
		// runtime handling.
		<-ctx.Done()
		cancel()
	}()
	return ctx, cancel
}

type CommandOption interface {
	Apply(cmd *Command)
}

type commandOptionFunc func(cmd *Command)

func (of commandOptionFunc) Apply(cmd *Command) {
	of(cmd)
}

func WithHelp(help string) CommandOption {
	return commandOptionFunc(func(cmd *Command) {
		cmd.SetHelp(help)
	})
}

func WithDescription(description string) CommandOption {
	return commandOptionFunc(func(cmd *Command) {
		cmd.SetDescription(description)
	})
}
