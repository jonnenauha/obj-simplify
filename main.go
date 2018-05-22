package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jonnenauha/obj-simplify/objectfile"
	"github.com/pkg/profile"
)

var (
	StartParams = startParams{
		Gzip:    -1,
		Epsilon: 1e-6,
	}

	ApplicationName = "obj-simplify"
	ApplicationURL  = "https://github.com/jonnenauha/" + ApplicationName
	Version         string
	VersionHash     string
	VersionDate     string

	Processors = []*processor{
		&processor{Processor: Duplicates{}},
		&processor{Processor: Merge{}},
	}
)

type startParams struct {
	Input  string
	Output string

	Workers int
	Gzip    int
	Epsilon float64

	Strict     bool
	Stdout     bool
	Quiet      bool
	NoProgress bool
	CpuProfile bool
}

func (sp startParams) IsGzipEnabled() bool {
	return sp.Gzip >= gzip.BestSpeed && sp.Gzip <= gzip.BestCompression
}

func init() {
	version := false

	StartParams.Workers = runtime.NumCPU() * 4
	if StartParams.Workers < 4 {
		StartParams.Workers = 4
	}

	flag.StringVar(&StartParams.Input,
		"in", StartParams.Input, "Input file.")
	flag.StringVar(&StartParams.Output,
		"out", StartParams.Output, "Output file or directory.")

	flag.IntVar(&StartParams.Workers,
		"workers", StartParams.Workers, "Number of worker goroutines.")
	flag.IntVar(&StartParams.Gzip,
		"gzip", StartParams.Gzip, "Gzip compression level on the output for both -stdout and -out. <=0 disables compression, use 1 (best speed) to 9 (best compression) to enable.")
	flag.Float64Var(&StartParams.Epsilon,
		"epsilon", StartParams.Epsilon, "Epsilon for float comparisons.")

	flag.BoolVar(&StartParams.Strict,
		"strict", StartParams.Strict, "Errors out on spec violations, otherwise continues if the error is recoverable.")
	flag.BoolVar(&StartParams.Stdout,
		"stdout", StartParams.Stdout, "Write output to stdout. If enabled -out is ignored and logging directed to stderr. Use -quiet if you can't separate stdout from stderr (e.g. non-trivial in Windows).")
	flag.BoolVar(&StartParams.Quiet,
		"quiet", StartParams.Quiet, "Silence stdout printing.")
	flag.BoolVar(&StartParams.NoProgress,
		"no-progress", StartParams.NoProgress, "No shell progress bars.")
	flag.BoolVar(&StartParams.CpuProfile,
		"cpu-profile", StartParams.CpuProfile, "Record ./cpu.pprof profile.")
	flag.BoolVar(&version,
		"version", false, "Print version and exit, ignores -quiet.")

	// -no-xxx to disable post processors
	for _, processor := range Processors {
		flag.BoolVar(&processor.Disabled, processor.NameCmd(), processor.Disabled, processor.Desc())
	}

	flag.Parse()

	initLogging(!StartParams.Stdout)

	// -version: ignores -stdout as we are about to exit
	if version {
		fmt.Printf("%s %s\n", ApplicationName, getVersion(true))
		os.Exit(0)
	}

	if StartParams.Workers < 1 {
		logFatal("-workers must be a positive number, given: %d", StartParams.Workers)
	}

	// -gzip
	if StartParams.Gzip < -1 || StartParams.Gzip > gzip.BestCompression {
		logFatal("-gzip must be -1 to 9, given: %d", StartParams.Gzip)
	}

	// -in
	StartParams.Input = cleanPath(StartParams.Input)
	if len(StartParams.Input) == 0 {
		logFatal("-in missing")
	} else if !fileExists(StartParams.Input) {
		logFatal("-in file %q does not exist", StartParams.Input)
	}

	// -out
	if !StartParams.Stdout {
		if len(StartParams.Output) > 0 {
			StartParams.Output = cleanPath(StartParams.Output)
		} else {
			if iExt := strings.LastIndex(StartParams.Input, "."); iExt != -1 {
				StartParams.Output = StartParams.Input[0:iExt] + ".simplified" + StartParams.Input[iExt:]
			} else {
				StartParams.Output = StartParams.Input + ".simplified"
			}
		}
		// don't allow user to overwrite source file, this app can be destructive and should
		// not overwrite the source files. If user really wants to do this, he can rename the output file.
		if StartParams.Input == StartParams.Output {
			logFatal("Overwriting input file is not allowed, both input and output point to %s\n", StartParams.Input)
		}
	}
}

