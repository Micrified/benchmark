/*
 *******************************************************************************
 *              (C) Copyright 2020 Delft University of Technology              *
 * Created: 20/10/2020                                                         *
 *                                                                             *
 * Programmer(s):                                                              *
 * - Charles Randolph                                                          *
 *                                                                             *
 * Description:                                                                *
 *  This package contains functions used evaluating a directory of C programs. *
 *   The tool used for evaluating is perf stat for Linux. For this reason, thi *
 *  s package is only compatible with Linux. In order to get perf stat, look t *
 *  o install linux-tools-5.4.0-51-generic from your package manager. This pac *
 *  kage was developed on Ubuntu 18.04.5 LTS                                   *
 *                                                                             *
 *******************************************************************************
*/

package benchmark

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"errors"
	"regexp"
	"unicode"
	"math"
	"bufio"
)


/*
 *******************************************************************************
 *                              Type Definitions                               *
 *******************************************************************************
*/


// Describes directories used
type Configuration struct {
	Src      string            // Benchmarks directory
	Stats    string            // Data (results) directory
	Bin      string            // Binaries (compiled benchmarks)
}

// Describes a benchmark
type Benchmark struct {
	Name        string
	Path        string
	Runtime     float64
	Uncertainty float64
}


/*
 *******************************************************************************
 *                         Internal Package Functions                          *
 *******************************************************************************
*/


// Concatenates the given strings on the path
func path (elements ...string) string {
	if len(elements) == 0 {
		return ""
	}
	if len(elements) == 1 {
		return elements[0]
	}
	p := elements[0]
	for _, e := range elements[1:] {
		p = p + "/" + e
	}
	return p
}

// Returns true if a directory contains a given file
func directory_contains_file (name, directory string) (bool, error) {
	files, err := ioutil.ReadDir(directory)
	if nil != err {
		return false, err
	}
	for _, f := range files {
		if f.Name() == name {
			return true, nil
		}
	}
	return false, nil
}

// Returns slice with names of all files containing given suffix
func get_files_by_suffix (directory, suffix string) ([]string, error) {
	bag := []string{}

	// Get all files
	files, err := ioutil.ReadDir(directory)
	if nil != err {
		return bag, errors.New("Unable to read directory \"" + directory + "\", reason: " + err.Error())
	}

	// Sort files with given suffix into the bag
	for _, file := range files {

		// Skip directories (we only want files)
		if file.IsDir() {
			continue;
		}

		// Only copy in files with the desired suffix
		if strings.HasSuffix(file.Name(), suffix) {
			bag = append(bag, directory + "/" + file.Name())
		}
	}

	return bag, nil
}

// Returns a compile command for the given directory 
func get_compile_command (compiler, name, src_dir, bin_dir string) (*exec.Cmd, error) {
	var args []string = []string{}

	// Create and insert executable file name (bin_dir/name)
	executable_file_path := path(bin_dir, name)
	args = append(args, "-o")
	args = append(args, executable_file_path)

	// Obtain source files
	source_files, err := get_files_by_suffix(src_dir, ".c")
	if nil != err {
		return nil, err
	} else {
		args = append(args, source_files...)
	}

	// Check cardinality of source files
	if len(source_files) == 0 {
		return nil, errors.New("No source files found to compile")
	}

	return exec.Command(compiler, args...), nil
}

// Executes a command through a fork and exec
func compile_benchmark (cfg Configuration, benchmark *Benchmark, compiler string) error {

	// Obtain the compile command
	cmd, err := get_compile_command(compiler, benchmark.Name, benchmark.Path, cfg.Bin)
	if nil != err {
		return err
	}

	// Assign environment
	cmd.Env = os.Environ()

	// Execute the command
	return cmd.Run()
}

