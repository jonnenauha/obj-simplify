package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jonnenauha/obj-simplify/objectfile"
)

var (
	StartParams = startParams{
		Input:   "",
		Output:  "",
		Eplison: 1e-6,
	}

	ApplicationName    = "obj-simplify"
	ApplicationURL     = "https://github.com/jonnenauha/" + ApplicationName
	ApplicationVersion = "0.1" // inject in build step or git tag?

	Processors = []*processor{
		&processor{Processor: Duplicates{}},
		&processor{Processor: Merge{}},
	}
)

type startParams struct {
	Input      string
	Output     string
	Stdout     bool
	Quiet      bool
	NoProgress bool
	Eplison    float64
}

func init() {
	version := false

	flag.StringVar(&StartParams.Input,
		"in", StartParams.Input, "Input file.")
	flag.StringVar(&StartParams.Output,
		"out", StartParams.Output, "Output file or directory.")
	flag.BoolVar(&StartParams.Stdout,
		"stdout", StartParams.Stdout, "Write output file to stdout. If enabled -out is ignored and any logging is written to stderr.")
	flag.Float64Var(&StartParams.Eplison,
		"epsilon", StartParams.Eplison, "Epsilon for float comparisons.")
	flag.BoolVar(&StartParams.Quiet,
		"quiet", StartParams.Quiet, "Silence stdout printing.")
	flag.BoolVar(&StartParams.NoProgress,
		"no-progress", StartParams.NoProgress, "No shell progress bars.")
	flag.BoolVar(&version,
		"version", false, "Print version and exit, ignores -quiet.")

	// -no-xxx to disable post processors
	for _, processor := range Processors {
		flag.BoolVar(&processor.Disabled, processor.NameCmd(), processor.Disabled, processor.Desc())
	}

	flag.Parse()

	initLogging(!StartParams.Stdout)

	// -version
	if version {
		fmt.Printf("%s v%s\n", ApplicationName, ApplicationVersion)
		os.Exit(0)
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
	// cpu profiling for development
	// github.com/pkg/profile
	//defer profile.Start(profile.ProfilePath(".")).Stop()

	if b, err := json.MarshalIndent(StartParams, "", "  "); err == nil {
		logInfo("\n%s v%s %s", ApplicationName, ApplicationVersion, b)
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
	parser := &Parser{}
	obj, err := parser.ParseFile(StartParams.Input)
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
	w := &Writer{obj: obj}
	if StartParams.Stdout {
		logFatalError(w.WriteTo(os.Stdout))
	} else {
		logFatalError(w.WriteFile(StartParams.Output))
	}
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
	logFileStats()

	logInfo(" ")
}

func logGeometryStats(stats, postprocessed objectfile.GeometryStats) {
	logInfo(" ")
	logResultsIntPostfix("Vertices", postprocessed.Vertices, computeStatsDiff(stats.Vertices, postprocessed.Vertices))
	logResultsIntPostfix("Normals", postprocessed.Normals, computeStatsDiff(stats.Normals, postprocessed.Normals))
	logResultsIntPostfix("UVs", postprocessed.UVs, computeStatsDiff(stats.UVs, postprocessed.UVs))
	logResultsIntPostfix("Params", postprocessed.Params, computeStatsDiff(stats.Params, postprocessed.Params))
}

func logObjectStats(stats, postprocessed objectfile.ObjStats) {
	logInfo(" ")
	logResultsIntPostfix("Groups", postprocessed.Groups, computeStatsDiff(stats.Groups, postprocessed.Groups))
	logResultsIntPostfix("Objects", postprocessed.Objects, computeStatsDiff(stats.Objects, postprocessed.Objects))
}

func logVertexDataStats(stats, postprocessed objectfile.ObjStats) {
	logInfo(" ")
	logResultsIntPostfix("Faces", postprocessed.Faces, computeStatsDiff(stats.Faces, postprocessed.Faces))
	logResultsIntPostfix("Lines", postprocessed.Lines, computeStatsDiff(stats.Lines, postprocessed.Lines))
	logResultsIntPostfix("Points", postprocessed.Points, computeStatsDiff(stats.Points, postprocessed.Points))
}

func logFileStats() {
	logInfo(" ")
	sizeIn, sizeOut := fileSize(StartParams.Input), fileSize(StartParams.Output)
	logResults("Input file", formatBytes(sizeIn))
	logResults("Output file", formatBytes(sizeOut))
	if sizeOut < sizeIn {
		logResultsPostfix("Diff", formatBytes(sizeOut-sizeIn), "-"+intToString(int(100-computePerc(float64(sizeOut), float64(sizeIn))))+"%%")
	} else {
		logResultsPostfix("Diff", formatBytes(sizeOut-sizeIn), "+"+intToString(int(computePerc(float64(sizeOut), float64(sizeIn))-100))+"%%")
	}
}

func computeStatsDiff(a, b int) string {
	if b == 0 || a == b {
		return ""
	}
	diff := b - a
	perc := computePerc(float64(b), float64(a))
	if perc > 100.0 {
		return fmt.Sprintf("%-7d    +%d", diff, int(perc)) + "%%"
	} else if perc <= 99.0 {
		return fmt.Sprintf("%-7d    -%d", diff, 100-int(perc)) + "%%"
	}
	return fmt.Sprintf("%-7d    -%.2f", diff, 100-perc) + "%%"
}

func computePerc(step, total float64) float64 {
	if step == 0 {
		return 0.0
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