func getVersion(date bool) (version string) {
	if Version == "" {
		return "dev"
	}
	version = fmt.Sprintf("v%s (%s)", Version, VersionHash)
	if date {
		version += " " + VersionDate
	}
	return version
}

type processor struct {
	Processor
	Disabled bool
}

func (p *processor) NameCmd() string {
	return "no-" + strings.ToLower(p.Name())
}

type Processor interface {
	Name() string
	Desc() string
	Execute(obj *objectfile.OBJ) error
}

func main() {
	// cpu profiling for development: github.com/pkg/profile
	if StartParams.CpuProfile {
		defer profile.Start(profile.ProfilePath(".")).Stop()
	}

	if b, err := json.MarshalIndent(StartParams, "", "  "); err == nil {
		logInfo("\n%s %s %s", ApplicationName, getVersion(false), b)
	} else {
		logFatalError(err)
	}

	type timing struct {
		Step     string
		Duration time.Duration
	}

	var (
		start    = time.Now()
		pre      = time.Now()
		timings  = []timing{}
		timeStep = func(step string) {
			timings = append(timings, timing{Step: step, Duration: time.Now().Sub(pre)})
			pre = time.Now()
		}
	)

	// parse
	obj, linesParsed, err := ParseFile(StartParams.Input)
	if err != nil {
		logFatalError(err)
	}
	timeStep("Parse")

	// store stats before post-processing
	preStats := obj.Stats()
	// @todo this is ugly, maybe the face objects could be marked somehow.
	// we want to show real stats, not faked object count stats at the end
	preStats.Objects = ObjectsParsed
	preStats.Groups = GroupsParsed

	// post processing
	for pi, processor := range Processors {
		logInfo(" ")
		if processor.Disabled {
			logInfo("processor #%d: %s - Disabled", pi+1, processor.Name())
			continue
		}
		logInfo("processor #%d: %s", pi+1, processor.Name())
		logFatalError(processor.Execute(obj))
		timeStep(processor.Name())
	}

	postStats := obj.Stats()

	// write file out
	var (
		w            = &Writer{obj: obj}
		linesWritten int
		errWrite     error
	)
	if StartParams.Stdout {
		linesWritten, errWrite = w.WriteTo(os.Stdout)
	} else {
		linesWritten, errWrite = w.WriteFile(StartParams.Output)
	}
	logFatalError(errWrite)
	timeStep("Write")

	// print stats etc
	logInfo(" ")
	durationTotal := time.Since(start)
	for _, timing := range timings {
		logResultsPostfix(timing.Step, formatDuration(timing.Duration), computeDurationPerc(timing.Duration, durationTotal)+"%%")
	}
	logResults("Total", formatDuration(durationTotal))

	logGeometryStats(preStats.Geometry, postStats.Geometry)
	logVertexDataStats(preStats, postStats)
	logObjectStats(preStats, postStats)
	logFileStats(linesParsed, linesWritten)

	if StartParams.IsGzipEnabled() {
		logInfo(" ")
		logInfo("Gzip compression enabled with level %d.", StartParams.Gzip)
		logInfo("Remeber to set 'Content-Encoding: gzip' header if you are hosting this file over HTTP.")
	}

	logInfo(" ")
}

func logGeometryStats(stats, postprocessed objectfile.GeometryStats) {
	if !stats.IsEmpty() {
		logInfo(" ")
	}
	if stats.Vertices > 0 {
		logResultsIntPostfix("Vertices", postprocessed.Vertices, computeStatsDiff(stats.Vertices, postprocessed.Vertices))
	}
	if stats.Normals > 0 {
		logResultsIntPostfix("Normals", postprocessed.Normals, computeStatsDiff(stats.Normals, postprocessed.Normals))
	}
	if stats.UVs > 0 {
		logResultsIntPostfix("UVs", postprocessed.UVs, computeStatsDiff(stats.UVs, postprocessed.UVs))
	}
	if stats.Params > 0 {
		logResultsIntPostfix("Params", postprocessed.Params, computeStatsDiff(stats.Params, postprocessed.Params))
	}
}