// Runs perf stat on executable in directory. Results placed in output directory
func evaluate_benchmark (benchmark *Benchmark, cfg Configuration, repeats int) error {
	var exists_executable bool 
	var err error
	var cmd *exec.Cmd

	// Create output file
	output_file_name := benchmark.Name + ".txt"
	output_file_path := path(cfg.Stats, output_file_name)
	args := []string{"chrt", "-f", "99", "perf", "stat", "-o", output_file_path, "-e", "duration_time"}

	// Locate the executable
	exists_executable, err = directory_contains_file(benchmark.Name, cfg.Bin)
	if nil != err {
		return errors.New("Unable to search directory \"" + cfg.Bin+ "\": " + err.Error())
	}

	// If the file doesn't exist
	if !exists_executable {
		return errors.New("Executable \"" + benchmark.Name + "\" not found in " + cfg.Bin)
	}

	// Append repeat count
	args = append(args, fmt.Sprintf("--repeat=%d", repeats))

	// Append executable name
	args = append(args, path(cfg.Bin, benchmark.Name))

	// Build command
	cmd = exec.Command("sudo", args...)
	cmd.Env = os.Environ()

	err = cmd.Run()
	return err
}

// Extracts perf stat runtime and uncertainty from results file; assigns to benchmark
func get_benchmark_results (cfg Configuration, benchmark *Benchmark) error {
	var line []byte                       = []byte{}
	var match_duration_exp string         = "([0-9],?)+\\s*ns"
	var match_uncertainty_exp string      = "[0-9]+.[0-9]+%"
	var err error                         = nil
	var match string                      = ""

	// Inline function returning float 
	get_float := func (match string) float64 {
		var value float64 = 0.0
		var filtered []rune = []rune{}
		var found_decimal bool = false
		var decimal_offset int = 0

		// Filter only digits
		runes := []rune(match)
		for _, r := range runes {
			if unicode.IsDigit(r) {
				filtered = append(filtered, r)
				if (found_decimal) {
					decimal_offset++
				}
			} else if (r == '.') {
				found_decimal = true
			}
		}

		// Assemble the float
		for _, d := range filtered {
			value = value * 10 + float64(byte(d - '0'))
		}

		return value / math.Pow10(decimal_offset)
	}

	// Inline function that returns the first matched regexp instance
	match_exp := func (line string, exp string) (string, error) {
		reg := regexp.MustCompile(exp)
		matches := reg.FindStringSubmatch(line)
		if len(matches) < 1 {
			return "", errors.New("No match found!")
		} else {
			return matches[0], nil
		}
	}

	// File path holding the results
	results_file_path := path(cfg.Stats, benchmark.Name + ".txt")

	// Attempt to open the file
	file, err := os.Open(results_file_path)
	if nil != err {
		return err
	} else {
		defer file.Close()
	}

	// Read lines
	more := true
	found_params := 0
	for reader := bufio.NewReader(file); more; {
		line, err = reader.ReadBytes('\n')

		// Register EoF
		if io.EOF == err {
			more = false
			err = nil
		}

		// Exit on non EoF error
		if nil != err {
			return err
		}

		// Try matching duration expression
		match, err = match_exp(string(line), match_duration_exp)
		if nil != err {
			continue
		} else {
			benchmark.Runtime = get_float(match)
			found_params++
		}

		// Try matching uncertainty expression
		match, err = match_exp(string(line), match_uncertainty_exp)
		if nil != err {
			continue
		} else {
			benchmark.Uncertainty = get_float(match)
			found_params++
		}

		if found_params >= 2 {
			break
		}
	}

	// Return error if didn't find params
	if found_params != 2 {
		return errors.New("Unable to locate runtime and/or uncertainty!")
	}

	return nil
}

// If needed, creates the supplied slice of directories as subdirectories
func make_directories (directories []string) error {
	var err error = nil

	// Creates directory if necessary
	make_if_needed := func (name string) error {
		exists, err := directory_contains_file(name, ".")
		if nil != err {
			return err
		}
		if !exists {
			err = os.Mkdir(name, 0777)
		}
		return err
	}

	// Create all supplied directories
	for _, d := range directories {
		err = make_if_needed(d)
		if nil != err {
			return err
		}
	}

	return nil
}


/*
 *******************************************************************************
 *                         External Package Functions                          *
 *******************************************************************************
*/


// Creates all the necessary folders (if needed) for running and evaluating benchmarks
func Init_Env (cfg Configuration) error {
	return make_directories([]string{cfg.Stats, cfg.Bin})
}

