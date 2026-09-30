// Command pixelita is the task-oriented front door to the shared image engine.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/engine"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

var legacyTools = []string{"scan", "quant", "webp", "jpeg", "resize", "diff", "look"}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	name := args[0]
	switch name {
	case "-version", "--version":
		cli.Version(stdout, "pixelita")
		return 0
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "optimize":
		return runOptimize(args[1:], stdout, stderr)
	case "compare":
		return runCompare(args[1:], stdout, stderr)
	case "view":
		return runView(args[1:], stdout, stderr)
	case "capabilities":
		return runCapabilities(args[1:], stdout, stderr)
	}
	for _, legacy := range legacyTools {
		if name == legacy || name == "img-"+legacy {
			return runLegacy("img-"+legacy, args[1:], stderr)
		}
	}
	return argumentError(stdout, stderr, "pixelita", args, "unknown_command", fmt.Errorf("unknown command %q", name), nil)
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("inspect")
	var asJSON, measure, verbose bool
	var jobs int
	fs.BoolVar(&asJSON, "json", false, "emit JSON")
	fs.BoolVar(&measure, "measure", false, "encode candidates and measure opportunities")
	fs.BoolVar(&verbose, "v", false, "show unchanged files")
	fs.IntVar(&jobs, "jobs", 0, "parallel workers")
	if done, err := parseTaskFlags(fs, args, stdout, "inspect [flags] <paths>..."); done {
		return 0
	} else if err != nil {
		return argumentError(stdout, stderr, "pixelita inspect", args, "invalid_arguments", err, nil)
	}
	rep, err := engine.Inspect(engine.InspectRequest{Paths: fs.Args(), Measure: measure, Jobs: jobs})
	if err != nil {
		return argumentError(stdout, stderr, "pixelita inspect", args, argumentCode(err), err, nil)
	}
	return rep.Emit(stdout, inspectColumns, asJSON, verbose)
}

func runOptimize(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("optimize")
	req := engine.OptimizeRequest{}
	var asJSON bool
	var widths string
	fs.BoolVar(&asJSON, "json", false, "emit JSON")
	fs.BoolVar(&req.Apply, "apply", false, "write the previewed decisions")
	fs.BoolVar(&req.Overwrite, "overwrite", false, "allow replacing an existing destination")
	fs.BoolVar(&req.Replace, "replace", false, "allow replacing each source")
	fs.BoolVar(&req.DeleteSource, "delete-source", false, "delete a source after a verified WebP write")
	fs.StringVar(&req.OutDir, "out-dir", "", "write outputs in this directory")
	fs.StringVar(&req.Format, "format", "keep", "keep or webp; conversion requires webp explicitly")
	fs.StringVar(&widths, "widths", "", "comma-separated resize variants; dimensions change only when explicit")
	fs.Float64Var(&req.MinGain, "min-gain", 0, "minimum percentage gain; 0 uses the strategy default")
	fs.Float64Var(&req.MinPSNR, "min-psnr", 0, "minimum PSNR; 0 uses the strategy default")
	fs.Float64Var(&req.MinSSIM, "min-ssim", 0, "minimum SSIM; 0 uses the strategy default")
	fs.IntVar(&req.Jobs, "jobs", 0, "parallel workers")
	if done, err := parseTaskFlags(fs, args, stdout, "optimize [flags] <paths>..."); done {
		return 0
	} else if err != nil {
		return argumentError(stdout, stderr, "pixelita optimize", args, "invalid_arguments", err, nil)
	}
	var err error
	if req.Widths, err = parseWidths(widths); err != nil {
		return argumentError(stdout, stderr, "pixelita optimize", args, "invalid_arguments", err, nil)
	}
	if req.MinGain < 0 || req.MinPSNR < 0 || req.MinSSIM < 0 || req.MinSSIM > 1 {
		return argumentError(stdout, stderr, "pixelita optimize", args, "invalid_threshold", fmt.Errorf("thresholds must be non-negative and min-ssim cannot exceed 1"), nil)
	}
	req.Paths = fs.Args()
	plan, err := engine.BuildOptimizePlan(req)
	if err != nil {
		code := argumentCode(err)
		var pe *ops.PlanError
		if errors.As(err, &pe) {
			code = pe.Code
		}
		return argumentError(stdout, stderr, "pixelita optimize", args, code, err, recoveryFor("optimize", code, args))
	}
	rep := engine.ExecuteOptimize(plan)
	if !req.Apply {
		nextArgs := append([]string{"optimize"}, args...)
		nextArgs = append(nextArgs, "--apply")
		rep.NextAction = &report.NextAction{Command: "pixelita", Args: nextArgs}
	}
	return rep.Emit(stdout, optimizeColumns, asJSON, true)
}

