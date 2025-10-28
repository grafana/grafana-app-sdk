# Contributing guide

All contributions are welcome.

If there is something you're curious about with the SDK (project direction, functionality, etc.), please do not hesitate to visit our [Discussions](https://github.com/grafana/grafana-app-sdk/discussions) section.

If you discover a bug, or think something should be included in the SDK, feel free to file an issue, or open a PR.

## Releasing a new version

In order to release a new version, you can use the `scripts/release.sh` script, like so:

```sh
# Release a new patch version, e.g. 1.0.1
./scripts/release.sh patch

# Release a new minor version, e.g. 1.1.0
./scripts/release.sh minor

# Release a new major version, e.g. 2.0.0
./scripts/release.sh major
```

The script will make sure that you have the latest `main` version in your tree, will run linter / tests / build and it will create a semver-appropriate signed tag and push it to remote. Our CI automation will in turn create an appropriate Github release with artifacts. The script currently does not support pre-release versions.

## Performance Benchmarking

The project includes comprehensive benchmarks for performance-critical components. Benchmarks help measure and track performance improvements over time.

### Running Benchmarks

```bash
# Run all benchmarks across the project
make bench

# Run benchmarks for specific test
go test -bench=BenchmarkDefaultInformerSupplier ./benchmark/

# Compare Default vs Optimized suppliers statistically
make bench-compare
```

### Memory Profiling

Generate memory profiles to identify allocation hotspots:

```bash
# Generate memory and CPU profiles
make bench-profile

# View memory profile interactively
go tool pprof target/profiles/mem.out

# Generate visual flamegraph (requires graphviz)
go tool pprof -http=:8080 target/profiles/mem.out
```

Common pprof commands:
- `top` - Show top memory consumers
- `list <function>` - Show source code with allocations
- `web` - Generate SVG graph (requires graphviz)
- `png` - Generate PNG graph

### CPU Profiling

Analyze CPU hotspots and bottlenecks:

```bash
# Generate CPU profile from benchmarks
go test -bench=BenchmarkDefaultInformerSupplier -cpuprofile=cpu.out ./benchmark/

# Analyze CPU profile
go tool pprof cpu.out

# View interactive CPU flamegraph
go tool pprof -http=:8080 cpu.out
```

### Benchmark-Guided Optimization Workflow

Use benchmarks to measure the impact of performance optimizations using a baseline comparison approach:

```bash
# 1. Establish performance baseline before making changes
make bench-baseline

# 2. Make your code optimizations
# ... edit code to improve performance ...

# 3. Compare current performance against baseline
make bench-compare
```

This workflow runs benchmarks 6 times each (minimum for statistical confidence) and uses [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) to provide statistical comparison with p-values.

#### Example Output

After running `make bench-compare`, you'll see statistical analysis like:

```
name                                  old time/op    new time/op    delta
InformerSupplier/10000_objects-12       101ms ± 2%      87ms ± 3%  -13.86%  (p=0.002 n=6+6)
InformerSupplier/50000_objects-12       505ms ± 1%     425ms ± 2%  -15.84%  (p=0.000 n=6+6)

name                                  old alloc/op   new alloc/op   delta
InformerSupplier/10000_objects-12      15.7MB ± 0%    13.2MB ± 0%  -15.92%  (p=0.000 n=6+6)
InformerSupplier/50000_objects-12      75.3MB ± 0%    62.1MB ± 0%  -17.53%  (p=0.000 n=6+6)

name                                  old allocs/op  new allocs/op  delta
InformerSupplier/10000_objects-12        293k ± 0%      248k ± 0%  -15.36%  (p=0.000 n=6+6)
InformerSupplier/50000_objects-12       1.39M ± 0%     1.08M ± 0%  -22.30%  (p=0.000 n=6+6)
```

**Understanding benchstat Output:**
- **delta**: Percentage change (negative = improvement for time/memory)
- **p-value**: Statistical significance (p < 0.05 = statistically significant difference)
- **n=6+6**: Number of samples from baseline and current (6 iterations each)
- **±%**: Standard deviation / confidence interval

**Interpreting Results:**
- **Negative delta + low p-value** (p < 0.05): Optimization successful!
- **Positive delta**: Performance regression, investigate further
- **Small delta (~<5%) + high p-value**: Change likely not significant
- **Need more samples**: Increase `-benchtime` if you see this warning

#### When to Use Benchmarks vs Profiling

**Use benchmarks (`bench-baseline` + `bench-compare`) when:**
- Measuring the overall impact of an optimization
- Comparing performance before/after code changes
- Validating that optimizations actually improved performance
- Need statistical confidence in results

**Use profiling (`bench-profile`) when:**
- Identifying where to optimize (finding hot paths)
- Understanding what causes allocations or CPU usage
- Don't know where the bottleneck is yet
- Planning optimization work

**Best workflow:** Profile first to identify bottlenecks → Optimize code → Benchmark to validate improvement

#### Practical Example: Optimization Cycle

```bash
# 1. Profile to identify bottleneck
make bench-profile
go tool pprof -http=:8080 target/profiles/mem.out
# Identify: High allocations in parseResponse() function

# 2. Establish baseline
make bench-baseline

# 3. Optimize the identified function
# Edit code to reduce allocations in parseResponse()

# 4. Validate improvement
make bench-compare
# See: -15% memory allocations, p=0.001 (statistically significant!)

# 5. If improvement is good, commit. If not, iterate.
```

### Benchmark Best Practices

1. **Isolate Changes**: Run benchmarks on the same hardware when comparing performance
2. **One Change at a Time**: Benchmark each optimization separately to understand its impact
3. **Reset Baseline**: Run `make bench-baseline` again after committing changes
4. **Memory Stats**: Always include `-benchmem` flag to track allocations (done automatically by our targets)
5. **Profile-Guided**: Use profiling to identify actual bottlenecks before optimizing
6. **Statistical Significance**: Only trust results with p < 0.05 and sufficient sample size

### Understanding Benchmark Results

```
BenchmarkDefaultInformerSupplier/10000_objects-12    34    101046569 ns/op    15661230 B/op    293083 allocs/op
```

- `34` - Number of iterations (b.N)
- `101046569 ns/op` - Average time per operation (101ms)
- `15661230 B/op` - Bytes allocated per operation (~15MB)
- `293083 allocs/op` - Number of allocations per operation
- `-12` - Number of CPU cores used
