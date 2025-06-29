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

| Benchmark Name                  | Iterations | ns/op | Bytes/op | Allocs/op |
| ------------------------------- | ---------- | ----- | -------- | --------- |
| BenchmarkExtractHost-4          | 47,327,336 | 131.1 | 0        | 0         |
| BenchmarkUnmarshalClientHello-4 | 11,734,141 | 504.5 | 0        | 0         |

---

## Summary

* **ExtractHost** is ~4x faster than **ParseClientHello**.
* Avoid using **ParseClientHello** if possible.
* Use ParseClientHello only when full handshake parsing is **required**.