func runCompare(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("compare")
	req := engine.CompareRequest{}
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "emit JSON")
	fs.Float64Var(&req.MinPSNR, "min-psnr", 0, "fail below PSNR")
	fs.Float64Var(&req.MinSSIM, "min-ssim", 0, "fail below SSIM")
	fs.BoolVar(&req.StrictSize, "strict-size", false, "reject different dimensions")
	if done, err := parseTaskFlags(fs, args, stdout, "compare [flags] <image-a> <image-b>"); done {
		return 0
	} else if err != nil {
		return argumentError(stdout, stderr, "pixelita compare", args, "invalid_arguments", err, nil)
	}
	if fs.NArg() != 2 {
		return argumentError(stdout, stderr, "pixelita compare", args, "expected_two_inputs", fmt.Errorf("compare requires exactly two image paths"), nil)
	}
	if req.MinPSNR < 0 || req.MinSSIM < 0 || req.MinSSIM > 1 {
		return argumentError(stdout, stderr, "pixelita compare", args, "invalid_threshold", fmt.Errorf("thresholds must be non-negative and min-ssim cannot exceed 1"), nil)
	}
	req.A, req.B = fs.Arg(0), fs.Arg(1)
	rep := engine.Compare(req)
	return rep.Emit(stdout, compareColumns, asJSON, true)
}

func runView(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("view")
	req := engine.ViewRequest{}
	max := ops.DefaultLookMax
	var asJSON, dryRun bool
	fs.BoolVar(&asJSON, "json", false, "emit JSON")
	fs.BoolVar(&dryRun, "dry-run", false, "decode and report without writing")
	fs.BoolVar(&req.Overwrite, "overwrite", false, "replace an existing preview")
	fs.IntVar(&max, "max", max, "longest preview side in pixels; 0 keeps the original")
	fs.StringVar(&req.Out, "out", "", "preview PNG path")
	if done, err := parseTaskFlags(fs, args, stdout, "view [flags] <paths>..."); done {
		return 0
	} else if err != nil {
		return argumentError(stdout, stderr, "pixelita view", args, "invalid_arguments", err, nil)
	}
	if fs.NArg() == 0 {
		return argumentError(stdout, stderr, "pixelita view", args, "no_input", cli.ErrNoInput, nil)
	}
	if max < 0 {
		return argumentError(stdout, stderr, "pixelita view", args, "invalid_option", fmt.Errorf("max cannot be negative"), nil)
	}
	req.Paths, req.Max, req.DryRun = fs.Args(), &max, dryRun
	rep, err := engine.View(req)
	if err != nil {
		code := "decode_failed"
		var pe *ops.PlanError
		if errors.As(err, &pe) {
			code = pe.Code
		}
		return argumentError(stdout, stderr, "pixelita view", args, code, err, recoveryFor("view", code, args))
	}
	return rep.Emit(stdout, viewColumns, asJSON, true)
}

func runCapabilities(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("capabilities")
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "emit JSON")
	done, err := parseTaskFlags(fs, args, stdout, "capabilities [--json]")
	if done {
		return 0
	}
	if err == nil && fs.NArg() != 0 {
		err = fmt.Errorf("capabilities takes no paths")
	}
	if err != nil {
		return argumentError(stdout, stderr, "pixelita capabilities", args, "invalid_arguments", err, nil)
	}
	caps := engine.Capabilities()
	if asJSON {
		rep := report.New("pixelita capabilities", "listed", true)
		rep.Capabilities = caps
		build := cli.BuildInfo()
		rep.Build = &build
		rep.Finish()
		_ = rep.WriteJSON(stdout)
		return 0
	}
	for _, c := range caps {
		fmt.Fprintf(stdout, "%-14s %-15s %s\n", c.ID, c.OutputFormat, strings.Join(c.Intents, ","))
	}
	return 0
}

