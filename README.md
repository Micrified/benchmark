# Benchmark

This is a Go package for evaluating benchmarks. A benchmark folder is specified to the package, along with a binary folder and statistics folder. The package provides functions for running `perf stat` on all benchmark executables (which it compiles) and reading the timing information into memory for use with other packages. Statistics for benchmarks are saved in a subdirectory in order to avoid having to re-evaluate them each time


## Setting up the benchmark directory 

The benchmarks are all expected to be located in sub-directories within a source folder. They should all be capable of being compiled with a simple `cc -o <package_name> *.c` command. The TACLe benchmarks project ([see repository here](https://github.com/tacle/tacle-bench)) is an example of a project containing useful benchmarks. In this case, you could specify your source directory as follows: 

```
src := "tacle-bench/bench/sequential"
```

## Evaluating benchmarks

Benchmarks should be evaluted in the following steps: 

1. Check if the benchmark already has results in the statistics directory
2. If not, then compile the benchmark if necessary
3. Execute the benchmark, and log the timing information to the statistics directory

The following example Go program shows how to do that: 

```
func main () {
	var benchmarks []*benchmark.Benchmark
	var unevaluated []*benchmark.Benchmark 
	var err error 

	// Setup the configuration
	cfg := benchmark.Configuration{Src: "tacle-bench/bench/sequential", Stats: "stats", Bin: "bin"}

	// Init environment
	if err := benchmark.Init_Env(cfg); nil != err {
		log.Fatal(err.Error())
	}

	// Init benchmarks
	benchmarks, err = benchmark.Init_Benchmarks(cfg)
	if nil != err {
		log.Fatal(err.Error())
	}

	// Extract any unevaluated benchmarks
	unevaluated, err = benchmark.Get_Unevaluated_Benchmarks(cfg, benchmarks)
	if nil != err {
		log.Fatal(err.Error())
	}

	// Evaluate all unevaluted benchmarks
	for _, b := range unevaluated {
		err = benchmark.Evaluate_Benchmark("cc", cfg, 10, b)
		if nil != err {
			log.Fatal(err.Error())
		}
	}

	// Print benchmarks
	for _, b := range benchmarks {
		fmt.Printf("%16s\t\t\t%.2f ns\t\t\t%.2f%%\n", b.Name, b.Runtime, b.Uncertainty)
	}
}
```