func logObjectStats(stats, postprocessed objectfile.ObjStats) {
	logInfo(" ")
	// There is a special case where input has zero objects and we have created one or more.
	if stats.Groups > 0 || postprocessed.Groups > 0 {
		logResultsIntPostfix("Groups", postprocessed.Groups, computeStatsDiff(stats.Groups, postprocessed.Groups))
	}
	if stats.Objects > 0 || postprocessed.Objects > 0 {
		logResultsIntPostfix("Objects", postprocessed.Objects, computeStatsDiff(stats.Objects, postprocessed.Objects))
	}
}

func logVertexDataStats(stats, postprocessed objectfile.ObjStats) {
	if stats.Faces > 0 || stats.Lines > 0 || stats.Points > 0 {
		logInfo(" ")
	}
	if stats.Faces > 0 {
		logResultsIntPostfix("Faces", postprocessed.Faces, computeStatsDiff(stats.Faces, postprocessed.Faces))
	}
	if stats.Lines > 0 {
		logResultsIntPostfix("Lines", postprocessed.Lines, computeStatsDiff(stats.Lines, postprocessed.Lines))
	}
	if stats.Points > 0 {
		logResultsIntPostfix("Points", postprocessed.Points, computeStatsDiff(stats.Points, postprocessed.Points))
	}
}

func logFileStats(linesParsed, linesWritten int) {
	logInfo(" ")
	logResults("Lines input", formatInt(linesParsed))
	if linesWritten < linesParsed {
		logResultsPostfix("Lines output", formatInt(linesWritten), fmt.Sprintf("%-10s %s", formatInt(linesWritten-linesParsed), "-"+intToString(int(100-computePerc(float64(linesWritten), float64(linesParsed))))+"%%"))
	} else {
		logResultsPostfix("Lines output", formatInt(linesWritten), fmt.Sprintf("+%-10s %s", formatInt(linesWritten-linesParsed), "+"+intToString(int(computePerc(float64(linesWritten), float64(linesParsed))-100))+"%%"))
	}

	logInfo(" ")
	sizeIn, sizeOut := fileSize(StartParams.Input), fileSize(StartParams.Output)
	logResults("File input", formatBytes(sizeIn))
	if !StartParams.Stdout {
		if sizeOut < sizeIn {
			logResultsPostfix("File output", formatBytes(sizeOut), fmt.Sprintf("%-10s %s", formatBytes(sizeOut-sizeIn), "-"+intToString(int(100-computePerc(float64(sizeOut), float64(sizeIn))))+"%%"))
		} else {
			logResultsPostfix("File output", formatBytes(sizeOut), fmt.Sprintf("+%-10s %s", formatBytes(sizeOut-sizeIn), "+"+intToString(int(computePerc(float64(sizeOut), float64(sizeIn))-100))+"%%"))
		}
	}
}

func computeStatsDiff(a, b int) string {
	if a == b {
		return ""
	}
	diff := b - a
	perc := computePerc(float64(b), float64(a))
	if perc >= 99.999999 {
		// positive 0 decimals
		return fmt.Sprintf("+%-7d", diff)
	} else if perc <= 99.0 {
		// negative 0 decimals
		return fmt.Sprintf("%-7d    -%d", diff, 100-int(perc)) + "%%"
	}
	// negative 2 decimals
	return fmt.Sprintf("%-7d    -%.2f", diff, 100-perc) + "%%"
}

func computePerc(step, total float64) float64 {
	if step == 0 {
		return 0.0
	} else if total == 0 {
		return 100.0
	}
	return (step / total) * 100.0
}

func computeFloatPerc(step, total float64) string {
	perc := computePerc(step, total)
	if perc < 1.0 {
		return fmt.Sprintf("%.2f", perc)
	}
	return intToString(int(perc))
}

func computeDurationPerc(step, total time.Duration) string {
	return computeFloatPerc(step.Seconds(), total.Seconds())
}