func parseTaskFlags(fs *flag.FlagSet, args []string, stdout io.Writer, syntax string) (bool, error) {
	err := cli.ParseFlags(fs, args)
	if !errors.Is(err, flag.ErrHelp) {
		return false, err
	}
	fmt.Fprintf(stdout, "usage: pixelita %s\n\nOptions:\n", syntax)
	fs.SetOutput(stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
	return true, nil
}

func argumentError(stdout, stderr io.Writer, tool string, args []string, code string, err error, next *report.NextAction) int {
	w := stderr
	if cli.WantsJSON(args) {
		w = stdout
	}
	cli.WriteArgumentError(w, tool, code, err, args, next)
	return 2
}

func argumentCode(err error) string {
	if errors.Is(err, cli.ErrNoInput) {
		return "no_input"
	}
	if strings.Contains(err.Error(), "unsupported input") {
		return "unsupported_input"
	}
	return "invalid_arguments"
}

func recoveryFor(command, code string, args []string) *report.NextAction {
	var flagName string
	switch code {
	case "destination_exists":
		flagName = "--overwrite"
	case "source_replacement_requires_permission":
		flagName = "--replace"
	default:
		return nil
	}
	nextArgs := append([]string{command}, args...)
	nextArgs = append(nextArgs, flagName)
	return &report.NextAction{Command: "pixelita", Args: nextArgs}
}

func parseWidths(s string) ([]int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("bad width %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func runLegacy(name string, args []string, stderr io.Writer) int {
	cmd := exec.Command(resolve(name), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func resolve(name string) string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), exeName(name))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return name
}

func exeName(name string) string {
	if filepath.Ext(name) == "" && os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "pixelita — inspect, plan, apply, and verify image work")
	fmt.Fprintln(w, "\nusage: pixelita <inspect|optimize|compare|view|capabilities> [flags] <paths>...")
	fmt.Fprintln(w, "\nRules:")
	fmt.Fprintln(w, "  optimize previews; add --apply to write")
	fmt.Fprintln(w, "  format changes, resizing, overwrite, replacement, and deletion are explicit")
	fmt.Fprintln(w, "  JSON is authoritative; failures include stable codes and recovery actions")
	fmt.Fprintln(w, "\nExamples:")
	fmt.Fprintln(w, "  pixelita inspect --json ./images")
	fmt.Fprintln(w, "  pixelita optimize ./images --json")
	fmt.Fprintln(w, "  pixelita optimize ./images --apply --out-dir ./dist")
}

var inspectColumns = []report.Column{
	{Title: "file", Width: 42, Value: func(i report.Item) string { return i.Path }},
	{Title: "size", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "format", Width: 8, Value: func(i report.Item) string { return i.Str("format") }},
	{Title: "dimensions", Width: 14, Value: func(i report.Item) string { return i.Str("size") }},
	{Title: "decision", Width: 24, Value: func(i report.Item) string { return report.Action(i, "inspected") }},
}

var optimizeColumns = []report.Column{
	{Title: "file", Width: 38, Value: func(i report.Item) string { return i.Path }},
	{Title: "strategy", Width: 14, Value: func(i report.Item) string { return i.Str("strategy") }},
	{Title: "output", Width: 38, Value: func(i report.Item) string { return i.Output }},
	{Title: "gain", Width: 7, Right: true, Value: func(i report.Item) string { return report.Percent(i.GainPercent, i.BytesAfter > 0) }},
	{Title: "decision", Width: 25, Value: func(i report.Item) string { return report.Action(i, "optimized") }},
}

var compareColumns = []report.Column{
	{Title: "a", Width: 38, Value: func(i report.Item) string { return i.Path }},
	{Title: "b", Width: 38, Value: func(i report.Item) string { return i.Output }},
	{Title: "psnr", Width: 10, Right: true, Value: func(i report.Item) string {
		v, ok := i.Num("psnr")
		if !ok {
			return "identical"
		}
		return fmt.Sprintf("%.1f dB", v)
	}},
	{Title: "ssim", Width: 8, Right: true, Value: func(i report.Item) string {
		v, ok := i.Num("ssim")
		if !ok {
			return ""
		}
		return fmt.Sprintf("%.3f", v)
	}},
}

var viewColumns = []report.Column{
	{Title: "file", Width: 42, Value: func(i report.Item) string { return i.Path }},
	{Title: "output", Width: 48, Value: func(i report.Item) string { return i.Output }},
	{Title: "shown", Width: 14, Value: func(i report.Item) string { return i.Str("to") }},
}
