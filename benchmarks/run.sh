# TIDE benchmarks (T111) — owner: Jason (maintainer)
#
# Discipline (PRD §1.4.5, ADR-001 §8): no throughput/latency number appears in
# docs, blog posts or READMEs unless it was produced by this script on named
# hardware. Record: hardware, commit, config, full output.

set -eu
echo "== TIDE benchmark =="
echo "date: $(date -u +%FT%TZ)"
echo "commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "cpu: $(uname -m) $(sysctl -n machdep.cpu.brand_string 2>/dev/null || lscpu 2>/dev/null | grep 'Model name' | head -1)"
go test -bench=. -benchtime=10000x -run=^$ ./benchmarks/ | tee benchmarks/last.txt
echo "saved to benchmarks/last.txt — paste hardware + output into docs/benchmarks.md"
