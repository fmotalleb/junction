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

| Benchmark Name              | Iterations | ns/op | Bytes/op | Allocs/op | Returns          |
| --------------------------- | ---------- | ----- | -------- | --------- | ---------------- |
| BenchmarkExtractHost-4 (str)| 28,172,822 | 206.3 | 48       | 3         | string           |
| BenchmarkExtractHost-4      | 47,327,336 | 131.1 | 0        | 0         | []byte           |
| BenchmarkParseClientHello-4 | 6,153,693  | 979.1 | 720      | 12        | *ClientHelloInfo |

---

## Summary

* **ExtractHost** is ~8.75x faster than **ParseClientHello**.
* **ExtractHost** has **ZERO** allocations.
* Avoid using **ParseClientHello** if possible.
