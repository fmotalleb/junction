# Benchmark Results for SNI Package

**Command run:**

```bash
go test -bench=. -benchtime 5s -benchmem
````

---

## System Information

* **OS:** linux
* **Architecture:** amd64
* **CPU:** Intel(R) Core(TM) i5-6500 CPU @ 3.20GHz
* **Package:** github.com/FMotalleb/junction/crypto/sni

---

## Benchmark Output

| Benchmark Name              | Iterations | ns/op | Bytes/op | Allocs/op |
| --------------------------- | ---------- | ----- | -------- | --------- |
| BenchmarkExtractHost-4      | 28,172,822 | 206.3 | 48       | 3         |
| BenchmarkParseClientHello-4 | 6,153,693  | 979.1 | 720      | 12        |

---

## Summary

* **BenchmarkExtractHost** runs approximately 4.7x faster than **BenchmarkParseClientHello**.
* **BenchmarkExtractHost** has significantly fewer memory allocations and lower bytes per operation.
* The more complex parsing done by **ParseClientHello** results in higher CPU and memory overhead.