// Creates all benchmarks (expects that directory holds list of benchmark sub-directories)
func Init_Benchmarks (cfg Configuration) ([]*Benchmark, error) {
	var benchmarks []*Benchmark
	var files []os.FileInfo
	var err error

	// Open the given directory
	files, err = ioutil.ReadDir(cfg.Src)
	if nil != err {
		return benchmarks, err
	}

	// Create the benchmarks
	for _, file := range files {

		// Ignore files
		if !file.IsDir() {
			continue
		}

		// Assume sub-directory is a benchmark
		n := file.Name()
		b := Benchmark{Name: n, Path: path(cfg.Src, n), Runtime: 0.0, Uncertainty: 0.0}
		benchmarks = append(benchmarks, &b)
	}

	return benchmarks, nil
}

// Returns a slice of all benchmarks needing evaluation (those without results)
func Get_Unevaluated_Benchmarks (cfg Configuration, benchmarks []*Benchmark) ([]*Benchmark, error) {
	var unevaluated_benchmarks []*Benchmark
	var exists_file bool
	var err error

	// For each benchmark, determine whether a results file exists
	for _, b := range benchmarks {

		// Attempt to locate file in directory
		results_file_name := b.Name + ".txt"
		exists_file, err = directory_contains_file(results_file_name, cfg.Stats)

		// Handle directory issues
		if nil != err {
			return unevaluated_benchmarks, errors.New("Problem opening results directory: " + err.Error())
		}

		// Add to unevaluted set if it doesn't exist
		if !exists_file {
			unevaluated_benchmarks = append(unevaluated_benchmarks, b)
			continue
		}

		// Otherwise read in and set results
		err = get_benchmark_results(cfg, b)
		if nil != err {
			return unevaluated_benchmarks, errors.New("Unable to read results: " + err.Error())
		}
	}

	return unevaluated_benchmarks, nil
}

// Evaluates the given benchmark and reads in the results
func Evaluate_Benchmark (compiler string, cfg Configuration, repeats int, benchmark *Benchmark) error {
	var was_compiled bool = false
	var err error

	// Determine if compile is needed
	was_compiled, err = directory_contains_file(benchmark.Name, cfg.Bin)

	// Return on directory error
	if nil != err {
		return errors.New("Unable to search \"" + cfg.Bin + "\": " + err.Error())
	}

	// If the benchmark must be compiled, then compile it now
	if !was_compiled {
		fmt.Printf("Compiling benchmark \"%s\"...\n", benchmark.Name)
		err = compile_benchmark(cfg, benchmark, compiler)
	}

	// If the benchmark was compiled and an error occurred
	if nil != err {
		return errors.New("Problem compiling benchmark: " + err.Error())
	} else {
		fmt.Println("- Success")
	}

	// Evaluate the benchmark
	fmt.Printf("Evaluating benchmark \"%s\"...\n", benchmark.Name)
	err = evaluate_benchmark(benchmark, cfg, repeats)
	if nil != err {
		return errors.New("Problem evaluating benchmark: " + err.Error())
	} else {
		fmt.Println("- Success")
	}

	// Read in the results
	err = get_benchmark_results(cfg, benchmark)
	if nil != err {
		return errors.New("Unable to read results for " + benchmark.Name + ": " + err.Error())
	}

	return nil
}

func main () {
	var benchmarks []*Benchmark
	var unevaluated []*Benchmark 
	var err error 

	// Setup the configuration
	cfg := Configuration{Src: "tacle-bench/bench/sequential", Stats: "stats", Bin: "bin"}

	// Init environment
	if err := Init_Env(cfg); nil != err {
		log.Fatal(err.Error())
	}

	// Init benchmarks
	benchmarks, err = Init_Benchmarks(cfg)
	if nil != err {
		log.Fatal(err.Error())
	}

	// Extract any unevaluated benchmarks
	unevaluated, err = Get_Unevaluated_Benchmarks(cfg, benchmarks)
	if nil != err {
		log.Fatal(err.Error())
	}

	// Evaluate all unevaluted benchmarks
	for _, b := range unevaluated {
		err = Evaluate_Benchmark("cc", cfg, 10, b)
		if nil != err {
			log.Fatal(err.Error())
		}
	}

	// Print benchmarks
	for _, b := range benchmarks {
		fmt.Printf("%16s\t\t\t%.2f ns\t\t\t%.2f%%\n", b.Name, b.Runtime, b.Uncertainty)
	}

}