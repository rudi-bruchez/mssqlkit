# Adversarial Code Review: mssqlkit

## Executive Summary
The `mssqlkit` repository demonstrates an exceptionally strong defensive posture. The architectural separation between offline code-generation and runtime execution eliminates entire classes of vulnerabilities (such as runtime file parsing, SSRF, and injection). The runtime library uses static queries and safe type conversions. No critical or high-severity vulnerabilities were found.

## Positive Security Controls
1. **Offline Code Generation**: The decision to generate `ddlmatrix` and `platform` facts offline using `codegen` means the runtime environment does not need to parse YAML, fetch data over the network, or handle malformed schemas.
2. **Defensive Network Bounds**: In `fetchbuilds`, network calls use explicit timeouts (`Timeout: 60 * time.Second`) and memory-bound reads (`io.LimitReader` capped at 32MB) to prevent resource exhaustion and Slowloris attacks.
3. **Fail-Closed Fallbacks**: In `platform/platform.go`, if an unknown `EngineEdition` is encountered, the tool falls back to `FamilyUnknown` rather than defaulting to a potentially more privileged platform like `FamilyBox`.
4. **SQL Injection Prevention**: `Probe()` uses a hardcoded `const probeQuery` with no string concatenation or parameters, effectively eliminating SQL injection risks.
5. **Safe Template Injection**: Go source code generation in `codegen/templates.go` uses `fmt.Sprintf("%q")` for string fields, preventing any possibility of malicious code injection via the upstream Microsoft Learn page or compromised YAML files.
6. **Defensive Query Options**: The probe query uses `OPTION (RECOMPILE, MAXDOP 1)`, which is a reliability and defense-in-depth measure, minimizing worker-thread footprint and preventing the tool from inducing an incident while probing a distressed SQL Server.

## Low-Severity Findings / Robustness Issues

### 1. Incomplete Slice Comparison in `compareBuild`
**File**: `platform/platform.go`
**Description**: The `compareBuild(a, b []int)` function loops only up to the length of the shorter slice (`for i := 0; i < len(a) && i < len(b); i++`). While `parseBuild` pads slices with zeros to ensure a minimum length of 4 (so standard 4-part build numbers compare correctly), if a 5-part version string were somehow encountered, the 5th part would be ignored. A version like `16.0.4295.3.1` would evaluate as equal to `16.0.4295.3`.
**Recommendation**: Add a length check after the loop to consider the longer slice as "greater" if all preceding elements are equal.
```go
func compareBuild(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] { return -1 }
		if a[i] > b[i] { return 1 }
	}
	if len(a) < len(b) { return -1 }
	if len(a) > len(b) { return 1 }
	return 0
}
```

### 2. Unbounded Recursive HTML Walk in `fetchbuilds`
**File**: `codegen/cmd/fetchbuilds/main.go`
**Description**: The `parse` and `parseTable` functions use a recursive closure (`walk`) to traverse the HTML DOM. While the input stream is correctly bounded to 32MB using `io.LimitReader`, an adversary in control of the `learn.microsoft.com` endpoint could serve a 32MB file consisting of deeply nested `<div>` tags. This could potentially trigger a stack overflow in the Go runtime during DOM traversal.
**Recommendation**: Given this is an offline maintenance tool, the impact is minimal (local crash). No immediate action is required, but an iterative approach to node traversal could be considered for maximum robustness.

### 3. Missing `MAX_DURATION` limits in `WAIT_AT_LOW_PRIORITY` (Design Note)
**File**: `ddlmatrix/data/features.yaml`
**Description**: The matrix indicates support for `WAIT_AT_LOW_PRIORITY`. While the tool identifies if the platform supports it, consumers might emit DDL that waits indefinitely if they only rely on the boolean availability.
**Recommendation**: Ensure downstream tools utilizing this matrix enforce sensible `MAX_DURATION` boundaries and handle the `ABORT_AFTER_WAIT` clause correctly to prevent self-inflicted denial of service.

## Conclusion
The `mssqlkit` codebase sets a high bar for secure design. By shifting the complexity of external data processing (web scraping, YAML parsing) to an offline maintenance phase, the runtime library remains lean, deterministic, and highly resilient against attack.
